// Package sshstate manages private, identity-bound OpenSSH state. It never
// persists bearer credentials. The CLI briefly persists a single-use relay
// ticket in ticket.json (0600) until its proxy claims and deletes it.
package sshstate

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

var ErrState = errors.New("invalid or inaccessible private SSH state")
var ErrCertificate = errors.New("invalid sandbox SSH certificate")

type Binding struct {
	Origin    string `json:"origin"`
	Identity  string `json:"identity"`
	Profile   string `json:"profile"`
	Workspace string `json:"workspace"`
	Sandbox   string `json:"sandbox"`
}

func (b Binding) ID() string {
	data, _ := json.Marshal(b)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

type State struct {
	Dir     string
	Binding Binding
	Key     ssh.Signer
}

func PrivateDir(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return ErrState
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return ErrState
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return ErrState
	}
	return nil
}

// Open creates one reusable key per binding. Linking a complete temporary key
// prevents concurrent prepares from overwriting it or reading a partial key.
func Open(root string, binding Binding) (*State, error) {
	for _, dir := range []string{root, filepath.Join(root, "ssh"), filepath.Join(root, "ssh", binding.ID())} {
		if err := PrivateDir(dir); err != nil {
			return nil, err
		}
	}
	state := &State{Dir: filepath.Join(root, "ssh", binding.ID()), Binding: binding}
	keyPath := state.IdentityFile()
	if _, err := os.Lstat(keyPath); errors.Is(err, os.ErrNotExist) {
		_, key, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, ErrState
		}
		block, err := ssh.MarshalPrivateKey(key, "")
		if err != nil {
			return nil, ErrState
		}
		tmp, err := writeTemp(state.Dir, pem.EncodeToMemory(block))
		if err != nil {
			return nil, err
		}
		err = os.Link(tmp, keyPath)
		_ = os.Remove(tmp)
		if err != nil && !errors.Is(err, os.ErrExist) {
			return nil, ErrState
		}
	}
	data, err := ReadPrivate(keyPath)
	if err != nil {
		return nil, err
	}
	state.Key, err = ssh.ParsePrivateKey(data)
	if err != nil || state.Key.PublicKey().Type() != ssh.KeyAlgoED25519 {
		return nil, ErrState
	}
	return state, nil
}

func ReadPrivate(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, ErrState
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, ErrState
	}
	return data, nil
}

func writeTemp(dir string, data []byte) (string, error) {
	f, err := os.CreateTemp(dir, ".ssh-*")
	if err != nil {
		return "", ErrState
	}
	name := f.Name()
	_, werr := f.Write(data)
	serr := f.Sync()
	cerr := f.Close()
	if werr != nil || serr != nil || cerr != nil {
		_ = os.Remove(name)
		return "", ErrState
	}
	return name, nil
}

func WritePrivate(path string, data []byte) error {
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return ErrState
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return ErrState
	}
	tmp, err := writeTemp(filepath.Dir(path), data)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	if err := os.Rename(tmp, path); err != nil {
		return ErrState
	}
	return nil
}

func (s *State) IdentityFile() string    { return filepath.Join(s.Dir, "id_ed25519") }
func (s *State) CertificateFile() string { return filepath.Join(s.Dir, "id_ed25519-cert.pub") }
func (s *State) KnownHostsFile() string  { return filepath.Join(s.Dir, "known_hosts") }
func (s *State) PublicKey() string {
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(s.Key.PublicKey())))
}

func (s *State) SaveCertificate(raw string, expires time.Time, now time.Time) error {
	key, _, options, rest, err := ssh.ParseAuthorizedKey([]byte(raw))
	if err != nil || len(options) != 0 || len(rest) != 0 {
		return ErrCertificate
	}
	cert, ok := key.(*ssh.Certificate)
	if !ok || cert.CertType != ssh.UserCert || !bytes.Equal(cert.Key.Marshal(), s.Key.PublicKey().Marshal()) || len(cert.ValidPrincipals) != 1 || cert.ValidPrincipals[0] != "user" || cert.ValidBefore != uint64(expires.Unix()) || cert.ValidBefore <= cert.ValidAfter || cert.ValidBefore-cert.ValidAfter > uint64((16*time.Minute)/time.Second) || len(cert.CriticalOptions) != 0 {
		return ErrCertificate
	}
	// Allow five minutes of client skew while retaining a bounded certificate
	// lifetime. Core and sshd enforce expiry on their own clocks at redemption.
	after := time.Unix(int64(cert.ValidAfter), 0)
	before := time.Unix(int64(cert.ValidBefore), 0)
	if now.Before(after.Add(-5*time.Minute)) || !now.Before(before.Add(5*time.Minute)) {
		return ErrCertificate
	}
	checkTime := now
	if checkTime.Before(after) {
		checkTime = after
	}
	if !checkTime.Before(before) {
		checkTime = before.Add(-time.Second)
	}
	checker := ssh.CertChecker{Clock: func() time.Time { return checkTime }}
	if err := checker.CheckCert("user", cert); err != nil {
		return ErrCertificate
	}
	return WritePrivate(s.CertificateFile(), ssh.MarshalAuthorizedKey(cert))
}

func (s *State) SaveHost(alias, ca string) error {
	if alias != s.Binding.Sandbox+".sandbox.runzivo" {
		return ErrCertificate
	}
	key, _, options, rest, err := ssh.ParseAuthorizedKey([]byte(ca))
	if err != nil || len(options) != 0 || len(rest) != 0 || key.Type() != ssh.KeyAlgoED25519 || strings.ContainsAny(ca, "\r\n") {
		return ErrCertificate
	}
	return WritePrivate(s.KnownHostsFile(), []byte("@cert-authority "+alias+" "+strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))+"\n"))
}
