package cli

import (
	"encoding/json"
	"net"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

const billingCheckoutResponse = `{"order_reference":"order-1","checkout_url":"https://checkout.example.test/path","payment_mode":"test"}`
const billingPortalResponse = `{"customer_portal_link":"https://portal.example.test/path"}`

func TestBillingHostedLinksDoNotOpenWhenSuppressed(t *testing.T) {
	for _, tc := range []struct {
		name  string
		args  []string
		route string
		body  string
	}{
		{"checkout no browser", []string{"billing", "checkout", "--pack", "pack", "--operation-id", "op", "--no-browser"}, "POST /v1/customer/tenants/t-1/billing/checkout", billingCheckoutResponse},
		{"checkout JSON", []string{"billing", "checkout", "--pack", "pack", "--operation-id", "op", "--json"}, "POST /v1/customer/tenants/t-1/billing/checkout", billingCheckoutResponse},
		{"portal no browser", []string{"billing", "portal", "order-1", "--no-browser"}, "POST /v1/customer/tenants/t-1/billing/purchases/order-1/portal", billingPortalResponse},
		{"portal JSON", []string{"billing", "portal", "order-1", "--json"}, "POST /v1/customer/tenants/t-1/billing/purchases/order-1/portal", billingPortalResponse},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := selectedHarness(t)
			opened := 0
			h.browser = func(string) error { opened++; return nil }
			h.json(tc.route, http.StatusOK, tc.body)
			if code, _, errOut := h.run(tc.args...); code != 0 || opened != 0 {
				t.Fatalf("exit=%d opened=%d stderr=%q", code, opened, errOut)
			}
		})
	}
}

func TestHostedURLRejectsBrowserNormalizedLoopbackForms(t *testing.T) {
	for _, raw := range []string{
		"https://127.0.0.0x1",
		"https://0x7f.0x0.0x0.0x1",
		"https://0177.00.00.0x1",
	} {
		if err := hostedURL(raw); err == nil {
			t.Errorf("accepted browser-normalized loopback URL %q", raw)
		}
	}
}

func TestBillingHostedLinksLaunchExactURLAndPreserveOutputOnLauncherError(t *testing.T) {
	for _, tc := range []struct {
		name  string
		args  []string
		route string
		body  string
		url   string
	}{
		{"checkout", []string{"billing", "checkout", "--pack", "pack", "--operation-id", "op"}, "POST /v1/customer/tenants/t-1/billing/checkout", billingCheckoutResponse, "https://checkout.example.test/path"},
		{"portal", []string{"billing", "portal", "order-1"}, "POST /v1/customer/tenants/t-1/billing/purchases/order-1/portal", billingPortalResponse, "https://portal.example.test/path"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := selectedHarness(t)
			requests := 0
			h.routes[tc.route] = func(w http.ResponseWriter, _ *http.Request) {
				requests++
				_, _ = w.Write([]byte(tc.body))
			}
			calls := 0
			h.browser = func(got string) error {
				calls++
				if got != tc.url {
					t.Errorf("launcher URL = %q, want %q", got, tc.url)
				}
				return nil
			}
			if code, out, errOut := h.run(tc.args...); code != 0 || calls != 1 || requests != 1 || !strings.Contains(out, tc.url) || errOut != "" {
				t.Fatalf("success exit=%d calls=%d requests=%d stdout=%q stderr=%q", code, calls, requests, out, errOut)
			}

			calls = 0
			h.browser = func(string) error { calls++; return net.ErrClosed }
			if code, out, errOut := h.run(tc.args...); code != 0 || calls != 1 || requests != 2 || !strings.Contains(out, tc.url) || !strings.Contains(errOut, "Could not open") {
				t.Fatalf("launcher failure exit=%d calls=%d requests=%d stdout=%q stderr=%q", code, calls, requests, out, errOut)
			}
		})
	}
}

func TestBillingCheckoutGeneratedOperationIDIsRecoverable(t *testing.T) {
	cases := []struct {
		name   string
		exit   int
		handle func(http.ResponseWriter, *http.Request)
	}{
		{"server error", exitUnavailable, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"code":"billing_failed"}}`))
		}},
		{"connection closed", exitUnavailable, func(w http.ResponseWriter, r *http.Request) {
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("test server does not support hijacking")
			}
			conn, _, err := hijacker.Hijack()
			if err != nil {
				t.Fatal(err)
			}
			_ = conn.Close()
		}},
		{"malformed success", exitFailure, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"order_reference":"order"}`)) }},
		{"invalid URL", exitFailure, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"order_reference":"order","checkout_url":"http://bad.example.test","payment_mode":"test"}`))
		}},
	}
	operationID := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := selectedHarness(t)
			requests := 0
			var operation string
			done := make(chan struct{})
			h.routes["POST /v1/customer/tenants/t-1/billing/checkout"] = func(w http.ResponseWriter, r *http.Request) {
				defer close(done)
				requests++
				var body map[string]string
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				operation = body["client_operation_id"]
				tc.handle(w, r)
			}
			code, _, errOut := h.run("billing", "checkout", "--pack", "pack", "--no-browser")
			<-done
			if code != tc.exit || requests != 1 || !operationID.MatchString(operation) || !strings.Contains(errOut, operation) || !strings.Contains(errOut, "same --operation-id") {
				t.Fatalf("exit=%d requests=%d operation=%q stderr=%q", code, requests, operation, errOut)
			}
		})
	}
}

func TestBillingCheckoutErrors(t *testing.T) {
	cases := []struct {
		name   string
		status int
		code   string
		exit   int
		want   string
	}{
		{"payer verification", http.StatusForbidden, "payer_verification_required", exitAccessDenied, "verified email"},
		{"workspace access", http.StatusForbidden, "workspace_forbidden", exitAccessDenied, "not authorized"},
		{"operation conflict", http.StatusConflict, "idempotency_key_conflict", exitFailure, "different --pack"},
		{"reconciliation conflict", http.StatusConflict, "billing_reconciliation_required", exitFailure, "same --operation-id"},
		{"billing country required", http.StatusConflict, "billing_country_required", exitFailure, "billing country"},
		{"country unsupported", http.StatusConflict, "country_not_supported", exitFailure, "billing country"},
		{"billing configuration unavailable", http.StatusServiceUnavailable, "billing_not_configured", exitUnavailable, "not configured"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := selectedHarness(t)
			requests := 0
			h.routes["POST /v1/customer/tenants/t-1/billing/checkout"] = func(w http.ResponseWriter, _ *http.Request) {
				requests++
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(`{"error":{"code":"` + tc.code + `"}}`))
			}
			if code, _, errOut := h.run("billing", "checkout", "--pack", "pack", "--operation-id", "op", "--no-browser"); code != tc.exit || requests != 1 || !strings.Contains(errOut, tc.want) || !strings.Contains(errOut, "operation-id op") {
				t.Fatalf("exit=%d requests=%d stderr=%q", code, requests, errOut)
			}
		})
	}
}

func TestBillingPortalErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		code   string
		exit   int
		want   string
	}{
		{"payer verification", http.StatusForbidden, "payer_verification_required", exitAccessDenied, "verified email"},
		{"purchase not found", http.StatusNotFound, "not_found", exitNotFound, "not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := selectedHarness(t)
			h.json("POST /v1/customer/tenants/t-1/billing/purchases/order-1/portal", tc.status, `{"error":{"code":"`+tc.code+`"}}`)
			if code, _, errOut := h.run("billing", "portal", "order-1", "--no-browser"); code != tc.exit || !strings.Contains(errOut, tc.want) {
				t.Fatalf("exit=%d stderr=%q", code, errOut)
			}
		})
	}
}

func TestBillingCommandsUseAPIKeyAndDoNotPersistHostedLinks(t *testing.T) {
	h := selectedHarness(t)
	before, err := h.store.Read()
	if err != nil {
		t.Fatal(err)
	}
	h.json("GET /v1/customer/tenants/t-1/billing", http.StatusOK, `{"billing":{"allowance_configured":false,"checkout_available":true,"payment_mode":"test","live_payments_enabled":false,"packs":[]}}`)
	h.json("GET /v1/customer/tenants/t-1/billing/purchases", http.StatusOK, `{"purchases":[],"limit":20}`)
	h.json("POST /v1/customer/tenants/t-1/billing/checkout", http.StatusOK, billingCheckoutResponse)
	h.json("POST /v1/customer/tenants/t-1/billing/purchases/order-1/portal", http.StatusOK, billingPortalResponse)
	for _, args := range [][]string{
		{"billing"},
		{"billing", "purchases"},
		{"billing", "checkout", "--pack", "pack", "--operation-id", "op", "--no-browser"},
		{"billing", "portal", "order-1", "--no-browser"},
	} {
		if code, _, errOut := h.run(args...); code != 0 {
			t.Fatalf("%q: %d %q", args, code, errOut)
		}
	}
	after, err := h.store.Read()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("billing persisted hosted state: before=%+v after=%+v", before, after)
	}

	keyHarness := newHarness(t)
	keyHarness.env["RZV_API_KEY"] = "test-all-access-key"
	keyHarness.env["RZV_API_URL"] = keyHarness.api.URL
	keyHarness.json("GET /v1/customer/tenants/t-1/billing", http.StatusOK, `{"billing":{"allowance_configured":false,"checkout_available":true,"payment_mode":"test","live_payments_enabled":false,"packs":[]}}`)
	keyHarness.json("GET /v1/customer/tenants/t-1/billing/purchases", http.StatusOK, `{"purchases":[],"limit":20}`)
	keyHarness.json("POST /v1/customer/tenants/t-1/billing/checkout", http.StatusOK, billingCheckoutResponse)
	keyHarness.json("POST /v1/customer/tenants/t-1/billing/purchases/order-1/portal", http.StatusOK, billingPortalResponse)
	for _, args := range [][]string{
		{"billing", "--workspace", "t-1"},
		{"billing", "purchases", "--workspace", "t-1"},
		{"billing", "checkout", "--workspace", "t-1", "--pack", "pack", "--operation-id", "op", "--no-browser"},
		{"billing", "portal", "order-1", "--workspace", "t-1", "--no-browser"},
	} {
		if code, _, errOut := keyHarness.run(args...); code != 0 {
			t.Fatalf("API key %q: %d %q", args, code, errOut)
		}
	}
	for _, auth := range keyHarness.seenAuth {
		if auth != "Bearer test-all-access-key" {
			t.Fatalf("billing used unexpected credentials: %q", auth)
		}
	}
}
