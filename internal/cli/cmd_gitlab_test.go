package cli

import (
	"net/http"
	"strings"
	"testing"
)

const gitlabConnectionsJSON = `{"connections":[{"connection_id":"glc_0123456789abcdef01234567","gitlab_user_id":42,"gitlab_username":"ditah","status":"active","reconnect_required":false,"instance_id":"gli_0123456789abcdef01234567","created_at":"2026-10-26T09:00:00Z"}]}`

func TestGitLabConnectionsAndDisconnect(t *testing.T) {
	h := selectedHarness(t)
	h.json("GET /v1/customer/tenants/t-1/gitlab/connections", 200, gitlabConnectionsJSON)
	code, out, _ := h.run("gitlab", "connections")
	if code != 0 || !strings.Contains(out, "GitLab connections for workspace t-1") || !strings.Contains(out, "glc_0123456789abcdef01234567") || !strings.Contains(out, "ditah") {
		t.Fatalf("connections: %d %q", code, out)
	}
	if strings.Contains(out, "token") {
		t.Fatalf("connections output leaks credentials: %q", out)
	}
	h.routes["DELETE /v1/customer/tenants/t-1/gitlab/connections/glc_0123456789abcdef01234567"] = func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}
	if code, out, _ := h.run("gitlab", "disconnect", "glc_0123456789abcdef01234567", "--json"); code != 0 || !strings.Contains(out, `"disconnected":true`) {
		t.Fatalf("disconnect: %d %q", code, out)
	}
}

func TestGitLabErrorsRenderPlainly(t *testing.T) {
	h := selectedHarness(t)
	h.json("GET /v1/customer/tenants/t-1/gitlab/connections", 404, `{"error":{"code":"integration_disabled"}}`)
	code, _, errOut := h.run("gitlab", "connections")
	if code == 0 || !strings.Contains(errOut, "integration_disabled") {
		t.Fatalf("disabled connections: %d %q", code, errOut)
	}
	h.json("GET /v1/customer/tenants/t-1/gitlab/connections", 403, `{"error":{"code":"api_key_scope_required"}}`)
	if code, _, errOut := h.run("gitlab", "connections"); code == 0 || !strings.Contains(errOut, "does not allow") {
		t.Fatalf("scope error: %d %q", code, errOut)
	}
	if code, _, _ := h.run("gitlab", "disconnect"); code == 0 {
		t.Fatal("disconnect without an id must fail")
	}
	// A malformed id is refused before any request: no path traversal.
	before := len(h.seenAuth)
	if code, _, errOut := h.run("gitlab", "disconnect", "glc_../../tenants/t-2"); code == 0 || !strings.Contains(errOut, "connection id must look like") || len(h.seenAuth) != before {
		t.Fatalf("malformed id: %d %q requests=%d", code, errOut, len(h.seenAuth)-before)
	}
}

func TestGitLabConnectPrintsPortalURLAndOpensOnlyWithFlag(t *testing.T) {
	h := selectedHarness(t)
	var opened []string
	open := func(u string) error {
		opened = append(opened, u)
		return nil
	}
	var stdout, stderr strings.Builder
	code := Run(t.Context(), []string{"gitlab", "connect"}, Deps{Stdout: &stdout, Stderr: &stderr, Store: h.store, Getenv: func(k string) string { return h.env[k] }, OpenBrowser: open})
	if code != 0 || !strings.Contains(stdout.String(), "https://app.runzivo.dev/settings?tab=integrations") || len(opened) != 0 {
		t.Fatalf("connect without --open: %d %q opened=%v", code, stdout.String(), opened)
	}
	h.env["RZV_ENV"] = "shared"
	stdout.Reset()
	code = Run(t.Context(), []string{"gitlab", "connect", "--open"}, Deps{Stdout: &stdout, Stderr: &stderr, Store: h.store, Getenv: func(k string) string { return h.env[k] }, OpenBrowser: open})
	if code != 0 || len(opened) != 1 || opened[0] != "https://shared-app.runzivo.dev/settings?tab=integrations" {
		t.Fatalf("connect --open: %d opened=%v", code, opened)
	}
}
