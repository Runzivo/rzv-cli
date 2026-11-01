package sshstate

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func binding() Binding {
	return Binding{Origin: "https://api.example.test", Identity: "subject", Workspace: "workspace", Sandbox: "sbx_0123456789abcdef01234567", Profile: "default"}
}
func signer(t *testing.T) ssh.Signer {
	t.Helper()
	_, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	s, e := ssh.NewSignerFromKey(key)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func TestPrivateKeysAreReusedAndBound(t *testing.T) {
	root := t.TempDir()
	b := binding()
	var wg sync.WaitGroup
	keys := make(chan string, 8)
	for range 8 {
		wg.Go(func() {
			s, e := Open(root, b)
			if e != nil {
				t.Error(e)
				return
			}
			keys <- s.PublicKey()
		})
	}
	wg.Wait()
	close(keys)
	first := ""
	for k := range keys {
		if first != "" && first != k {
			t.Fatal("concurrent key replacement")
		}
		first = k
	}
	s, e := Open(root, b)
	if e != nil {
		t.Fatal(e)
	}
	for path, mode := range map[string]os.FileMode{s.Dir: 0700, s.IdentityFile(): 0600} {
		info, e := os.Stat(path)
		if e != nil || info.Mode().Perm() != mode {
			t.Fatalf("%s: %v %v", path, info, e)
		}
	}
	for _, change := range []func(*Binding){func(b *Binding) { b.Workspace = "other" }, func(b *Binding) { b.Sandbox = "other" }, func(b *Binding) { b.Identity = "other" }, func(b *Binding) { b.Origin = "https://other.test" }, func(b *Binding) { b.Profile = "other" }} {
		other := b
		change(&other)
		s2, e := Open(root, other)
		if e != nil || s2.PublicKey() == first {
			t.Fatal("binding not isolated", e)
		}
	}
	entries, _ := os.ReadDir(s.Dir)
	if len(entries) != 1 {
		t.Fatal("temporary keys left behind")
	}
}
func TestCertificatesAndHostAuthority(t *testing.T) {
	s, e := Open(t.TempDir(), binding())
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now().Truncate(time.Second)
	ca := signer(t)
	makeCert := func(key ssh.PublicKey, principal string, before time.Time) *ssh.Certificate {
		cert := &ssh.Certificate{Key: key, CertType: ssh.UserCert, ValidPrincipals: []string{principal}, ValidAfter: uint64(now.Add(-time.Minute).Unix()), ValidBefore: uint64(before.Unix())}
		if e := cert.SignCert(rand.Reader, ca); e != nil {
			t.Fatal(e)
		}
		return cert
	}
	expiry := now.Add(15 * time.Minute)
	valid := makeCert(s.Key.PublicKey(), "user", expiry)
	if e := s.SaveCertificate(string(ssh.MarshalAuthorizedKey(valid)), expiry, now); e != nil {
		t.Fatal(e)
	}
	for _, skew := range []time.Duration{-4 * time.Minute, 4 * time.Minute} {
		if e := s.SaveCertificate(string(ssh.MarshalAuthorizedKey(valid)), expiry, now.Add(skew)); e != nil {
			t.Fatal("clock skew rejected", skew, e)
		}
	}
	if e := s.SaveCertificate(string(ssh.MarshalAuthorizedKey(valid)), expiry, now.Add(-7*time.Minute)); e == nil {
		t.Fatal("excessive skew accepted")
	}
	tooLong := makeCert(s.Key.PublicKey(), "user", now.Add(16*time.Minute))
	if e := s.SaveCertificate(string(ssh.MarshalAuthorizedKey(tooLong)), now.Add(16*time.Minute), now.Add(4*time.Minute)); e == nil {
		t.Fatal("overlong TTL accepted")
	}
	tampered := *valid
	tampered.Serial++
	if e := s.SaveCertificate(string(ssh.MarshalAuthorizedKey(&tampered)), expiry, now.Add(-4*time.Minute)); e == nil {
		t.Fatal("invalid signature accepted with skew")
	}
	for _, cert := range []*ssh.Certificate{makeCert(signer(t).PublicKey(), "user", expiry), makeCert(s.Key.PublicKey(), "root", expiry), makeCert(s.Key.PublicKey(), "user", now.Add(-6*time.Minute))} {
		if e := s.SaveCertificate(string(ssh.MarshalAuthorizedKey(cert)), time.Unix(int64(cert.ValidBefore), 0), now); e == nil {
			t.Fatal("invalid certificate accepted")
		}
	}
	if e := s.SaveCertificate(string(ssh.MarshalAuthorizedKey(valid)), expiry.Add(time.Second), now); e == nil {
		t.Fatal("mismatched expiry accepted")
	}
	alias := s.Binding.Sandbox + ".sandbox.runzivo.dev"
	raw := string(bytes.TrimSpace(ssh.MarshalAuthorizedKey(ca.PublicKey())))
	if e := s.SaveHost(alias, raw); e != nil {
		t.Fatal(e)
	}
	data, e := ReadPrivate(s.KnownHostsFile())
	if e != nil || string(data) != "@cert-authority "+alias+" "+raw+"\n" {
		t.Fatal("wrong authority", e)
	}
	if s.SaveHost("*.rzv", raw) == nil || s.SaveHost(alias, raw+"\nanything") == nil {
		t.Fatal("unsafe authority accepted")
	}
	for _, invalid := range []string{s.Binding.Sandbox + ".sandbox.runzivo", alias + ".evil", "*.sandbox.runzivo.dev", "sbx_aaaaaaaaaaaaaaaaaaaaaaaa.sandbox.runzivo.dev"} {
		if s.SaveHost(invalid, raw) == nil {
			t.Fatalf("invalid host alias accepted: %q", invalid)
		}
	}
	if after, err := ReadPrivate(s.KnownHostsFile()); err != nil || !bytes.Equal(after, data) {
		t.Fatal("invalid alias changed the pinned authority", err)
	}
}
func TestStateRefusesSymlinksAndPublicPrivateKeys(t *testing.T) {
	root := t.TempDir()
	s, e := Open(root, binding())
	if e != nil {
		t.Fatal(e)
	}
	if e := os.Chmod(s.IdentityFile(), 0644); e != nil {
		t.Fatal(e)
	}
	if _, e := Open(root, binding()); e == nil {
		t.Fatal("public private key accepted")
	}
	target := filepath.Join(root, "outside")
	if e := os.WriteFile(target, []byte("original"), 0600); e != nil {
		t.Fatal(e)
	}
	link := filepath.Join(s.Dir, "link")
	if e := os.Symlink(target, link); e != nil {
		t.Skip(e)
	}
	if WritePrivate(link, []byte("replace")) == nil {
		t.Fatal("symlink replaced")
	}
	if data, _ := os.ReadFile(target); string(data) != "original" {
		t.Fatal("target changed")
	}
}
