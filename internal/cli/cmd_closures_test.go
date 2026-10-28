package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const closureJSON = `{"id":"c1","status":"pending","reason":"Need closure","requested_by":{"email":"member@example.test"},"requested_at":"2026-01-01T00:00:00Z","decided_at":"2026-01-02T00:00:00Z","decided_by_type":"staff","decision_note":"Reviewed","version":1}`
const cancelledClosureJSON = `{"id":"c1","status":"cancelled","reason":"Need closure","requested_by":{"email":null},"requested_at":"2026-01-01T00:00:00Z","decided_at":"2026-01-02T00:00:00Z","decided_by_type":"customer","decision_note":null,"version":2}`

func TestWorkspaceClosures(t *testing.T) {
	h := selectedHarness(t)
	h.json("GET /v1/customer/tenants/t-1/closure-requests", 200, `{"requests":[`+closureJSON+`]}`)
	h.routes["POST /v1/customer/tenants/t-1/closure-requests"] = func(w http.ResponseWriter, r *http.Request) {
		var v map[string]string
		_ = json.NewDecoder(r.Body).Decode(&v)
		if v["reason"] != "Need closure" || v["idempotency_key"] != "12345678" {
			t.Error(v)
		}
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"request":` + closureJSON + `}`))
	}
	h.routes["POST /v1/customer/tenants/t-1/closure-requests/c1/cancel"] = func(w http.ResponseWriter, r *http.Request) {
		var v map[string]int64
		_ = json.NewDecoder(r.Body).Decode(&v)
		if v["expected_version"] != 1 {
			t.Error(v)
		}
		_, _ = w.Write([]byte(`{"request":` + cancelledClosureJSON + `}`))
	}
	if code, out, _ := h.run("workspaces", "closures", "list"); code != 0 || !strings.Contains(out, "c1") || !strings.Contains(out, "member@example.test") || !strings.Contains(out, "staff") || !strings.Contains(out, "Reviewed") {
		t.Fatal(out)
	}
	if code, _, _ := h.run("workspaces", "closures", "request", "--reason", "Need closure", "--idempotency-key", "12345678"); code != 0 {
		t.Fatal(code)
	}
	if code, out, _ := h.run("workspaces", "closures", "cancel", "c1", "--expected-version", "1"); code != 0 || !strings.Contains(out, "cancelled") {
		t.Fatal(code)
	}
	h.json("POST /v1/customer/tenants/t-1/closure-requests/c1/cancel", 409, `{"error":{"code":"version_conflict"}}`)
	if code, _, _ := h.run("workspaces", "closures", "cancel", "c1", "--expected-version", "1"); code != exitFailure {
		t.Fatal(code)
	}
}

func TestWorkspaceClosureJSONErrorsAndAPIKey(t *testing.T) {
	h := selectedHarness(t)
	h.json("GET /v1/customer/tenants/t-1/closure-requests", 200, `{"requests":[]}`)
	if code, out, _ := h.run("workspaces", "closures", "list", "--json"); code != 0 || !strings.Contains(out, "requests") {
		t.Fatal(code, out)
	}
	h.json("GET /v1/customer/tenants/t-1/closure-requests", 403, `{"error":{"code":"insufficient_role"}}`)
	if code, _, _ := h.run("workspaces", "closures", "list"); code != exitAccessDenied {
		t.Fatal(code)
	}
	h.json("POST /v1/customer/tenants/t-1/closure-requests/c1/cancel", 404, `{"error":{"code":"not_found"}}`)
	if code, _, _ := h.run("workspaces", "closures", "cancel", "c1", "--expected-version", "1"); code != exitNotFound {
		t.Fatal(code)
	}
	k := newHarness(t)
	k.env["RZV_API_KEY"] = "test-all-key"
	k.env["RZV_API_URL"] = k.api.URL
	k.json("GET /v1/customer/tenants/t-5/closure-requests", 200, `{"requests":[]}`)
	if code, _, _ := k.run("workspaces", "closures", "list", "--workspace", "t-5"); code != 0 || k.seenAuth[0] != "Bearer test-all-key" {
		t.Fatal(code, k.seenAuth)
	}
}
func TestWorkspaceClosureValidationBeforeHTTP(t *testing.T) {
	h := newHarness(t)
	for _, a := range [][]string{{"workspaces", "closures", "request", "--reason", "", "--idempotency-key", "12345678"}, {"workspaces", "closures", "cancel", "c1", "--expected-version", "0"}} {
		if code, _, _ := h.run(a...); code != exitUsage {
			t.Fatal(code)
		}
	}
	if len(h.seenAuth) != 0 {
		t.Fatal("called API")
	}
}
