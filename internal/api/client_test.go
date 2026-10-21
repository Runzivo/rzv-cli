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
