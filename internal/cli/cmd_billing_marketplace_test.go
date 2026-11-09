package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestMarketplaceClaimable(t *testing.T) {
	h := selectedHarness(t)
	h.json("GET /v1/customer/tenants/t-1/billing/marketplace/claimable", 200, `{"claimable":[{"account_id":4242,"account_login":"acme-inc"},{"account_id":7,"account_login":"octo"}]}`)
	code, out, errOut := h.run("billing", "marketplace", "claimable")
	if code != 0 || errOut != "" || !strings.Contains(out, "ACCOUNT ID") || !strings.Contains(out, "4242") || !strings.Contains(out, "acme-inc") || !strings.Contains(out, "octo") {
		t.Fatalf("claimable: %d %q %q", code, out, errOut)
	}
	// A slug resolves like every other workspace command.
	if code, out, _ := h.run("billing", "marketplace", "claimable", "--workspace", "acme"); code != 0 || !strings.Contains(out, "4242") {
		t.Fatalf("claimable by slug: %d %q", code, out)
	}
	code, out, _ = h.run("billing", "marketplace", "claimable", "--json")
	var got struct {
		WorkspaceID string `json:"workspace_id"`
		Claimable   []struct {
			AccountID    int64  `json:"account_id"`
			AccountLogin string `json:"account_login"`
		} `json:"claimable"`
	}
	if err := json.Unmarshal([]byte(out), &got); code != 0 || err != nil || got.WorkspaceID != "t-1" || len(got.Claimable) != 2 || got.Claimable[0].AccountID != 4242 || got.Claimable[0].AccountLogin != "acme-inc" {
		t.Fatalf("claimable --json: %d %q %v", code, out, err)
	}

	h.json("GET /v1/customer/tenants/t-1/billing/marketplace/claimable", 200, `{"claimable":[]}`)
	if code, out, _ := h.run("billing", "marketplace", "claimable"); code != 0 || !strings.Contains(out, "No GitHub Marketplace plan is waiting for this workspace.") {
		t.Fatalf("empty claimable: %d %q", code, out)
	}
	if code, out, _ := h.run("billing", "marketplace", "claimable", "--json"); code != 0 || !strings.Contains(out, `"claimable":[]`) {
		t.Fatalf("empty claimable --json: %d %q", code, out)
	}
	h.env["LANG"] = "fr_CA.UTF-8"
	if code, out, _ := h.run("billing", "marketplace", "claimable"); code != 0 || !strings.Contains(out, "Aucun forfait GitHub Marketplace") {
		t.Fatalf("fr-CA empty claimable: %d %q", code, out)
	}
	delete(h.env, "LANG")

	h.json("GET /v1/customer/tenants/t-1/billing/marketplace/claimable", 200, `{}`)
	if code, _, _ := h.run("billing", "marketplace", "claimable"); code == 0 {
		t.Fatal("a response without claimable was accepted")
	}
}

func TestMarketplaceClaimableRefusals(t *testing.T) {
	h := selectedHarness(t)
	for _, tc := range []struct {
		status int
		code   string
		exit   int
		want   string
	}{
		{403, "workspace_owner_required", exitAccessDenied, "Only a workspace owner"},
		{403, "api_key_scope_required", exitAccessDenied, "API keys cannot use them"},
		{503, "billing_not_configured", exitUnavailable, "Billing is not available"},
	} {
		h.json("GET /v1/customer/tenants/t-1/billing/marketplace/claimable", tc.status, `{"error":{"code":"`+tc.code+`"}}`)
		if code, _, errOut := h.run("billing", "marketplace", "claimable"); code != tc.exit || !strings.Contains(errOut, tc.want) {
			t.Fatalf("%s = %d %q", tc.code, code, errOut)
		}
	}
	code, _, errOut := h.run("billing", "marketplace", "claimable", "--json")
	if code != exitUnavailable || !strings.Contains(errOut, `"error":"billing_not_configured"`) {
		t.Fatalf("--json error = %d %q", code, errOut)
	}
}

func TestMarketplaceClaimableAPIKey(t *testing.T) {
	h := newHarness(t)
	h.env["RZV_API_KEY"] = "rzv_test_key"
	h.env["RZV_API_URL"] = h.api.URL
	h.json("GET /v1/customer/tenants/t-5/billing/marketplace/claimable", 403, `{"error":{"code":"api_key_scope_required"}}`)
	code, _, errOut := h.run("billing", "marketplace", "claimable", "--workspace", "t-5")
	if code != exitAccessDenied || !strings.Contains(errOut, "rzv login") {
		t.Fatalf("api key claimable = %d %q", code, errOut)
	}
}

func claimRoute(h *harness, seen *map[string]any, calls *int, status int, body string) {
	h.routes["POST /v1/customer/tenants/t-1/billing/marketplace/claim"] = func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		*calls++
		*seen = nil
		_ = json.Unmarshal(raw, seen)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

func TestMarketplaceClaim(t *testing.T) {
	h := selectedHarness(t)
	var seen map[string]any
	calls := 0
	claimRoute(h, &seen, &calls, 200, `{"marketplace_claim":{"account_id":4242,"account_login":"acme-inc","already_linked":false}}`)
	code, out, errOut := h.run("billing", "marketplace", "claim", "--account-id", "4242")
	if code != 0 || errOut != "" || !strings.Contains(out, "Linked the GitHub Marketplace purchase of acme-inc (account 4242) to workspace t-1") {
		t.Fatalf("claim: %d %q %q", code, out, errOut)
	}
	if len(seen) != 1 || seen["account_id"] != float64(4242) {
		t.Fatalf("claim body = %#v", seen)
	}
	code, out, _ = h.run("billing", "marketplace", "claim", "--account-id", "4242", "--workspace", "acme", "--json")
	var got struct {
		WorkspaceID string `json:"workspace_id"`
		Claim       struct {
			AccountID     int64  `json:"account_id"`
			AccountLogin  string `json:"account_login"`
			AlreadyLinked bool   `json:"already_linked"`
		} `json:"marketplace_claim"`
	}
	if err := json.Unmarshal([]byte(out), &got); code != 0 || err != nil || got.WorkspaceID != "t-1" || got.Claim.AccountID != 4242 || got.Claim.AccountLogin != "acme-inc" || got.Claim.AlreadyLinked {
		t.Fatalf("claim --json: %d %q %v", code, out, err)
	}

	claimRoute(h, &seen, &calls, 200, `{"marketplace_claim":{"account_id":4242,"account_login":"acme-inc","already_linked":true}}`)
	if code, out, _ := h.run("billing", "marketplace", "claim", "--account-id", "4242"); code != 0 || !strings.Contains(out, "is already linked to workspace t-1. Nothing changed.") {
		t.Fatalf("already linked: %d %q", code, out)
	}
	if code, out, _ := h.run("billing", "marketplace", "claim", "--account-id", "4242", "--json"); code != 0 || !strings.Contains(out, `"already_linked":true`) {
		t.Fatalf("already linked --json: %d %q", code, out)
	}
	h.env["LANG"] = "fr_CA.UTF-8"
	if code, out, _ := h.run("billing", "marketplace", "claim", "--account-id", "4242"); code != 0 || !strings.Contains(out, "déjà lié") {
		t.Fatalf("fr-CA already linked: %d %q", code, out)
	}
	delete(h.env, "LANG")

	claimRoute(h, &seen, &calls, 200, `{}`)
	if code, _, _ := h.run("billing", "marketplace", "claim", "--account-id", "4242"); code == 0 {
		t.Fatal("a response without marketplace_claim was accepted")
	}
}

func TestMarketplaceClaimRefusals(t *testing.T) {
	h := selectedHarness(t)
	var seen map[string]any
	calls := 0
	for _, tc := range []struct {
		status int
		code   string
		exit   int
		want   string
	}{
		{403, "workspace_owner_required", exitAccessDenied, "Only a workspace owner"},
		{403, "api_key_scope_required", exitAccessDenied, "API keys cannot use them"},
		{403, "tenant_membership_required", exitAccessDenied, "not authorized"},
		{409, "marketplace_claim_not_owner", exitFailure, "no active connection of that GitHub account"},
		{409, "marketplace_claim_no_hold", exitFailure, "not waiting to be claimed"},
		{409, "marketplace_workspace_has_plan", exitFailure, "already has another active plan"},
		{503, "billing_not_configured", exitUnavailable, "Billing is not available"},
		{400, "invalid_request", exitFailure, "400 invalid_request"},
	} {
		claimRoute(h, &seen, &calls, tc.status, `{"error":{"code":"`+tc.code+`"}}`)
		code, out, errOut := h.run("billing", "marketplace", "claim", "--account-id", "4242")
		if code != tc.exit || out != "" || !strings.Contains(errOut, tc.want) {
			t.Fatalf("%s = %d %q %q", tc.code, code, out, errOut)
		}
		code, _, errOut = h.run("billing", "marketplace", "claim", "--account-id", "4242", "--json")
		if code != tc.exit || !strings.Contains(errOut, `"error":"`+tc.code+`"`) && tc.code != "tenant_membership_required" {
			t.Fatalf("%s --json = %d %q", tc.code, code, errOut)
		}
	}
}

// A stale sign-in ends with the shared `rzv login` guidance that every other
// billing write gives; the command does not retry on its own, and running it
// again after signing in sends the same request.
func TestMarketplaceClaimStaleSignIn(t *testing.T) {
	h := selectedHarness(t)
	var seen map[string]any
	calls := 0
	claimRoute(h, &seen, &calls, 403, `{"error":{"code":"reauth_required"}}`)
	code, _, errOut := h.run("billing", "marketplace", "claim", "--account-id", "4242")
	if code != exitAuth || !strings.Contains(errOut, "Sign in again with `rzv login`, then retry.") || calls != 1 {
		t.Fatalf("stale sign-in = %d %q calls=%d", code, errOut, calls)
	}
	claimRoute(h, &seen, &calls, 200, `{"marketplace_claim":{"account_id":4242,"account_login":"acme-inc","already_linked":false}}`)
	if code, out, _ := h.run("billing", "marketplace", "claim", "--account-id", "4242"); code != 0 || !strings.Contains(out, "Linked") || calls != 2 {
		t.Fatalf("retry after sign-in = %d %q calls=%d", code, out, calls)
	}
}

func TestMarketplaceClaimAPIKey(t *testing.T) {
	h := newHarness(t)
	h.env["RZV_API_KEY"] = "rzv_test_key"
	h.env["RZV_API_URL"] = h.api.URL
	h.json("POST /v1/customer/tenants/t-5/billing/marketplace/claim", 403, `{"error":{"code":"api_key_scope_required"}}`)
	code, _, errOut := h.run("billing", "marketplace", "claim", "--account-id", "4242", "--workspace", "t-5")
	if code != exitAccessDenied || !strings.Contains(errOut, "rzv login") {
		t.Fatalf("api key claim = %d %q", code, errOut)
	}
}

func TestMarketplaceClaimValidatesBeforeHTTP(t *testing.T) {
	h := selectedHarness(t)
	before := len(h.seenAuth)
	for _, args := range [][]string{
		{"billing", "marketplace", "claim"},
		{"billing", "marketplace", "claim", "--account-id", "0"},
		{"billing", "marketplace", "claim", "--account-id", "-3"},
		{"billing", "marketplace", "claim", "--account-id", "abc"},
		{"billing", "marketplace", "claim", "--account-id", "1.5"},
		{"billing", "marketplace", "claim", "--account-id", "99999999999999999999"},
		{"billing", "marketplace", "claim", "--account-id", "1", "extra"},
		{"billing", "marketplace", "claim", "--account-id", "1", "--bogus", "x"},
		{"billing", "marketplace", "claimable", "extra"},
	} {
		if code, _, errOut := h.run(args...); code != exitUsage {
			t.Fatalf("%v = %d %q; want usage", args, code, errOut)
		}
	}
	if len(h.seenAuth) != before {
		t.Fatalf("validation made HTTP requests: %v", h.seenAuth[before:])
	}
	if code, _, errOut := h.run("billing", "marketplace", "claim", "--account-id", "0"); !strings.Contains(errOut, "positive whole number") {
		t.Fatalf("message = %d %q", code, errOut)
	}
}
