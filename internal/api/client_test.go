package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := New(server.URL, "test-credential", "rzv/test", nil)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestNewRejectsUnsafeBaseURLs(t *testing.T) {
	for _, raw := range []string{"http://api.example.test", "https://api.example.test/v1", "https://u:p@api.example.test", "not a url", "https://api.example.test?x=1"} {
		if _, err := New(raw, "x", "", nil); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
	if _, err := New("https://api.runzivo.dev/", "x", "", nil); err != nil {
		t.Errorf("rejected trailing slash: %v", err)
	}
}

func TestMembershipsSendsBearerAndDecodes(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/customer/memberships" || r.Header.Get("Authorization") != "Bearer test-credential" || r.Header.Get("User-Agent") != "rzv/test" {
			t.Errorf("unexpected request %s %v", r.URL.Path, r.Header)
		}
		_, _ = w.Write([]byte(`{"memberships":[{"tenant_id":"t1","tenant_slug":"acme","tenant_name":"Acme","role":"owner"}]}`))
	})
	got, err := client.Memberships(context.Background())
	if err != nil || len(got) != 1 || got[0].TenantName != "Acme" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestErrorDecoding(t *testing.T) {
	cases := []struct {
		status int
		body   string
		code   string
	}{
		{401, `{"error":{"code":"unauthorized"}}`, "unauthorized"},
		{403, `{"error":{"code":"Bad Code <script>"}}`, "invalid_error_response"},
		{500, `not json`, "invalid_error_response"},
	}
	for _, tc := range cases {
		client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		})
		_, err := client.Memberships(context.Background())
		var apiErr *Error
		if !errors.As(err, &apiErr) || apiErr.Status != tc.status || apiErr.Code != tc.code {
			t.Errorf("status %d: got %v", tc.status, err)
		}
	}
}

func TestMalformedAndRedirect(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"something":"else"}`))
	})
	var apiErr *Error
	if _, err := client.Memberships(context.Background()); !errors.As(err, &apiErr) || apiErr.Code != "malformed_response" {
		t.Fatalf("want malformed_response, got %v", err)
	}
	redirect := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://elsewhere.example.test/", http.StatusFound)
	})
	if _, err := redirect.Memberships(context.Background()); !errors.As(err, &apiErr) || apiErr.Status != http.StatusFound {
		t.Fatalf("redirect followed or misreported: %v", err)
	}
}

func TestCreateWorkspaceBody(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if r.Method != http.MethodPost || body["slug"] != "acme" || body["idempotency_key"] != "k" {
			t.Errorf("unexpected body %v", body)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"workspace":{"tenant_id":"t1","tenant_slug":"acme","tenant_name":"Acme","role":"owner"}}`))
	})
	ws, err := client.CreateWorkspace(context.Background(), CreateWorkspaceInput{Slug: "acme", Name: "Acme", IdempotencyKey: "k"})
	if err != nil || ws.TenantID != "t1" {
		t.Fatalf("got %+v, %v", ws, err)
	}
}

func TestAPIKeyPermissionsBodyAndResponse(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if r.Method != http.MethodPost || r.URL.Path != "/v1/customer/tenants/t/api-keys" || body["scopes"] != nil {
			t.Errorf("unexpected request %s %s %v", r.Method, r.URL.Path, body)
		}
		permissions, ok := body["permissions"].([]any)
		if !ok || len(permissions) != 2 || permissions[0] != "jobs:write" || permissions[1] != "logs:read" {
			t.Errorf("unexpected permissions %v", body)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"api_key":{"id":"k1","name":"deploy","prefix":"rzv_test","scopes":[],"permissions":["jobs:write","logs:read"],"created_at":"a","last_used_at":null,"revoked_at":null,"expires_at":null},"secret":"one-time-secret"}`))
	})
	created, err := client.CreateAPIKey(context.Background(), "t", CreateAPIKeyInput{Name: "deploy", Permissions: []string{"jobs:write", "logs:read"}, IdempotencyKey: "op"})
	if err != nil || created.Secret != "one-time-secret" || len(created.APIKey.Permissions) != 2 || created.APIKey.Permissions[1] != "logs:read" {
		t.Fatalf("got %+v, %v", created, err)
	}
}

func TestGitHubInstallationsAndWorkspaceConnection(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/customer/tenants/t1/github-installations":
			if r.Method != http.MethodGet {
				t.Errorf("installations method = %s", r.Method)
			}
			_, _ = w.Write([]byte(`{"installations":[{"installation_id":"9","state":"active","account_name":"acme","manage_url":null,"runner_permission_missing":true,"setup_state":"permissions_pending","next_retry_at":null,"account_archived":true,"account_archived_at":"2026-10-20T00:00:00Z","repositories":[{"repository_id":"100","name":"acme/app","authorized":true,"archived":true,"archived_at":null}]}]}`))
		case "/v1/customer/github/workspaces/start":
			if r.Method != http.MethodPost {
				t.Errorf("start method = %s", r.Method)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"state":"s1","expires_at":"2026-10-22T00:00:00Z"}`))
		case "/v1/customer/github/workspaces/complete":
			var input CompleteWorkspaceConnectionInput
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.State != "s1" || input.InstallationID != 9 || input.UserAccessToken != "token" {
				t.Errorf("complete input = %+v, %v", input, err)
			}
			_, _ = w.Write([]byte(`{"outcome":"installation_pending","account_login":"acme","account_type":"Organization","workspace":{"tenant_id":"t1","tenant_slug":"acme","tenant_name":"Acme","role":"owner"}}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
	installations, err := client.Installations(context.Background(), "t1")
	if err != nil || len(installations) != 1 || !installations[0].RunnerPermissionMissing || !installations[0].Repositories[0].Archived {
		t.Fatalf("installations = %+v, %v", installations, err)
	}
	start, err := client.StartWorkspaceConnection(context.Background())
	if err != nil || start.State != "s1" {
		t.Fatalf("start = %+v, %v", start, err)
	}
	completed, err := client.CompleteWorkspaceConnection(context.Background(), CompleteWorkspaceConnectionInput{State: "s1", InstallationID: 9, UserAccessToken: "token"})
	if err != nil || completed.Outcome != "installation_pending" || completed.Workspace == nil || completed.Workspace.TenantID != "t1" {
		t.Fatalf("complete = %+v, %v", completed, err)
	}
}

func TestGitHubWorkspaceConnectionRejectsMalformedDecision(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"outcome":"created","account_login":"acme","account_type":"Organization"}`))
	})
	_, err := client.CompleteWorkspaceConnection(context.Background(), CompleteWorkspaceConnectionInput{State: "s", InstallationID: 1, UserAccessToken: "token"})
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != "malformed_response" {
		t.Fatalf("want malformed response, got %v", err)
	}
}

func TestGitHubInstallationsRejectMalformedIdentifiers(t *testing.T) {
	for _, body := range []string{
		`{"installations":[{"state":"active","account_name":null,"manage_url":null,"repositories":[]}]}`,
		`{"installations":[{"installation_id":"9","state":"active","account_name":null,"manage_url":null,"repositories":[{"repository_id":"","name":null,"authorized":true}]}]}`,
		`{"installations":[{"installation_id":"9","state":"active","manage_url":null,"repositories":[]}]}`,
		`{"installations":[{"installation_id":"9","state":"active","account_name":null,"repositories":[]}]}`,
		`{"installations":[{"installation_id":"9","state":"active","account_name":null,"manage_url":null,"repositories":[{"repository_id":"10","authorized":false}]}]}`,
		`{"installations":[{"installation_id":"9","state":"active","account_name":null,"manage_url":null,"repositories":[{"repository_id":"10","name":null}]}]}`,
		`{"installations":[{"installation_id":"9","state":"active","account_name":null,"manage_url":null,"repositories":[{"repository_id":"10","name":null,"authorized":null}]}]}`,
	} {
		client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(body))
		})
		_, err := client.Installations(context.Background(), "t1")
		var apiErr *Error
		if !errors.As(err, &apiErr) || apiErr.Code != "malformed_response" {
			t.Fatalf("body %s: want malformed response, got %v", body, err)
		}
	}
}

func TestRedeemCodeBodyAndResponse(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if r.Method != http.MethodPost || r.URL.Path != "/v1/customer/tenants/t/billing/codes/redeem" || body["code"] != "input-code" {
			t.Errorf("unexpected request %s %s %v", r.Method, r.URL.Path, body)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"redemption":{"id":"r1","kind":"voucher","code":"1234","status":"credited","equivalent_minutes":1,"credit_expires_at":null},"replayed":false}`))
	})
	result, err := client.RedeemCode(context.Background(), "t", "input-code")
	if err != nil || result.Redemption.ID != "r1" || result.Redemption.EquivalentMinutes != 1 || result.Redemption.CreditExpiresAt != nil || result.Replayed {
		t.Fatalf("got %+v, %v", result, err)
	}
}

func TestRedeemCodeRejectsInvalidEquivalentMinutes(t *testing.T) {
	cases := map[string]string{
		"missing": `{"redemption":{"id":"r1","kind":"promo","code":"SAFE","status":"credited","credit_expires_at":null},"replayed":false}`,
		"null":    `{"redemption":{"id":"r1","kind":"promo","code":"SAFE","status":"credited","equivalent_minutes":null,"credit_expires_at":null},"replayed":false}`,
		"zero":    `{"redemption":{"id":"r1","kind":"promo","code":"SAFE","status":"credited","equivalent_minutes":0,"credit_expires_at":null},"replayed":false}`,
	}
	for name, body := range cases {
		client := newTestClient(t, respond(http.StatusCreated, body))
		if _, err := client.RedeemCode(context.Background(), "t", "input-code"); err == nil {
			t.Errorf("equivalent_minutes %s accepted", name)
		}
	}
}

func TestTenantPathEscapes(t *testing.T) {
	if got := tenantPath("a/b c", "/jobs"); got != "/v1/customer/tenants/a%2Fb%20c/jobs" {
		t.Fatalf("got %q", got)
	}
}
