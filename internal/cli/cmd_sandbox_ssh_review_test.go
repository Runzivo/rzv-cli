package cli

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Runzivo/rzv-cli/internal/api"
	"golang.org/x/crypto/ssh"
)

func TestSSHUnavailableFallbackAndGuidance(t *testing.T) {
	if _, err := exec.LookPath("ssh"); err != nil {
		t.Skip("OpenSSH unavailable")
	}
	for _, status := range []int{404, 503} {
		for _, locale := range []string{"en", "fr_CA"} {
			t.Run(fmt.Sprintf("%d/%s", status, locale), func(t *testing.T) {
				h := selectedHarness(t)
				h.env["LC_MESSAGES"] = locale
				code := "not_found"
				if status == 503 {
					code = "sandbox_ssh_unavailable"
				}
				h.json("POST /v1/customer/tenants/t-1/sandboxes/"+sbxID+"/ssh-ticket", status, `{"error":{"code":"`+code+`"}}`)
				h.json("GET /v1/customer/tenants/t-1/sandboxes/"+sbxID+"/ssh-host", status, `{"error":{"code":"`+code+`"}}`)
				h.json("GET /v1/customer/tenants/t-1/sandboxes/"+sbxID, 200, sandboxJSON)
				terminalRoute(t, h, func(conn net.Conn, reader *bufio.Reader, _ http.Header) {
					_, _, _ = readClientFrame(reader)
					_, _ = conn.Write(serverFrame(2, []byte("PTY fallback")))
					_, _ = conn.Write(serverFrame(8, []byte{3, 232}))
				})
				terminal := &fakeTerminal{interactive: true, cols: 80, rows: 24}
				got, out, stderr := h.runWithTerminal(terminal, strings.NewReader(""), "sandboxes", "ssh", sbxID)
				if got != 0 || out != "PTY fallback" || !strings.Contains(stderr, "PTY") || strings.Count(stderr, "\n") != 1 {
					t.Fatal(got, out, stderr)
				}
				for _, args := range [][]string{
					{"sandboxes", "ssh", sbxID, "--", "echo hello"},
					{"sandboxes", "ssh", sbxID, "--stdio"},
					{"sandboxes", "ssh-proxy", sbxID},
					{"sandboxes", "ssh-config", sbxID},
					{"sandboxes", "ssh-config", sbxID, "--refresh"},
					{"sandboxes", "ssh", sbxID}, // noninteractive must not open a PTY
				} {
					got, out, stderr := h.run(args...)
					if got != exitUnavailable || out != "" || !strings.Contains(stderr, "rzv sandboxes terminal") || !strings.Contains(stderr, "rzv sandboxes exec") {
						t.Fatal(args, got, out, stderr)
					}
				}
			})
		}
	}
}

func TestSSHDoesNotFallbackForOtherFailures(t *testing.T) {
	a := &app{Deps: Deps{Getenv: func(string) string { return "" }}}
	for _, e := range []*api.Error{{Status: 404, Code: "sandbox_not_found"}, {Status: 403, Code: "permission_required"}, {Status: 503, Code: "sandboxes_unavailable"}} {
		err := a.sshAPIError(e)
		if strings.Contains(err.Error(), "Standard SSH is not enabled") {
			t.Fatal(err)
		}
	}
	for _, e := range []*api.Error{{Status: 409, Code: "session_limit_reached"}, {Status: 429, Code: "ticket_limit_reached"}, {Status: 200, Code: "invalid_ssh_ticket"}} {
		err := a.sshAPIError(e)
		if !strings.Contains(err.Error(), "SSH") || strings.Contains(err.Error(), "terminal") || strings.Contains(err.Error(), "malformed_response") {
			t.Fatal(err)
		}
	}
	if _, _, err := a.splitSSHArguments([]string{"-l", "root"}); err == nil {
		t.Fatal("accepted username override")
	}
}

func TestWriteSSHConfigPreservesBodyModeAndInclude(t *testing.T) {
	for _, body := range []string{"Host personal\n  User me\n", "# comment\r\nInclude ~/.ssh/rzv/*.conf\r\nHost personal\r\n  User me\r\n"} {
		for _, mode := range []os.FileMode{0600, 0640, 0644} {
			home := t.TempDir()
			dir := filepath.Join(home, ".ssh")
			if e := os.Mkdir(dir, 0700); e != nil {
				t.Fatal(e)
			}
			path := filepath.Join(dir, "config")
			if e := os.WriteFile(path, []byte(body), mode); e != nil {
				t.Fatal(e)
			}
			if e := os.Chmod(path, mode); e != nil {
				t.Fatal(e)
			}
			a := &app{Deps: Deps{Stdout: io.Discard, Getenv: func(string) string { return "" }}}
			for range 2 {
				if e := a.writeSSHConfigAtHome(home, sbxID, "# Managed by rzv.\nHost "+sbxID+".rzv\n"); e != nil {
					t.Fatal(e)
				}
			}
			got, _ := os.ReadFile(path)
			info, _ := os.Stat(path)
			want := body
			if !strings.Contains(body, "Include") {
				want = "Include ~/.ssh/rzv/*.conf\n" + body
			}
			if string(got) != want || info.Mode().Perm() != mode {
				t.Fatal(string(got), info.Mode())
			}
		}
	}
}

func TestWriteSSHConfigRefusesUnownedAndSymlink(t *testing.T) {
	for _, symlink := range []bool{false, true} {
		home := t.TempDir()
		dir := filepath.Join(home, ".ssh", "rzv")
		if e := os.MkdirAll(dir, 0700); e != nil {
			t.Fatal(e)
		}
		path := filepath.Join(dir, sbxID+".conf")
		original := "# user managed\nHost example\n"
		target := path
		if symlink {
			target = filepath.Join(home, "managed-config")
			if e := os.Symlink(target, filepath.Join(home, ".ssh", "config")); e != nil {
				t.Skip(e)
			}
		}
		if e := os.WriteFile(target, []byte(original), 0600); e != nil {
			t.Fatal(e)
		}
		a := &app{Deps: Deps{Stdout: io.Discard, Getenv: func(string) string { return "" }}}
		err := a.writeSSHConfigAtHome(home, sbxID, "# Managed by rzv.\n")
		if err == nil || (symlink && !strings.Contains(err.Error(), "symlink")) || (!symlink && !strings.Contains(err.Error(), "not owned")) {
			t.Fatal(err)
		}
		got, _ := os.ReadFile(target)
		if string(got) != original {
			t.Fatal("overwrote user configuration")
		}
		if symlink {
			info, _ := os.Lstat(filepath.Join(home, ".ssh", "config"))
			if info.Mode()&os.ModeSymlink == 0 {
				t.Fatal("replaced symlink")
			}
		}
	}
}

func TestSSHNormalLifecycleCloses(t *testing.T) {
	for _, reason := range []string{"idle_timeout", "sandbox_stopped", "sandbox_expired", "sandbox_deleted"} {
		t.Run(reason, func(t *testing.T) {
			f := newSSHFixture(t, func(c net.Conn, _ *bufio.Reader) {
				_, _ = c.Write(sshFrame(8, append([]byte{3, 232}, []byte(reason)...)))
			})
			input, writer := io.Pipe()
			defer writer.Close()
			var output, stderr bytes.Buffer
			code := Run(context.Background(), []string{"sandboxes", "ssh-proxy", sbxID}, fixtureDeps(f, t.TempDir(), input, &output, &stderr))
			if code != 0 || output.Len() != 0 || !strings.Contains(stderr.String(), reason) {
				t.Fatal(code, output.String(), stderr.String())
			}
		})
	}
}

func TestSSHConfigListResolvesMembershipOnce(t *testing.T) {
	h := selectedHarness(t)
	h.json("GET /v1/customer/tenants/t-1/sandboxes", 200, `{"sandboxes":[`+sandboxJSON+`,`+strings.ReplaceAll(sandboxJSON, sbxID, "sbx_aaaaaaaaaaaaaaaaaaaaaaaa")+`],"max_running":2,"sizes":["2vcpu-8gb"]}`)
	f := newSSHFixture(t, nil)
	ca := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(f.hostCA.PublicKey())))
	for _, id := range []string{sbxID, "sbx_aaaaaaaaaaaaaaaaaaaaaaaa"} {
		h.json("GET /v1/customer/tenants/t-1/sandboxes/"+id+"/ssh-host", 200, `{"host_ca_public_key":"`+ca+`","host_key_alias":"`+id+`.sandbox.runzivo"}`)
	}
	original := h.routes["GET /v1/customer/memberships"]
	count := 0
	h.routes["GET /v1/customer/memberships"] = func(w http.ResponseWriter, r *http.Request) { count++; original(w, r) }
	code, out, stderr := h.run("sandboxes", "ssh-config")
	if code != 0 || count != 1 || strings.Count(out, "ProxyCommand") != 2 || strings.Contains(out, "%h") {
		t.Fatal(code, count, out, stderr)
	}
}

func TestSSHArgumentsKeepLeadingOptionsInRemoteCommand(t *testing.T) {
	a := &app{Deps: Deps{Getenv: func(string) string { return "" }}}
	options, remote, err := a.splitSSHArguments([]string{"-T", "--", "-oProxyCommand=local-command"})
	if err != nil || len(options) != 1 || options[0] != "-T" || len(remote) != 1 || remote[0] != "-oProxyCommand=local-command" {
		t.Fatal(options, remote, err)
	}
}

func TestWorkspaceHelpExplainsAPIKeyIDs(t *testing.T) {
	for _, locale := range []string{"en", "fr_CA"} {
		h := newHarness(t)
		h.env["LC_MESSAGES"] = locale
		want := "API keys require an exact workspace id"
		if locale == "fr_CA" {
			want = "Les clés API exigent un identifiant exact"
		}
		for _, args := range [][]string{{"workspaces", "--help"}, {"accounts", "--help"}, {"workspaces"}} {
			code, out, stderr := h.run(args...)
			if (code != 0 && code != exitUsage) || !strings.Contains(out+stderr, want) {
				t.Fatal(code, out, stderr)
			}
		}
	}
}
