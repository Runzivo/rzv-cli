package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// #165: billing controls reach the customer API with the same permissions as
// the portal. These tests cover request bodies, usage errors, denials and the
// retry guidance for checkout.
func TestBillingControlsReadAndWrite(t *testing.T) {
	h := selectedHarness(t)
	h.json("GET /v1/customer/tenants/t-1/billing/invoices", 200, `{"invoices":[{"invoice_number":"INV-1"}],"limit":50}`)
	if code, out, _ := h.run("billing", "invoices"); code != 0 || !strings.Contains(out, "1 invoice(s)") {
		t.Fatalf("invoices: %d %q", code, out)
	}
	if code, out, _ := h.run("billing", "invoices", "--json"); code != 0 || !strings.Contains(out, `"INV-1"`) {
		t.Fatalf("invoices --json: %d %q", code, out)
	}

	h.json("GET /v1/customer/tenants/t-1/billing/auto-recharge", 200, `{"auto_recharge":{"supported":true,"enabled":false,"threshold_minutes":10,"pack_id":"p1","payment_method_saved":false,"in_flight":false}}`)
	if code, out, _ := h.run("billing", "auto-recharge"); code != 0 || !strings.Contains(out, "Auto-recharge for workspace t-1: off") {
		t.Fatalf("auto-recharge: %d %q", code, out)
	}
	var seen map[string]any
	h.routes["PUT /v1/customer/tenants/t-1/billing/auto-recharge"] = func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &seen)
		_, _ = w.Write([]byte(`{"auto_recharge":{"supported":true,"enabled":true,"threshold_minutes":30,"pack_id":"p1"}}`))
	}
	if code, out, errOut := h.run("billing", "auto-recharge", "set", "--enabled", "true", "--threshold", "30", "--pack", "p1"); code != 0 || !strings.Contains(out, "on") {
		t.Fatalf("auto-recharge set: %d %q %q", code, out, errOut)
	}
	if seen["enabled"] != true || seen["threshold_minutes"] != float64(30) || seen["pack_id"] != "p1" {
		t.Fatalf("auto-recharge body = %#v", seen)
	}
	if code, _, _ := h.run("billing", "auto-recharge", "set", "--enabled", "maybe", "--threshold", "30", "--pack", "p1"); code != exitUsage {
		t.Fatalf("bad --enabled = %d; want usage", code)
	}
	if code, _, _ := h.run("billing", "auto-recharge", "set", "--enabled", "true", "--threshold", "30"); code != exitUsage {
		t.Fatalf("missing --pack = %d; want usage", code)
	}

	h.json("GET /v1/customer/tenants/t-1/billing/country", 200, `{"billing_country":"CA"}`)
	if code, out, _ := h.run("billing", "country"); code != 0 || !strings.Contains(out, "CA") {
		t.Fatalf("country: %d %q", code, out)
	}
	h.routes["PUT /v1/customer/tenants/t-1/billing/country"] = func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"country":"US"`) {
			w.WriteHeader(400)
			return
		}
		_, _ = w.Write([]byte(`{"billing_country":"US"}`))
	}
	if code, out, _ := h.run("billing", "country", "set", "US"); code != 0 || !strings.Contains(out, "US") {
		t.Fatalf("country set: %d %q", code, out)
	}
	if code, _, errOut := h.run("billing", "country", "set"); code != exitUsage {
		t.Fatalf("country set without code = %d %q; want usage", code, errOut)
	}

	h.json("GET /v1/customer/tenants/t-1/billing/marketplace", 200, `{"marketplace":{"status":"none"}}`)
	if code, out, _ := h.run("billing", "marketplace"); code != 0 || !strings.Contains(out, "Marketplace plan for workspace t-1") {
		t.Fatalf("marketplace: %d %q", code, out)
	}
}

func TestBillingCheckoutAndPortal(t *testing.T) {
	h := selectedHarness(t)
	var checkoutBody map[string]any
	h.routes["POST /v1/customer/tenants/t-1/billing/checkout"] = func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &checkoutBody)
		_, _ = w.Write([]byte(`{"order_reference":"ord_1","checkout_url":"https://checkout.example.test/s/1","payment_mode":"test"}`))
	}
	code, out, _ := h.run("billing", "checkout", "--pack", "p1", "--operation-id", "op-retry-1")
	if code != 0 || !strings.Contains(out, "https://checkout.example.test/s/1") || !strings.Contains(out, "--operation-id op-retry-1") {
		t.Fatalf("checkout: %d %q", code, out)
	}
	if checkoutBody["pack_id"] != "p1" || checkoutBody["client_operation_id"] != "op-retry-1" {
		t.Fatalf("checkout body = %#v", checkoutBody)
	}
	if code, _, _ := h.run("billing", "checkout", "--pack", "p1"); code != exitUsage {
		t.Fatalf("checkout without --operation-id = %d; want usage", code)
	}

	h.json("POST /v1/customer/tenants/t-1/billing/portal", 200, `{"billing_portal_url":"https://portal.example.test/p/1"}`)
	if code, out, _ := h.run("billing", "portal"); code != 0 || !strings.Contains(out, "do not store it") {
		t.Fatalf("portal: %d %q", code, out)
	}
	h.json("POST /v1/customer/tenants/t-1/billing/portal", 403, `{"error":{"code":"permission_required"}}`)
	if code, _, errOut := h.run("billing", "portal"); code != exitAccessDenied || !strings.Contains(errOut, "Your workspace role does not allow this change.") {
		t.Fatalf("denied portal = %d %q; want the role message", code, errOut)
	}
	h.json("POST /v1/customer/tenants/t-1/billing/portal", 403, `{"error":{"code":"reauth_required"}}`)
	if code, _, errOut := h.run("billing", "portal"); code != exitAuth || !strings.Contains(errOut, "Sign in again with `rzv login`, then retry.") {
		t.Fatalf("stale sign-in portal = %d %q; want the sign-in message", code, errOut)
	}
	h.json("PUT /v1/customer/tenants/t-1/billing/auto-recharge", 403, `{"error":{"code":"auto_recharge_enable_requires_person"}}`)
	if code, _, errOut := h.run("billing", "auto-recharge", "set", "--enabled", "true", "--threshold", "30", "--pack", "p1"); code != exitAccessDenied || !strings.Contains(errOut, "needs a person signed in, not an API key") {
		t.Fatalf("api-key enable = %d %q; want the person-only message", code, errOut)
	}
	h.json("GET /v1/customer/tenants/t-1/billing/invoices", 403, `{"error":{"code":"unknown_code"}}`)
	if code, _, errOut := h.run("billing", "invoices"); code != exitAccessDenied || !strings.Contains(errOut, "not authorized") {
		t.Fatalf("generic 403 = %d %q; want the generic message", code, errOut)
	}
}
