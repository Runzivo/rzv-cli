package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Runzivo/rzv-cli/internal/api"
	"golang.org/x/crypto/ssh"
)

const (
	orcaInstance = "orca-4f1d2c3b-aaaa-bbbb-cccc-0123456789ab"
	orcaCommit   = "0123456789abcdef0123456789abcdef01234567"
	orcaBase     = "/v1/customer/tenants/t-1/sandboxes"
)

// orcaFake records the local programs rzv orca runs and answers git and ssh.
type orcaFake struct {
	mu      sync.Mutex
	calls   []string
	scripts []string
	bundles [][]byte
	sshFail func(attempt int) error
	sshRuns int
}

func (f *orcaFake) install(t *testing.T) {
	t.Helper()
	saved, savedRetry, savedPoll := orcaRun, orcaSSHRetry, orcaPollEvery
	orcaSSHRetry, orcaPollEvery = time.Millisecond, time.Millisecond
	t.Cleanup(func() { orcaRun, orcaSSHRetry, orcaPollEvery = saved, savedRetry, savedPoll })
	orcaRun = func(_ context.Context, _ string, stdin io.Reader, stdout, stderr io.Writer, name string, args ...string) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.calls = append(f.calls, name+" "+strings.Join(args, " "))
		if name == "ssh" {
			f.sshRuns++
			f.scripts = append(f.scripts, args[len(args)-1])
			if stdin != nil {
				data, _ := io.ReadAll(stdin)
				f.bundles = append(f.bundles, data)
			}
			if f.sshFail != nil {
				return f.sshFail(f.sshRuns)
			}
			return nil
		}
		switch args[0] {
		case "remote":
			if len(args) == 1 {
				_, _ = io.WriteString(stdout, "origin\n")
			} else {
				_, _ = io.WriteString(stdout, "https://user:secret-token@github.com/acme/widgets.git\n")
			}
		case "config":
			_, _ = io.WriteString(stdout, map[string]string{"user.name": "Dev Person", "user.email": "dev@example.com"}[args[1]]+"\n")
		case "rev-parse":
			_, _ = io.WriteString(stdout, orcaCommit+"\n")
		case "bundle":
			if err := os.WriteFile(args[3], []byte("bundle-bytes"), 0o600); err != nil {
				return err
			}
		}
		return nil
	}
}

func (f *orcaFake) called(prefix string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

type orcaCore struct {
	mu       sync.Mutex
	statuses []string
	created  []api.CreateSandboxInput
	deleted  int
	started  int
	stopped  int
	synced   int
	gets     int
}

func orcaHarness(t *testing.T) (*harness, *orcaCore, *orcaFake) {
	t.Helper()
	h := selectedHarness(t)
	home := t.TempDir()
	h.env["HOME"] = home
	h.env["ORCA_RECIPE_RESULT_SCHEMA_VERSION"] = "2"
	h.env["ORCA_VM_INSTANCE_ID"] = orcaInstance
	h.env["ORCA_WORKSPACE_NAME"] = "Fix Login Bug"
	h.env["ORCA_REPO_PATH"] = t.TempDir()
	h.env["ORCA_REPO_REF_HEAD"] = orcaCommit
	h.env["ORCA_REPO_BRANCH"] = "fix-login"
	h.env["ORCA_REPO_URL"] = "https://github.com/acme/widgets.git"
	core := &orcaCore{statuses: []string{"starting", "running"}}
	signer := testSSHSigner(t)
	hostJSON, _ := json.Marshal(api.SSHHost{HostCAPublicKey: strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey()))), HostKeyAlias: sbxID + ".sandbox.runzivo.dev"})
	sandbox := func(status string) string {
		s := strings.Replace(sandboxWith(status), `"name":"dev"`, `"name":"orca-fix-login-bug-4f1d2c3b"`, 1)
		return strings.Replace(s, `"credits_used":20.5`, `"credits_used":20.5,"lifetime_hours":24`, 1)
	}
	h.routes["GET "+orcaBase] = func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"sandboxes":[`+sandbox("running")+`,`+strings.Replace(sandboxWith("stopped"), `"id":"sbx_0123456789abcdef01234567"`, `"id":"sbx_bbbbbbbbbbbbbbbbbbbbbbbb"`, 1)+`],"max_running":2,"sizes":["2vcpu-8gb","4vcpu-16gb"],"lifetimes":[8,12,24],"ssh_enabled":true}`)
	}
	h.routes["POST "+orcaBase] = func(w http.ResponseWriter, r *http.Request) {
		var in api.CreateSandboxInput
		_ = json.NewDecoder(r.Body).Decode(&in)
		core.mu.Lock()
		core.created = append(core.created, in)
		core.mu.Unlock()
		w.WriteHeader(201)
		_, _ = io.WriteString(w, sandbox("pending"))
	}
	h.routes["GET "+orcaBase+"/"+sbxID] = func(w http.ResponseWriter, _ *http.Request) {
		core.mu.Lock()
		status := core.statuses[min(core.gets, len(core.statuses)-1)]
		core.gets++
		core.mu.Unlock()
		_, _ = io.WriteString(w, sandbox(status))
	}
	h.routes["DELETE "+orcaBase+"/"+sbxID] = func(w http.ResponseWriter, _ *http.Request) {
		core.mu.Lock()
		core.deleted++
		core.mu.Unlock()
		_, _ = io.WriteString(w, sandbox("deleting"))
	}
	h.routes["POST "+orcaBase+"/"+sbxID+"/start"] = func(w http.ResponseWriter, _ *http.Request) {
		core.mu.Lock()
		core.started++
		core.mu.Unlock()
		_, _ = io.WriteString(w, sandbox("starting"))
	}
	h.routes["POST "+orcaBase+"/"+sbxID+"/stop"] = func(w http.ResponseWriter, _ *http.Request) {
		core.mu.Lock()
		core.stopped++
		core.mu.Unlock()
		_, _ = io.WriteString(w, sandbox("stopping"))
	}
	h.routes["POST "+orcaBase+"/"+sbxID+"/exec"] = func(w http.ResponseWriter, r *http.Request) {
		core.mu.Lock()
		if core.stopped == 0 && strings.Contains(readBody(r), `"cmd":"sync"`) {
			core.synced++
		}
		core.mu.Unlock()
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w, `{"type":"exit","exit_code":0,"timed_out":false}`+"\n")
	}
	h.routes["GET "+orcaBase+"/"+sbxID+"/ssh-host"] = func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(hostJSON)
	}
	fake := &orcaFake{}
	fake.install(t)
	return h, core, fake
}

func orcaPayloadJSON(root string) string {
	return `{"schemaVersion":1,"mode":"destroy","recipeId":"runzivo-2vcpu","instanceId":"` + orcaInstance + `","recipeResult":{"schemaVersion":2,"checkoutMode":"provisioned-root","connection":{"type":"ssh","projectRoot":"` + root + `","target":{"label":"x","configHost":"` + sbxID + `.rzv","host":"` + sbxID + `.rzv","port":22,"username":"user"}},"userData":{"provider":"runzivo","sandboxId":"` + sbxID + `","workspaceId":"t-1","profile":"default"}}}`
}

func TestOrcaCreatePrintsRecipeResult(t *testing.T) {
	h, core, fake := orcaHarness(t)
	code, out, errOut := h.run("orca", "create", "--size", "4vcpu-16gb", "--lifetime", "24h")
	if code != 0 {
		t.Fatalf("create exit %d: %s", code, errOut)
	}
	var result orcaResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("stdout is not one JSON object: %q", out)
	}
	if result.SchemaVersion != 2 || result.CheckoutMode != "provisioned-root" || result.Connection.Type != "ssh" ||
		result.Connection.ProjectRoot != "/workspace/widgets" || result.Connection.Target.ConfigHost != sbxID+".rzv" ||
		result.Connection.Target.Username != "user" || result.UserData.SandboxID != sbxID || result.UserData.WorkspaceID != "t-1" || result.UserData.Provider != "runzivo" {
		t.Fatalf("result: %+v", result)
	}
	if len(core.created) != 1 {
		t.Fatalf("created %d sandboxes", len(core.created))
	}
	in := core.created[0]
	if in.ClientOperationID != orcaInstance || in.Size != "4vcpu-16gb" || in.LifetimeHours != 24 || in.Name != "orca-fix-login-bug-4f1d2c3b" {
		t.Fatalf("create input: %+v", in)
	}
	conf, err := os.ReadFile(filepath.Join(h.env["HOME"], ".ssh", "rzv", sbxID+".conf"))
	if err != nil || !strings.Contains(string(conf), "StrictHostKeyChecking yes") || !strings.Contains(string(conf), "--stdio") {
		t.Fatalf("ssh config: %v %q", err, conf)
	}
	if len(fake.scripts) != 1 || string(fake.bundles[0]) != "bundle-bytes" {
		t.Fatalf("ssh runs: %d", len(fake.scripts))
	}
	script := fake.scripts[0]
	for _, want := range []string{"branch='fix-login'", "git checkout --quiet -B \"$branch\" \"$commit\"", "git remote add 'origin' 'https://github.com/acme/widgets.git'", "git config user.email 'dev@example.com'", "root='/workspace/widgets'"} {
		if !strings.Contains(script, want) {
			t.Errorf("script lacks %q:\n%s", want, script)
		}
	}
	if strings.Contains(script, "secret-token") || strings.Contains(out, "secret-token") || strings.Contains(errOut, "secret-token") {
		t.Fatal("a remote credential reached the sandbox or the output")
	}
	if !fake.called("git update-ref -d refs/rzv-orca/") {
		t.Fatal("temporary bundle ref was not removed")
	}
	if !strings.Contains(errOut, "Expires") || !strings.Contains(errOut, "60 equivalent minutes per hour") {
		t.Fatalf("summary: %q", errOut)
	}
	if core.deleted != 0 {
		t.Fatal("a successful create deleted its sandbox")
	}
}

func TestOrcaCreateRetriesSSHAndCleansUpOnFailure(t *testing.T) {
	h, core, fake := orcaHarness(t)
	fake.sshFail = func(attempt int) error {
		if attempt < 3 {
			return exitStatus(t, 255)
		}
		return nil
	}
	if code, _, errOut := h.run("orca", "create"); code != 0 || fake.sshRuns != 3 {
		t.Fatalf("retry: %d after %d ssh runs: %s", code, fake.sshRuns, errOut)
	}
	if string(fake.bundles[2]) != "bundle-bytes" {
		t.Fatal("a retry did not resend the whole bundle")
	}
	if core.created[0].LifetimeHours != 0 || core.created[0].Size != "2vcpu-8gb" {
		t.Fatalf("defaults: %+v", core.created[0])
	}

	fake.sshRuns = 0
	fake.sshFail = func(int) error { return exitStatus(t, 4) }
	code, out, errOut := h.run("orca", "create")
	if code == 0 || out != "" || fake.sshRuns != 1 {
		t.Fatalf("failed create: %d %q after %d runs", code, out, fake.sshRuns)
	}
	if core.deleted != 1 || !strings.Contains(errOut, "Deleted sandbox "+sbxID+" after the failed create") {
		t.Fatalf("cleanup: deleted %d, %q", core.deleted, errOut)
	}
	if _, err := os.Stat(filepath.Join(h.env["HOME"], ".ssh", "rzv", sbxID+".conf")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("ssh config left behind")
	}
}

func TestOrcaCreateFailsOnSandboxFailure(t *testing.T) {
	h, core, _ := orcaHarness(t)
	core.statuses = []string{"failed"}
	if code, out, errOut := h.run("orca", "create"); code == 0 || out != "" || core.deleted != 1 || !strings.Contains(errOut, "failed") {
		t.Fatalf("failed sandbox: %d %q %q deleted %d", code, out, errOut, core.deleted)
	}
}

func TestOrcaCreateRefusals(t *testing.T) {
	h, core, _ := orcaHarness(t)
	h.env["ORCA_RECIPE_RESULT_SCHEMA_VERSION"] = "1"
	if code, _, errOut := h.run("orca", "create"); code != exitUsage || !strings.Contains(errOut, "schema") {
		t.Fatalf("schema 1: %d %q", code, errOut)
	}
	h.env["ORCA_RECIPE_RESULT_SCHEMA_VERSION"] = "2"
	h.env["ORCA_VM_INSTANCE_ID"] = ""
	if code, _, _ := h.run("orca", "create"); code != exitUsage {
		t.Fatalf("no instance: %d", code)
	}
	h.env["ORCA_VM_INSTANCE_ID"] = orcaInstance
	if code, _, errOut := h.run("orca", "create", "--size", "8vcpu-32gb"); code == 0 || !strings.Contains(errOut, "not on offer") {
		t.Fatalf("unavailable size: %d %q", code, errOut)
	}
	if code, _, _ := h.run("orca", "create", "--lifetime", "48h"); code != exitUsage {
		t.Fatalf("48h: %d", code)
	}
	if code, _, _ := h.run("orca", "create", "--json"); code != exitUsage {
		t.Fatalf("--json: %d", code)
	}
	h.env["ORCA_REPO_REF_HEAD"] = "main"
	if code, _, _ := h.run("orca", "create"); code == 0 {
		t.Fatal("a symbolic commit was accepted")
	}
	if len(core.created) != 0 {
		t.Fatal("a refused create reached Core")
	}
}

func TestOrcaDefaultsApplyToCreate(t *testing.T) {
	h, core, _ := orcaHarness(t)
	if code, out, errOut := h.run("orca", "defaults", "--disk-gib", "40", "--lifetime", "12h", "--size", "4vcpu-16gb"); code != 0 || !strings.Contains(out, "12h") || !strings.Contains(out, "40 GiB") {
		t.Fatalf("defaults: %d %q %q", code, out, errOut)
	}
	if code, _, errOut := h.run("orca", "create"); code != 0 {
		t.Fatal(errOut)
	}
	if in := core.created[0]; in.DiskGiB != 40 || in.LifetimeHours != 12 || in.Size != "4vcpu-16gb" {
		t.Fatalf("create used %+v", in)
	}
	if code, _, _ := h.run("orca", "create", "--size", "2vcpu-8gb", "--lifetime", "8h"); code != 0 {
		t.Fatal("override")
	}
	if in := core.created[1]; in.Size != "2vcpu-8gb" || in.LifetimeHours != 0 || in.DiskGiB != 40 {
		t.Fatalf("flags did not win: %+v", in)
	}
	if code, out, _ := h.run("orca", "defaults", "--reset", "--json"); code != 0 || !strings.Contains(out, `"defaults":{}`) {
		t.Fatalf("reset: %q", out)
	}
	for _, args := range [][]string{{"orca", "defaults", "--disk-gib", "100"}, {"orca", "defaults", "--size", "1vcpu"}, {"orca"}, {"orca", "bogus"}} {
		if code, _, _ := h.run(args...); code != exitUsage {
			t.Errorf("%v: %d", args, code)
		}
	}
}

func TestOrcaLifecycle(t *testing.T) {
	h, core, fake := orcaHarness(t)
	conf := filepath.Join(h.env["HOME"], ".ssh", "rzv", sbxID+".conf")

	core.statuses = []string{"running", "stopped"}
	code, out, errOut := h.runWithTerminal(nil, strings.NewReader(orcaPayloadJSON("/workspace/widgets")), "orca", "suspend")
	if code != 0 || out != "" || core.stopped != 1 || core.synced != 1 || !strings.Contains(errOut, "compute is not charged") || strings.Contains(errOut, "Could not flush") {
		t.Fatalf("suspend: %d %q %q stopped %d synced %d", code, out, errOut, core.stopped, core.synced)
	}

	core.gets, core.statuses = 0, []string{"stopped", "starting", "running"}
	code, out, errOut = h.runWithTerminal(nil, strings.NewReader(orcaPayloadJSON("/workspace/widgets")), "orca", "resume")
	var result orcaResult
	if code != 0 || core.started != 1 || json.Unmarshal([]byte(out), &result) != nil || result.Connection.ProjectRoot != "/workspace/widgets" {
		t.Fatalf("resume: %d %q %q", code, out, errOut)
	}
	if _, err := os.Stat(conf); err != nil {
		t.Fatal("resume did not write the ssh config")
	}
	if last := fake.scripts[len(fake.scripts)-1]; last != "test -d '/workspace/widgets/.git'" {
		t.Fatalf("resume check: %q", last)
	}

	core.gets, core.statuses = 0, []string{"expired"}
	if code, out, errOut := h.runWithTerminal(nil, strings.NewReader(orcaPayloadJSON("/workspace/widgets")), "orca", "resume"); code == 0 || out != "" || !strings.Contains(errOut, "lifetime") {
		t.Fatalf("expired resume: %d %q %q", code, out, errOut)
	}
	if code, _, _ := h.runWithTerminal(nil, strings.NewReader(orcaPayloadJSON("/etc")), "orca", "resume"); code != exitUsage {
		t.Fatal("a project root outside /workspace was accepted")
	}

	code, out, errOut = h.runWithTerminal(nil, strings.NewReader(orcaPayloadJSON("/workspace/widgets")), "orca", "destroy")
	if code != 0 || out != "" || core.deleted != 1 || !strings.Contains(errOut, "Work you did not push is gone") {
		t.Fatalf("destroy: %d %q %q", code, out, errOut)
	}
	if _, err := os.Stat(conf); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("destroy left the ssh config")
	}
	h.json("DELETE "+orcaBase+"/"+sbxID, 404, `{"error":{"code":"sandbox_not_found","message":"gone"}}`)
	if code, _, errOut := h.runWithTerminal(nil, strings.NewReader(orcaPayloadJSON("/workspace/widgets")), "orca", "destroy"); code != 0 {
		t.Fatalf("destroy of a gone sandbox: %d %q", code, errOut)
	}

	other := strings.Replace(orcaPayloadJSON("/workspace/widgets"), `"provider":"runzivo"`, `"provider":"lima"`, 1)
	if code, _, _ := h.runWithTerminal(nil, strings.NewReader(other), "orca", "destroy"); code != exitUsage {
		t.Fatal("another provider's workspace was accepted")
	}
	if code, _, _ := h.runWithTerminal(nil, strings.NewReader("not json"), "orca", "destroy"); code != exitUsage {
		t.Fatal("bad payload accepted")
	}
}

func TestOrcaStatusListsOnlyOrcaSandboxes(t *testing.T) {
	h, _, _ := orcaHarness(t)
	code, out, _ := h.run("orca", "status", "--json")
	var listed struct {
		Sandboxes []api.Sandbox `json:"sandboxes"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &listed) != nil || len(listed.Sandboxes) != 1 || listed.Sandboxes[0].ID != sbxID {
		t.Fatalf("status: %d %q", code, out)
	}
}

func TestOrcaNamesAndURLs(t *testing.T) {
	cases := map[string]string{
		orcaSandboxName("Fix Login Bug", orcaInstance):                                     "orca-fix-login-bug-4f1d2c3b",
		orcaSandboxName("", "orca-x"):                                                      "orca-workspace-x",
		orcaSandboxName(strings.Repeat("a", 100), orcaInstance):                            "orca-" + strings.Repeat("a", 50) + "-4f1d2c3b",
		orcaRepoName("", [][2]string{{"origin", "git@github.com:acme/my.repo.git"}}, "/x"): "my.repo",
		orcaRepoName("", nil, "/home/me/project with space"):                               "project-with-space",
		orcaRepoName("", nil, "/"):                                                         "repo",
		orcaRepoName("", nil, "/src/foo..bar"):                                             "foo.bar",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
	if n := orcaSandboxName(strings.Repeat("ab ", 60), orcaInstance); len(n) > 80 {
		t.Errorf("name too long: %d", len(n))
	}
	for raw, want := range map[string]string{
		"https://u:p@github.com/a/b.git": "https://github.com/a/b.git",
		"git@github.com:a/b.git":         "git@github.com:a/b.git",
		"HTTPS://tok@github.com/a/b":     "https://github.com/a/b",
	} {
		if got, ok := stripURLCredentials(raw); !ok || got != want {
			t.Errorf("%s: got %q %v", raw, got, ok)
		}
	}
	if got, ok := stripURLCredentials("https://user:bad%zztoken@github.com/a/b"); ok || got != "" {
		t.Errorf("an unparsable URL with user info was kept: %q", got)
	}
}

func TestOrcaCreateCancelledDeletesItsSandbox(t *testing.T) {
	h, core, _ := orcaHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	create := h.routes["POST "+orcaBase]
	h.routes["POST "+orcaBase] = func(w http.ResponseWriter, r *http.Request) {
		create(w, r)
		cancel() // Orca's SIGTERM arrives while the sandbox starts.
	}
	core.statuses = []string{"starting"}
	var stdout, stderr bytes.Buffer
	code := Run(ctx, []string{"orca", "create"}, Deps{Stdout: &stdout, Stderr: &stderr, Store: h.store, Getenv: func(k string) string { return h.env[k] }})
	if code == 0 || stdout.Len() != 0 || core.deleted != 1 || !strings.Contains(stderr.String(), "rzv: Cancelled.") {
		t.Fatalf("cancelled create: %d %q deleted %d: %s", code, stdout.String(), core.deleted, stderr.String())
	}
}

func TestOrcaPayloadRejectsBadProfile(t *testing.T) {
	h, core, _ := orcaHarness(t)
	bad := strings.Replace(orcaPayloadJSON("/workspace/widgets"), `"profile":"default"`, `"profile":"../../x"`, 1)
	if code, _, _ := h.runWithTerminal(nil, strings.NewReader(bad), "orca", "destroy"); code != exitUsage || core.deleted != 0 {
		t.Fatalf("bad profile: %d deleted %d", code, core.deleted)
	}
}

// TestOrcaCheckoutScriptRuns runs the remote checkout script with real git
// and sh, with a local directory standing in for the sandbox.
func TestOrcaCheckoutScriptRuns(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repo := t.TempDir()
	git := func(args ...string) string {
		out, err := exec.Command("git", append([]string{"-C", repo, "-c", "user.name=T", "-c", "user.email=t@example.com"}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q", "-b", "main")
	git("commit", "-q", "--allow-empty", "-m", "one")
	first := git("rev-parse", "HEAD")
	git("commit", "-q", "--allow-empty", "-m", "two")
	second := git("rev-parse", "HEAD")
	git("remote", "add", "origin", "https://example.com/acme/widgets.git")

	saved := orcaRun
	t.Cleanup(func() { orcaRun = saved })
	orcaRun = func(ctx context.Context, dir string, stdin io.Reader, stdout, stderr io.Writer, name string, args ...string) error {
		if name == "ssh" {
			name, args = "sh", []string{"-c", args[len(args)-1]}
		}
		return saved(ctx, dir, stdin, stdout, stderr, name, args...)
	}
	var log bytes.Buffer
	a := &app{Deps: Deps{Stdout: &log, Stderr: &log, Getenv: func(string) string { return "" }}, ctx: context.Background()}
	root := filepath.Join(t.TempDir(), "widgets")
	src := &orcaSourceInfo{repoPath: repo, commit: first, branch: "feature/x", remotes: [][2]string{{"origin", "https://example.com/acme/widgets.git"}}, userName: "It's me", userEmail: "me@example.com"}
	for range 2 { // the second run is a retried create: it must change nothing
		if err := a.orcaPushSource(sbxID, root, src); err != nil {
			t.Fatalf("push: %v\n%s", err, log.String())
		}
	}
	read := func(args ...string) string {
		out, err := exec.Command("git", append([]string{"-C", root}, args...)...).Output()
		if err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
		return strings.TrimSpace(string(out))
	}
	if read("rev-parse", "HEAD") != first || read("branch", "--show-current") != "feature/x" || read("remote", "get-url", "origin") != "https://example.com/acme/widgets.git" || read("config", "user.name") != "It's me" {
		t.Fatal("checkout is wrong")
	}
	if refs := read("for-each-ref", "refs/rzv-orca"); refs != "" {
		t.Fatalf("temporary refs left in the sandbox: %s", refs)
	}
	if out, _ := exec.Command("git", "-C", repo, "for-each-ref", "refs/rzv-orca").Output(); len(bytes.TrimSpace(out)) != 0 {
		t.Fatalf("temporary refs left locally: %s", out)
	}
	src.commit = second
	if err := a.orcaPushSource(sbxID, root, src); err == nil || !strings.Contains(err.Error(), "another checkout") {
		t.Fatalf("a different commit over an existing checkout: %v", err)
	}
}

// exitStatus returns a real *exec.ExitError with the given status.
func exitStatus(t *testing.T, status int) error {
	t.Helper()
	err := exec.Command("sh", "-c", "exit "+strconv.Itoa(status)).Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("no exit error: %v", err)
	}
	return err
}

func readBody(r *http.Request) string {
	data, _ := io.ReadAll(r.Body)
	return string(data)
}
