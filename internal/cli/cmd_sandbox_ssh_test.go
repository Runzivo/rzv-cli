package cli

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // WebSocket handshake, not authentication.
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Runzivo/rzv-cli/internal/api"
	"github.com/Runzivo/rzv-cli/internal/config"
	"github.com/Runzivo/rzv-cli/internal/oidctest"
	"github.com/Runzivo/rzv-cli/internal/sshstate"
	"golang.org/x/crypto/ssh"
)

type sshFixture struct {
	server              *httptest.Server
	ca, hostCA          ssh.Signer
	mu                  sync.Mutex
	tickets             map[string]bool
	issued, connections int
	serve               func(net.Conn, *bufio.Reader)
}

func testSSHSigner(t *testing.T) ssh.Signer {
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
func sshFrame(op byte, data []byte) []byte {
	head := []byte{0x80 | op}
	switch {
	case len(data) < 126:
		head = append(head, byte(len(data)))
	case len(data) <= 65535:
		head = append(head, 126, byte(len(data)>>8), byte(len(data)))
	default:
		head = append(head, 127)
		head = binary.BigEndian.AppendUint64(head, uint64(len(data)))
	}
	return append(head, data...)
}
func newSSHFixture(t *testing.T, serve func(net.Conn, *bufio.Reader)) *sshFixture {
	t.Helper()
	f := &sshFixture{ca: testSSHSigner(t), hostCA: testSSHSigner(t), tickets: map[string]bool{}, serve: serve}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := "/v1/customer/tenants/t-1/sandboxes/" + sbxID
		f.mu.Lock()
		hostCA := f.hostCA
		serve := f.serve
		f.mu.Unlock()
		host := api.SSHHost{HostCAPublicKey: strings.TrimSpace(string(ssh.MarshalAuthorizedKey(hostCA.PublicKey()))), HostKeyAlias: sbxID + ".sandbox.runzivo.dev"}
		if r.URL.Path == path+"/ssh" {
			f.mu.Lock()
			ok := f.tickets[r.URL.Query().Get("ticket")]
			delete(f.tickets, r.URL.Query().Get("ticket"))
			if ok {
				f.connections++
			}
			f.mu.Unlock()
			if !ok {
				http.Error(w, "invalid ticket", 401)
				return
			}
			conn, rw, e := w.(http.Hijacker).Hijack()
			if e != nil {
				t.Error(e)
				return
			}
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(45 * time.Second))
			sum := sha1.Sum([]byte(r.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11")) //nolint:gosec
			_, _ = fmt.Fprintf(rw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", base64.StdEncoding.EncodeToString(sum[:]))
			_ = rw.Flush()
			serve(conn, rw.Reader)
			return
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			http.Error(w, "missing authorization", 401)
			return
		}
		if r.URL.Path == "/v1/customer/memberships" {
			_, _ = io.WriteString(w, `{"memberships":[{"tenant_id":"t-1","tenant_slug":"demo","tenant_name":"Demo","role":"owner"}]}`)
			return
		}
		if r.URL.Path == path+"/ssh-host" && r.Method == "GET" {
			_ = json.NewEncoder(w).Encode(host)
			return
		}
		if r.URL.Path != path+"/ssh-ticket" || r.Method != "POST" {
			http.NotFound(w, r)
			return
		}
		var input struct {
			PublicKey string `json:"public_key"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			http.Error(w, "bad key", 400)
			return
		}
		key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(input.PublicKey))
		if err != nil {
			http.Error(w, "bad key", 400)
			return
		}
		expires := time.Now().Truncate(time.Second).Add(15 * time.Minute)
		cert := &ssh.Certificate{Key: key, CertType: ssh.UserCert, ValidPrincipals: []string{"user"}, ValidAfter: uint64(time.Now().Add(-time.Minute).Unix()), ValidBefore: uint64(expires.Unix()), Permissions: ssh.Permissions{Extensions: map[string]string{"permit-pty": "", "permit-port-forwarding": ""}}}
		if err := cert.SignCert(rand.Reader, f.ca); err != nil {
			t.Error(err)
			return
		}
		nonce := make([]byte, 24)
		_, _ = rand.Read(nonce)
		ticket := hex.EncodeToString(nonce)
		f.mu.Lock()
		f.tickets[ticket] = true
		f.issued++
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(api.SSHTicket{SSHHost: host, URL: strings.Replace(f.server.URL, "http://", "ws://", 1) + path + "/ssh", Ticket: ticket, ExpiresAt: time.Now().Add(30 * time.Second), Certificate: string(ssh.MarshalAuthorizedKey(cert)), CertificateExpiresAt: expires, Principal: "user"})
	}))
	t.Cleanup(f.server.Close)
	return f
}
func fixtureDeps(f *sshFixture, root string, stdin io.Reader, stdout, stderr io.Writer) Deps {
	return Deps{Store: &config.Store{Dir: root}, Stdin: stdin, Stdout: stdout, Stderr: stderr, Getenv: func(k string) string {
		switch k {
		case "RZV_API_KEY":
			return "fixture-key"
		case "RZV_API_URL":
			return f.server.URL
		case "RZV_WORKSPACE":
			return "t-1"
		case "HOME":
			return root
		}
		return ""
	}}
}

func TestSSHProxyBinaryHalfCloseAndConfig(t *testing.T) {
	payload := bytes.Repeat([]byte{0, 255, 128, 1, 13, 10, 17, 19}, (6<<20)/8)
	f := newSSHFixture(t, func(conn net.Conn, r *bufio.Reader) {
		for {
			op, data, e := readClientFrame(r)
			if e != nil {
				return
			}
			if op == 2 {
				_, _ = conn.Write(sshFrame(2, data))
				continue
			}
			if op != 1 || string(data) != `{"type":"eof"}` {
				t.Error("invalid client EOF")
				return
			}
			_, _ = conn.Write(sshFrame(2, []byte("after-eof")))
			_, _ = conn.Write(sshFrame(1, []byte(`{"type":"eof"}`)))
			_, _ = conn.Write(sshFrame(8, []byte{3, 232}))
			return
		}
	})
	root := t.TempDir()
	var output, stderr bytes.Buffer
	deps := fixtureDeps(f, root, bytes.NewReader(payload), &output, &stderr)
	if code := Run(context.Background(), []string{"sandboxes", "ssh", sbxID, "--stdio"}, deps); code != 0 {
		t.Fatalf("proxy exit %d: %s", code, stderr.String())
	}
	want := append(append([]byte{}, payload...), []byte("after-eof")...)
	if !bytes.Equal(output.Bytes(), want) {
		t.Fatalf("binary mismatch: %d/%d", output.Len(), len(want))
	}
	output.Reset()
	stderr.Reset()
	if code := Run(context.Background(), []string{"sandboxes", "ssh-config", sbxID, "--write"}, deps); code != 0 {
		t.Fatal(code, stderr.String())
	}
	data, e := os.ReadFile(filepath.Join(root, ".ssh", "rzv", sbxID+".conf"))
	if e != nil {
		t.Fatal(e)
	}
	for _, s := range []string{"StrictHostKeyChecking yes", "HostKeyAlias " + sbxID + ".sandbox.runzivo.dev", "ForwardAgent no", "ForwardX11 no", "Match host", "--stdio", "--refresh"} {
		if !strings.Contains(string(data), s) {
			t.Fatal("missing config", s)
		}
	}
	for range 2 {
		if code := Run(context.Background(), []string{"sandboxes", "ssh-config", sbxID, "--write"}, deps); code != 0 {
			t.Fatal(code)
		}
	}
	include, _ := os.ReadFile(filepath.Join(root, ".ssh", "config"))
	if strings.Count(string(include), "Include") != 1 {
		t.Fatal("duplicate include")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.issued != 1 {
		t.Fatal("ssh-config unnecessarily minted tickets")
	}
}

func TestSSHProxyServerCloseAndInvalidControl(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		t.Run(fmt.Sprint(invalid), func(t *testing.T) {
			f := newSSHFixture(t, func(c net.Conn, _ *bufio.Reader) {
				if invalid {
					_, _ = c.Write(sshFrame(1, []byte(`{"type":"unknown"}`)))
				} else {
					_, _ = c.Write(sshFrame(8, []byte{3, 232}))
				}
			})
			input, writer := io.Pipe()
			defer writer.Close()
			var output, stderr bytes.Buffer
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			code := Run(ctx, []string{"sandboxes", "ssh-proxy", sbxID + ".rzv"}, fixtureDeps(f, t.TempDir(), input, &output, &stderr))
			if (code != 0) != invalid || output.Len() != 0 {
				t.Fatal(code, output.String(), stderr.String())
			}
		})
	}
}

func writeSSHSigner(t *testing.T, dir, name string) (ssh.Signer, string) {
	t.Helper()
	_, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	s, e := ssh.NewSignerFromKey(key)
	if e != nil {
		t.Fatal(e)
	}
	block, e := ssh.MarshalPrivateKey(key, "")
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(dir, name)
	if e := os.WriteFile(path, pem.EncodeToMemory(block), 0600); e != nil {
		t.Fatal(e)
	}
	return s, path
}

func startTestSSHD(t *testing.T, f *sshFixture, dir string) string {
	t.Helper()
	sshd, e := exec.LookPath("sshd")
	if e != nil {
		t.Skip("local OpenSSH sshd is unavailable")
	}
	host, keyPath := writeSSHSigner(t, dir, "host-key")
	cert := &ssh.Certificate{Key: host.PublicKey(), CertType: ssh.HostCert, ValidPrincipals: []string{sbxID + ".sandbox.runzivo.dev"}, ValidAfter: uint64(time.Now().Add(-time.Minute).Unix()), ValidBefore: uint64(time.Now().Add(time.Hour).Unix())}
	if e := cert.SignCert(rand.Reader, f.hostCA); e != nil {
		t.Fatal(e)
	}
	for name, data := range map[string][]byte{"host-cert.pub": ssh.MarshalAuthorizedKey(cert), "user-ca.pub": ssh.MarshalAuthorizedKey(f.ca.PublicKey()), "principals": []byte("user\n")} {
		if e := os.WriteFile(filepath.Join(dir, name), data, 0600); e != nil {
			t.Fatal(e)
		}
	}
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	address := listener.Addr().String()
	_, port, _ := net.SplitHostPort(address)
	_ = listener.Close()
	current, e := user.Current()
	if e != nil {
		t.Fatal(e)
	}
	conf := fmt.Sprintf("ListenAddress 127.0.0.1\nPort %s\nHostKey %s\nHostCertificate %s\nTrustedUserCAKeys %s\nAuthorizedPrincipalsFile %s\nAuthorizedKeysFile none\nPasswordAuthentication no\nKbdInteractiveAuthentication no\nUsePAM no\nStrictModes no\nPidFile %s\nAllowUsers %s\nAllowTcpForwarding local\nAllowAgentForwarding no\nX11Forwarding no\nSubsystem sftp internal-sftp\nLogLevel ERROR\n", port, keyPath, filepath.Join(dir, "host-cert.pub"), filepath.Join(dir, "user-ca.pub"), filepath.Join(dir, "principals"), filepath.Join(dir, "sshd.pid"), current.Username)
	configPath := filepath.Join(dir, "sshd_config")
	if e := os.WriteFile(configPath, []byte(conf), 0600); e != nil {
		t.Fatal(e)
	}
	log, e := os.Create(filepath.Join(dir, "sshd.log"))
	if e != nil {
		t.Fatal(e)
	}
	cmd := exec.Command(sshd, "-D", "-e", "-f", configPath)
	cmd.Stdout = log
	cmd.Stderr = log
	if e := cmd.Start(); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait(); _ = log.Close() })
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, e := net.DialTimeout("tcp", address, 50*time.Millisecond)
		if e == nil {
			_ = conn.Close()
			return address
		}
		time.Sleep(20 * time.Millisecond)
	}
	data, _ := os.ReadFile(log.Name())
	t.Fatalf("test sshd did not start: %s", data)
	return ""
}

func relayToSSHD(t *testing.T, address string) func(net.Conn, *bufio.Reader) {
	return func(conn net.Conn, reader *bufio.Reader) {
		remote, e := net.Dial("tcp", address)
		if e != nil {
			t.Error(e)
			return
		}
		defer remote.Close()
		var writes sync.Mutex
		send := func(op byte, data []byte) {
			writes.Lock()
			defer writes.Unlock()
			_, _ = conn.Write(sshFrame(op, data))
		}
		inputDone := make(chan struct{})
		go func() {
			defer close(inputDone)
			for {
				op, data, e := readClientFrame(reader)
				if e != nil {
					return
				}
				switch op {
				case 2:
					if _, e := remote.Write(data); e != nil {
						return
					}
				case 1:
					if string(data) == `{"type":"eof"}` {
						_ = remote.(*net.TCPConn).CloseWrite()
						return
					}
				case 8:
					return
				}
			}
		}()
		buffer := make([]byte, 32<<10)
		for {
			n, e := remote.Read(buffer)
			if n > 0 {
				send(2, buffer[:n])
			}
			if e != nil {
				send(1, []byte(`{"type":"eof"}`))
				send(8, []byte{3, 232})
				_ = conn.Close()
				<-inputDone
				return
			}
		}
	}
}

func TestOpenSSHFileToolsForwardingAndRefresh(t *testing.T) {
	for _, tool := range []string{"ssh", "scp", "sftp", "rsync", "sshd"} {
		if _, e := exec.LookPath(tool); e != nil {
			t.Skip(tool + " unavailable")
		}
	}
	root := t.TempDir()
	binaryPath := filepath.Join(root, "rzv binary")
	stateRoot := filepath.Join(root, "state with spaces %")
	build := exec.Command("go", "build", "-o", binaryPath, "../../cmd/rzv")
	if out, e := build.CombinedOutput(); e != nil {
		t.Fatalf("build: %v %s", e, out)
	}
	f := newSSHFixture(t, nil)
	address := startTestSSHD(t, f, root)
	f.mu.Lock()
	f.serve = relayToSSHD(t, address)
	f.mu.Unlock()
	env := append(os.Environ(), "RZV_CONFIG_DIR="+stateRoot, "RZV_API_KEY=fixture-key", "RZV_API_URL="+f.server.URL, "RZV_WORKSPACE=t-1", "HOME="+root)
	run := func(tool string, args ...string) []byte {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, tool, args...)
		cmd.Env = env
		out, e := cmd.CombinedOutput()
		if e != nil {
			t.Fatalf("%s failed: %v\n%s", filepath.Base(tool), e, out)
		}
		return out
	}
	configData := run(binaryPath, "sandboxes", "ssh-config", sbxID)
	path := filepath.Join(root, "client.conf")
	if e := os.WriteFile(path, configData, 0600); e != nil {
		t.Fatal(e)
	}
	current, _ := user.Current()
	alias := sbxID + ".rzv"
	if out := run("ssh", "-F", path, "-l", current.Username, alias, "printf 'ssh-ok'"); string(out) != "ssh-ok" {
		t.Fatalf("SSH output %q", out)
	}
	// The transport must outlive its 15-second WebSocket handshake context.
	if out := run("ssh", "-F", path, "-l", current.Username, alias, "sleep 16; printf session-alive"); string(out) != "session-alive" {
		t.Fatal(string(out))
	}
	// The next connection must refresh an expired/missing certificate before
	// OpenSSH loads it. The proxy must consume a fresh ticket for each connection.
	stateDirs, _ := filepath.Glob(filepath.Join(stateRoot, "ssh", "*"))
	if len(stateDirs) != 1 {
		t.Fatal(stateDirs)
	}
	certPath := filepath.Join(stateDirs[0], "id_ed25519-cert.pub")
	before, _ := os.ReadFile(certPath)
	_ = os.Remove(certPath)
	if out := run("ssh", "-F", path, "-l", current.Username, alias, "printf reconnect"); string(out) != "reconnect" {
		t.Fatal(string(out))
	}
	after, _ := os.ReadFile(certPath)
	if bytes.Equal(before, after) {
		t.Fatal("certificate was not refreshed")
	}
	// The test sshd accepts the local account, while production uses user.
	// Override the account only in the test executable shim, never the CLI.
	sshPath, _ := exec.LookPath("ssh")
	shimDir := filepath.Join(root, "bin")
	if e := os.Mkdir(shimDir, 0700); e != nil {
		t.Fatal(e)
	}
	shim := "#!/bin/sh\nexec " + shellArg(sshPath) + " -l " + shellArg(current.Username) + " \"$@\"\n"
	if e := os.WriteFile(filepath.Join(shimDir, "ssh"), []byte(shim), 0700); e != nil {
		t.Fatal(e)
	}
	env = append(env, "PATH="+shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	// Wrapper preserves remote exit status with system OpenSSH.
	cmd := exec.Command(binaryPath, "sandboxes", "ssh", sbxID, "--", "exit 17")
	cmd.Env = env
	out, e := cmd.CombinedOutput()
	var status *exec.ExitError
	if !errors.As(e, &status) || status.ExitCode() != 17 {
		t.Fatalf("remote exit: %v %s", e, out)
	}
	// A command beginning with -o must reach the remote shell, never OpenSSH's
	// option parser. If the guard regresses, this creates a local marker file.
	marker := filepath.Join(root, "proxy-injection")
	attack := "-oProxyCommand=touch " + shellArg(marker)
	cmd = exec.Command(binaryPath, "sandboxes", "ssh", sbxID, "--", "--", attack)
	cmd.Env = env
	out, e = cmd.CombinedOutput()
	if !errors.As(e, &status) || status.ExitCode() == 255 || !bytes.Contains(out, []byte("ProxyCommand")) {
		t.Fatalf("leading option was not a remote command: %v %s", e, out)
	}
	if _, e := os.Stat(marker); !os.IsNotExist(e) {
		t.Fatal("remote text executed locally")
	}
	source := filepath.Join(root, "source.bin")
	payload := bytes.Repeat([]byte{0, 255, 13, 10, 23}, 1<<15)
	_ = os.WriteFile(source, payload, 0600)
	scpDest := filepath.Join(root, "scp.bin")
	run("scp", "-F", path, "-o", "User="+current.Username, source, alias+":"+scpDest)
	if data, _ := os.ReadFile(scpDest); !bytes.Equal(data, payload) {
		t.Fatal("scp mismatch")
	}
	sftpDest := filepath.Join(root, "sftp.bin")
	batch := filepath.Join(root, "sftp.batch")
	_ = os.WriteFile(batch, []byte("put "+source+" "+sftpDest+"\n"), 0600)
	run("sftp", "-F", path, "-o", "User="+current.Username, "-b", batch, alias)
	if data, _ := os.ReadFile(sftpDest); !bytes.Equal(data, payload) {
		t.Fatal("sftp mismatch")
	}
	rsyncDest := filepath.Join(root, "rsync.bin")
	run("rsync", "-e", "ssh -F "+shellArg(path)+" -l "+shellArg(current.Username), source, alias+":"+rsyncDest)
	if data, _ := os.ReadFile(rsyncDest); !bytes.Equal(data, payload) {
		t.Fatal("rsync mismatch")
	}
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "forward-ok") }))
	defer service.Close()
	local, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	localAddr := local.Addr().String()
	_ = local.Close()
	forward := exec.Command("ssh", "-F", path, "-l", current.Username, "-N", "-L", localAddr+":"+strings.TrimPrefix(service.URL, "http://"), alias)
	forward.Env = env
	if e := forward.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() { _ = forward.Process.Kill(); _ = forward.Wait() }()
	deadline := time.Now().Add(5 * time.Second)
	ok := false
	client := http.Client{Timeout: 200 * time.Millisecond}
	for time.Now().Before(deadline) {
		res, e := client.Get("http://" + localAddr)
		if e == nil {
			data, _ := io.ReadAll(res.Body)
			_ = res.Body.Close()
			if string(data) == "forward-ok" {
				ok = true
				break
			}
		}
		time.Sleep(30 * time.Millisecond)
	}
	if !ok {
		t.Fatal("local forwarding did not deliver service body")
	}
	// Installed binary login, using the repository's local OIDC provider.
	provider := oidctest.New(t)
	env = append(env, "RZV_API_KEY=")
	login := exec.Command(binaryPath, "login", "--issuer", provider.Issuer(), "--api-base-url", f.server.URL, "--allow-insecure-loopback", "--no-browser", "--workspace", "Demo")
	login.Env = env
	stderr, e := login.StderrPipe()
	if e != nil {
		t.Fatal(e)
	}
	if e := login.Start(); e != nil {
		t.Fatal(e)
	}
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), provider.Issuer()) {
			if e := provider.Browser("good-code", false)(scanner.Text()); e != nil {
				t.Fatal(e)
			}
			break
		}
	}
	if e := login.Wait(); e != nil {
		t.Fatal("installed login", e)
	}
	loginConfig := run(binaryPath, "sandboxes", "ssh-config", sbxID)
	_ = os.WriteFile(path, loginConfig, 0600)
	if out := run("ssh", "-F", path, "-l", current.Username, alias, "printf login-ok"); string(out) != "login-ok" {
		t.Fatal(string(out))
	}
	// A fresh host key signed by the same CA survives a sandbox restart.
	restarted := filepath.Join(root, "restart")
	if e := os.Mkdir(restarted, 0700); e != nil {
		t.Fatal(e)
	}
	restartAddress := startTestSSHD(t, f, restarted)
	f.mu.Lock()
	f.serve = relayToSSHD(t, restartAddress)
	f.mu.Unlock()
	if out := run("ssh", "-F", path, "-l", current.Username, alias, "printf restarted"); string(out) != "restarted" {
		t.Fatal(string(out))
	}
	// A mismatched CA must prevent authentication and command execution.
	f.mu.Lock()
	f.hostCA = testSSHSigner(t)
	f.mu.Unlock()
	denied := exec.Command("ssh", "-F", path, "-l", current.Username, alias, "printf should-not-run")
	denied.Env = env
	deniedOut, deniedErr := denied.CombinedOutput()
	if deniedErr == nil || strings.Contains(string(deniedOut), "should-not-run") {
		t.Fatal("host CA mismatch did not fail closed")
	}

}

func TestSSHSandboxIDHostAliases(t *testing.T) {
	for _, host := range []string{sbxID, sbxID + ".rzv", sbxID + ".sandbox.runzivo.dev"} {
		if id, err := sshSandboxID(host); err != nil || id != sbxID {
			t.Fatalf("valid alias %q: id=%q err=%v", host, id, err)
		}
	}
	for _, host := range []string{sbxID + ".sandbox.runzivo", sbxID + ".sandbox.runzivo.dev.evil", sbxID + ".sandbox.runzivo.dev.rzv", sbxID + ".rzv.sandbox.runzivo.dev", "sbx_short.sandbox.runzivo.dev", "*.sandbox.runzivo.dev"} {
		if _, err := sshSandboxID(host); err == nil {
			t.Fatalf("invalid alias accepted: %q", host)
		}
	}
}

func TestSSHRejectsWrongIdentityAndInvalidHosts(t *testing.T) {
	f := newSSHFixture(t, func(net.Conn, *bufio.Reader) { t.Error("unexpected connection") })
	var out, stderr bytes.Buffer
	deps := fixtureDeps(f, t.TempDir(), strings.NewReader(""), &out, &stderr)
	for _, host := range []string{"-oProxyCommand=bad", "../file", sbxID + ".rzv.evil", sbxID + "\nHost *", sbxID + ".sandbox.runzivo", sbxID + ".sandbox.runzivo.dev.evil"} {
		if code := Run(context.Background(), []string{"sandboxes", "ssh-proxy", host}, deps); code != exitUsage {
			t.Fatal(code)
		}
	}
	if code := Run(context.Background(), []string{"sandboxes", "ssh-proxy", sbxID + ".rzv", "--binding", strings.Repeat("0", 64)}, deps); code != 255 {
		t.Fatal("identity binding accepted", code)
	}
	if out.Len() != 0 {
		t.Fatal("proxy emitted non-SSH output")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.issued != 0 || f.connections != 0 {
		t.Fatal("invalid input reached ticket API")
	}
}

func TestSSHTicketIsClaimedOnlyOnce(t *testing.T) {
	f := newSSHFixture(t, func(net.Conn, *bufio.Reader) {})
	deps := fixtureDeps(f, t.TempDir(), nil, io.Discard, io.Discard)
	deps.Now = time.Now
	a := &app{Deps: deps, ctx: context.Background()}
	cmd, _ := parse([]string{"sandboxes", "ssh-config", sbxID})
	state, client, e := a.sshState(cmd, sbxID)
	if e != nil {
		t.Fatal(e)
	}
	ticket, e := a.prepareSSH(state, client)
	if e != nil {
		t.Fatal(e)
	}
	data, _ := json.Marshal(ticket)
	if e := sshstate.WritePrivate(pendingTicketPath(state), data); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	claimed := make(chan bool, 8)
	for range 8 {
		wg.Go(func() { _, e := takeSSHTicket(state); claimed <- e == nil })
	}
	wg.Wait()
	close(claimed)
	count := 0
	for ok := range claimed {
		if ok {
			count++
		}
	}
	if count != 1 {
		t.Fatal("ticket claimed", count, "times")
	}
	paths, _ := filepath.Glob(filepath.Join(state.Dir, ".ticket-*"))
	if len(paths) != 0 {
		t.Fatal("ticket scratch files leaked")
	}
}

func TestSSHProxyServerHalfCloseStillAcceptsInput(t *testing.T) {
	received := make(chan string, 1)
	f := newSSHFixture(t, func(conn net.Conn, reader *bufio.Reader) {
		_, _ = conn.Write(sshFrame(2, []byte("banner")))
		_, _ = conn.Write(sshFrame(1, []byte(`{"type":"eof"}`)))
		var data []byte
		for {
			op, b, e := readClientFrame(reader)
			if e != nil {
				return
			}
			if op == 2 {
				data = append(data, b...)
			}
			if op == 1 {
				received <- string(data)
				_, _ = conn.Write(sshFrame(8, []byte{3, 232}))
				return
			}
		}
	})
	input, send := io.Pipe()
	output, receive := io.Pipe()
	defer send.Close()
	defer output.Close()
	var stderr bytes.Buffer
	done := make(chan int, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		done <- Run(ctx, []string{"sandboxes", "ssh-proxy", sbxID + ".rzv"}, fixtureDeps(f, t.TempDir(), input, receive, &stderr))
	}()
	banner, e := io.ReadAll(output)
	if e != nil || string(banner) != "banner" {
		t.Fatal(string(banner), e)
	}
	if _, e := send.Write([]byte("late-input")); e != nil {
		t.Fatal(e)
	}
	_ = send.Close()
	if code := <-done; code != 0 {
		t.Fatal(code, stderr.String())
	}
	if got := <-received; got != "late-input" {
		t.Fatal(got)
	}
}

func TestSSHUsesTerminalWhenOpenSSHIsMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	h := selectedHarness(t)
	h.json("GET /v1/customer/tenants/t-1/sandboxes/"+sbxID, 200, sandboxJSON)
	terminalRoute(t, h, func(conn net.Conn, reader *bufio.Reader, _ http.Header) {
		_, _, _ = readClientFrame(reader)
		_, _ = conn.Write(serverFrame(2, []byte("PTY fallback")))
		_, _ = conn.Write(serverFrame(8, []byte{3, 232}))
	})
	terminal := &fakeTerminal{interactive: true, cols: 80, rows: 24}
	if code, out, e := h.runWithTerminal(terminal, strings.NewReader(""), "sandboxes", "ssh", sbxID); code != 0 || out != "PTY fallback" {
		t.Fatal(code, out, e)
	}
}
