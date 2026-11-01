package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Runzivo/rzv-cli/internal/config"
	"github.com/Runzivo/rzv-cli/internal/oidctest"
)

// harness runs rzv against a fake identity provider and a fake customer API.
type harness struct {
	t        *testing.T
	provider *oidctest.Provider
	api      *httptest.Server
	routes   map[string]http.HandlerFunc
	store    *config.Store
	env      map[string]string
	seenAuth []string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{t: t, provider: oidctest.New(t), routes: map[string]http.HandlerFunc{}, env: map[string]string{}}
	h.store = &config.Store{Dir: t.TempDir()}
	h.api = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.seenAuth = append(h.seenAuth, r.Header.Get("Authorization"))
		route, ok := h.routes[r.Method+" "+r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":"not_found"}}`))
			return
		}
		route(w, r)
	}))
	t.Cleanup(h.api.Close)
	h.json("GET /v1/customer/memberships", 200, `{"memberships":[
		{"tenant_id":"t-1","tenant_slug":"acme","tenant_name":"Acme","role":"owner"},
		{"tenant_id":"t-2","tenant_slug":"beta","tenant_name":"Beta\u001b[31m","role":"member"}]}`)
	return h
}

func (h *harness) json(route string, status int, body string) {
	h.routes[route] = func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

func (h *harness) run(args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), args, Deps{
		Stdout: &stdout, Stderr: &stderr, Store: h.store,
		Getenv:       func(k string) string { return h.env[k] },
		OpenBrowser:  h.openBrowser,
		CallbackPort: -1, LoginTimeout: 5 * time.Second,
	})
	return code, stdout.String(), stderr.String()
}

// openBrowser completes sign-in for the fake provider and ignores any other
// URL (such as GitHub's device page).
func (h *harness) openBrowser(u string) error {
	if strings.HasPrefix(u, h.provider.Issuer()) {
		return h.provider.Browser("good-code", false)(u)
	}
	return nil
}

func (h *harness) login() string {
	h.t.Helper()
	code, out, errOut := h.run("login", "--issuer", h.provider.Issuer(), "--api-base-url", h.api.URL, "--allow-insecure-loopback", "--json")
	if code != 0 {
		h.t.Fatalf("login exit %d: %s %s", code, out, errOut)
	}
	creds, err := h.store.Read()
	if err != nil || creds == nil {
		h.t.Fatalf("no credentials stored: %v", err)
	}
	if strings.Contains(out, creds.AccessToken) || strings.Contains(errOut, creds.AccessToken) {
		h.t.Fatal("access token printed")
	}
	return creds.AccessToken
}

func TestLoginWhoamiAndWorkspaces(t *testing.T) {
	h := newHarness(t)
	token := h.login()
	creds, _ := h.store.Read()
	if creds.Environment != "prd" || creds.ClientID != "rzv-cli" || creds.APIBaseURL != h.api.URL {
		t.Fatalf("unexpected stored credentials: env=%s client=%s api=%s", creds.Environment, creds.ClientID, creds.APIBaseURL)
	}

	code, out, _ := h.run("whoami")
	if code != 0 || !strings.Contains(out, "Signed in as octocat") || !strings.Contains(out, "Workspaces: 2") {
		t.Fatalf("whoami: %d %q", code, out)
	}
	if strings.Contains(out, token) {
		t.Fatal("whoami printed the token")
	}
	if h.seenAuth[len(h.seenAuth)-1] != "Bearer "+token {
		t.Fatal("API call did not carry the stored token")
	}

	code, out, _ = h.run("workspaces", "list")
	if code != 0 || !strings.Contains(out, "- Acme [t-1] (acme, owner)") || !strings.Contains(out, `Beta\u001b[31m`) {
		t.Fatalf("workspaces list: %d %q", code, out)
	}

	code, out, _ = h.run("workspaces", "select", "beta", "--json")
	if code != 0 || strings.TrimSpace(out) != `{"selected_workspace_id":"t-2"}` {
		t.Fatalf("select by slug: %d %q", code, out)
	}
	code, _, errOut := h.run("accounts", "select", "t-9")
	if code != exitAccessDenied || !strings.Contains(errOut, "not a member") {
		t.Fatalf("select unknown: %d %q", code, errOut)
	}

	code, out, _ = h.run("workspaces", "list", "--json")
	var listed struct {
		Workspaces []map[string]string `json:"workspaces"`
		Selected   string              `json:"selected_workspace_id"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &listed) != nil || listed.Selected != "t-2" || len(listed.Workspaces) != 2 {
		t.Fatalf("workspaces list --json: %d %q", code, out)
	}

	code, out, _ = h.run("logout")
	if code != 0 || out != "Logged out.\n" {
		t.Fatalf("logout: %d %q", code, out)
	}
	if code, _, _ := h.run("whoami"); code != exitAuth {
		t.Fatalf("whoami after logout: %d", code)
	}
}

func TestExpiredSessionIsRefusedLocally(t *testing.T) {
	h := newHarness(t)
	h.provider.TokenLifetime = time.Second
	h.login()
	creds, _ := h.store.Read()
	claims := h.provider.Claims("rzv-customer-api")
	claims["exp"] = time.Now().Add(-time.Minute).Unix()
	creds.AccessToken = h.provider.Sign(claims)
	_ = h.store.Write(creds)
	calls := len(h.seenAuth)
	code, _, errOut := h.run("workspaces", "list")
	if code != exitAuth || !strings.Contains(errOut, "expired") || len(h.seenAuth) != calls {
		t.Fatalf("expired session: %d %q", code, errOut)
	}
}

func TestAPIErrorsMapToExitCodes(t *testing.T) {
	h := newHarness(t)
	h.login()
	h.json("GET /v1/customer/memberships", 401, `{"error":{"code":"unauthorized"}}`)
	code, _, errOut := h.run("workspaces", "list", "--json")
	var e map[string]string
	if code != exitAuth || json.Unmarshal([]byte(errOut), &e) != nil || e["error"] != "authentication_required" {
		t.Fatalf("401: %d %q", code, errOut)
	}
	h.json("GET /v1/customer/memberships", 503, `{"error":{"code":"authentication_unavailable"}}`)
	if code, _, _ := h.run("workspaces", "list"); code != exitUnavailable {
		t.Fatalf("503: %d", code)
	}
}

func TestCreateWorkspace(t *testing.T) {
	h := newHarness(t)
	h.login()
	var body map[string]string
	h.routes["POST /v1/customer/workspaces"] = func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"workspace":{"tenant_id":"t-3","tenant_slug":"new","tenant_name":"New","role":"owner"}}`))
	}
	code, out, _ := h.run("workspaces", "create", "--slug", "new", "--name", "New")
	if code != 0 || !strings.Contains(out, "Created workspace New [t-3]") || len(body["idempotency_key"]) != 64 {
		t.Fatalf("create: %d %q %v", code, out, body)
	}
	if creds, _ := h.store.Read(); creds.SelectedWorkspace != "t-3" {
		t.Fatal("new workspace not selected")
	}
	h.json("POST /v1/customer/workspaces", 409, `{"error":{"code":"workspace_slug_taken"}}`)
	if code, _, errOut := h.run("workspaces", "create", "--slug", "new", "--name", "New"); code != exitFailure || !strings.Contains(errOut, "already taken") {
		t.Fatalf("slug taken: %d %q", code, errOut)
	}
}

func TestAPIKeyIsRefusedForHumanCommands(t *testing.T) {
	h := newHarness(t)
	h.env["RZV_API_KEY"] = "rzv_test_key"
	if code, _, errOut := h.run("workspaces", "list"); code != exitAccessDenied || !strings.Contains(errOut, "RZV_API_KEY") {
		t.Fatalf("api key on human command: %d %q", code, errOut)
	}
}

func TestUsageAndHelp(t *testing.T) {
	h := newHarness(t)
	if code, out, _ := h.run(); code != 0 || !strings.Contains(out, "Staff commands are not included") {
		t.Fatalf("help: %d %q", code, out)
	}
	if code, out, _ := h.run("help", "login"); code != 0 || !strings.Contains(out, "--env") {
		t.Fatalf("help login: %d %q", code, out)
	}
	if code, _, _ := h.run("staff", "roles", "list"); code != exitUsage {
		t.Fatalf("staff: %d", code)
	}
	if code, _, errOut := h.run("whoami", "--bogus", "x"); code != exitUsage || !strings.Contains(errOut, "--bogus") {
		t.Fatalf("unknown option: %d %q", code, errOut)
	}
	if code, _, _ := h.run("login", "--env", "nowhere"); code != exitUsage {
		t.Fatalf("unknown env: %d", code)
	}
	if code, out, _ := h.run("version", "--json"); code != 0 || !strings.Contains(out, `"version":"dev"`) {
		t.Fatalf("version: %d %q", code, out)
	}
	if code, out, _ := h.run("--version"); code != 0 || !strings.HasPrefix(out, "rzv dev") {
		t.Fatalf("--version: %d %q", code, out)
	}
}

func TestParse(t *testing.T) {
	cmd, err := parse([]string{"jobs", "get", "--workspace=t-1", "--json", "j-1", "--", "--literal"})
	if err != nil || cmd.name != "jobs" || strings.Join(cmd.args, ",") != "get,j-1,--literal" || cmd.options["workspace"] != "t-1" || !cmd.json() {
		t.Fatalf("parse: %+v %v", cmd, err)
	}
	if _, err := parse([]string{"x", "--json", "--json"}); err == nil {
		t.Fatal("duplicate flag accepted")
	}
	if _, err := parse([]string{"x", "--name"}); err == nil {
		t.Fatal("missing value accepted")
	}
	if _, err := parse([]string{"x", "--json=true"}); err == nil {
		t.Fatal("boolean with value accepted")
	}
}

// A bare command prints its usage on real lines, and an error still cannot
// carry a terminal escape sequence.
func TestUsageErrorsKeepNewlinesAndEscapeControlCharacters(t *testing.T) {
	h := newHarness(t)
	for _, name := range []string{"sandboxes", "tickets", "jobs"} {
		code, _, errOut := h.run(name)
		if code != exitUsage || !strings.HasPrefix(errOut, "rzv: Usage:\n  rzv "+name+" ") || strings.Contains(errOut, `\u000a`) {
			t.Errorf("%s: exit %d %q", name, code, errOut)
		}
	}
	code, _, errOut := h.run("whoami", "--bo\x1b[31mgus", "x")
	if code != exitUsage || strings.Contains(errOut, "\x1b") || !strings.Contains(errOut, `--bo\u001b[31mgus`) {
		t.Fatalf("escape in an error: %d %q", code, errOut)
	}
}
