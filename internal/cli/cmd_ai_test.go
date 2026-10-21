package cli

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Runzivo/rzv-cli/internal/api"
)

const aiProviderJSON = `{"provider_id":"aip_1","kind":"example_provider","label":"Connection","base_url":null,"key_hint":"hint","has_key":true,"status":"ok","last_tested_at":null,"last_error":null,"created_by":null,"created_at":"2026-10-21T00:00:00Z","updated_at":"2026-10-21T00:00:00Z"}`
const aiSettingsJSON = `{"mode":"byo","agents":{"assistant":{"enabled":true,"provider_id":"aip_1","model":"model-1"}},"autonomy":"ask","monthly_token_limit":null,"monthly_cost_limit_cents":null,"currency":"USD","allowed_roles":["owner"],"byo_available":true,"runzivo_available":false,"can_manage":true,"updated_at":null,"allowed_networks":[]}`

func TestAIProviderCommandsUseCustomerRoutes(t *testing.T) {
	h := selectedHarness(t)
	key := filepath.Join(t.TempDir(), "credential")
	secret := "secret-value-never-output"
	if err := os.WriteFile(key, []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}
	h.json("GET /v1/customer/tenants/t-1/ai/providers", 200, `{"providers":[`+aiProviderJSON+`]}`)
	if code, out, _ := h.run("ai", "providers", "list", "--json"); code != 0 || !strings.Contains(out, `"providers"`) {
		t.Fatalf("list: %d %q", code, out)
	}
	var add map[string]any
	h.routes["POST /v1/customer/tenants/t-1/ai/providers"] = func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&add)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(aiProviderJSON))
	}
	code, out, errOut := h.run("ai", "providers", "add", "--kind", "example_provider", "--label", "Connection", "--key-file", key)
	if code != 0 || add["kind"] != "example_provider" || add["api_key"] != secret || strings.Contains(out+errOut, secret) {
		t.Fatalf("add: %d %q %q %#v", code, out, errOut, add)
	}
	var update map[string]any
	h.routes["PUT /v1/customer/tenants/t-1/ai/providers/aip_1"] = func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&update)
		_, _ = w.Write([]byte(aiProviderJSON))
	}
	if code, _, _ := h.run("ai", "providers", "update", "aip_1", "--label", "Renamed"); code != 0 || update["label"] != "Renamed" || len(update) != 1 {
		t.Fatalf("update: %d %#v", code, update)
	}
	h.routes["DELETE /v1/customer/tenants/t-1/ai/providers/aip_1"] = func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }
	if code, out, _ := h.run("ai", "providers", "remove", "aip_1", "--json"); code != 0 || !strings.Contains(out, `"removed":true`) {
		t.Fatalf("remove: %d %q", code, out)
	}
	h.json("POST /v1/customer/tenants/t-1/ai/providers/aip_1/test", 200, `{"status":"ok","latency_ms":3,"error_code":null}`)
	if code, out, _ := h.run("ai", "providers", "test", "aip_1"); code != 0 || !strings.Contains(out, "3 ms") {
		t.Fatalf("test: %d %q", code, out)
	}
	h.routes["GET /v1/customer/tenants/t-1/ai/providers/aip_1/models"] = func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("refresh") != "true" {
			t.Error("missing refresh")
		}
		_, _ = w.Write([]byte(`{"models":[{"id":"model-1","display_name":"Model","created_at":null,"context_window":null,"recommended":true}],"fetched_at":"2026-10-21T00:00:00Z"}`))
	}
	if code, out, _ := h.run("ai", "providers", "models", "aip_1", "--refresh", "--json"); code != 0 || !strings.Contains(out, `"model-1"`) {
		t.Fatalf("models: %d %q", code, out)
	}
	data, _ := os.ReadFile(filepath.Join(h.store.Dir, "credentials.json"))
	if strings.Contains(string(data), secret) {
		t.Fatal("credential persisted")
	}
}

func TestAISettingsAndUsageCommandsUseCustomerRoutes(t *testing.T) {
	h := selectedHarness(t)
	h.json("GET /v1/customer/tenants/t-1/ai/settings", 200, aiSettingsJSON)
	if code, out, _ := h.run("ai", "settings", "show", "--json"); code != 0 || !strings.Contains(out, `"mode":"byo"`) {
		t.Fatalf("show: %d %q", code, out)
	}
	file := filepath.Join(t.TempDir(), "settings.json")
	patch := `{"monthly_token_limit":null,"agents":{"assistant":{"enabled":true,"provider_id":"aip_1","model":"model-1"}}}`
	if err := os.WriteFile(file, []byte(patch), 0o600); err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	h.routes["PUT /v1/customer/tenants/t-1/ai/settings"] = func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(aiSettingsJSON))
	}
	if code, _, _ := h.run("ai", "settings", "update", "--file", file); code != 0 || body["monthly_token_limit"] != nil || len(body) != 2 {
		t.Fatalf("settings update: %d %#v", code, body)
	}
	h.routes["GET /v1/customer/tenants/t-1/ai/usage/summary"] = func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("from") != "2026-01-01" || r.URL.Query().Get("to") != "2026-01-31" || r.URL.Query().Get("group_by") != "day" {
			t.Errorf("summary query %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"from":"2026-01-01T00:00:00Z","to":"2026-02-01T00:00:00Z","group_by":"day","totals":{"requests":2,"input_tokens":0,"output_tokens":0,"cached_tokens":0,"cost_estimate_micros":null,"unpriced_requests":0,"errors":0,"limited":0,"p50_latency_ms":null,"p95_latency_ms":null},"series":[]}`))
	}
	if code, out, _ := h.run("ai", "usage", "summary", "--from", "2026-01-01", "--to", "2026-01-31", "--group-by", "day", "--json"); code != 0 || !strings.Contains(out, `"requests":2`) {
		t.Fatalf("summary: %d %q", code, out)
	}
	h.routes["GET /v1/customer/tenants/t-1/ai/usage/calls"] = func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("cursor") != "next" || r.URL.Query().Get("limit") != "2" {
			t.Errorf("calls query %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"calls":[{"id":1,"created_at":"2026-10-21T00:00:00Z","agent":"assistant","mode":"byo","provider_kind":"example","model":"model-1","identity_id":null,"input_tokens":1,"output_tokens":2,"cached_tokens":null,"cost_estimate_micros":null,"latency_ms":3,"outcome":"ok","error_code":null}],"next_cursor":"later"}`))
	}
	if code, out, _ := h.run("ai", "usage", "calls", "--cursor", "next", "--limit", "2", "--json"); code != 0 || !strings.Contains(out, `"next_cursor":"later"`) {
		t.Fatalf("calls: %d %q", code, out)
	}
	h.json("GET /v1/customer/tenants/t-1/ai/usage/limit", 200, `{"period_start":"2026-10-01T00:00:00Z","period_end":"2026-11-01T00:00:00Z","used_tokens":4,"used_cost_micros":null,"token_limit":null,"cost_limit_cents":null,"limited":false,"limit_kind":null}`)
	if code, out, _ := h.run("ai", "usage", "limit", "--json"); code != 0 || !strings.Contains(out, `"used_tokens":4`) {
		t.Fatalf("limit: %d %q", code, out)
	}
}

func TestAIAuthorizationAndInvalidResponses(t *testing.T) {
	h := newHarness(t)
	h.env["RZV_API_KEY"] = "key"
	h.env["RZV_API_URL"] = h.api.URL
	h.json("GET /v1/customer/tenants/t-5/ai/providers", 200, `{"providers":[]}`)
	if code, _, _ := h.run("ai", "providers", "list", "--workspace", "t-5"); code != 0 {
		t.Fatalf("key read: %d", code)
	}
	calls := len(h.seenAuth)
	file := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(file, []byte(`{"mode":"off"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"ai", "providers", "add", "--workspace", "t-5", "--kind", "example"},
		{"ai", "providers", "update", "aip_1", "--workspace", "t-5", "--label", "x"},
		{"ai", "providers", "remove", "aip_1", "--workspace", "t-5"},
		{"ai", "providers", "test", "aip_1", "--workspace", "t-5"},
		{"ai", "settings", "update", "--workspace", "t-5", "--file", file},
	} {
		if code, _, _ := h.run(args...); code != exitAccessDenied || len(h.seenAuth) != calls {
			t.Fatalf("key mutation %q: %d calls=%d", args, code, len(h.seenAuth))
		}
	}
	h.json("GET /v1/customer/tenants/t-5/ai/providers", 200, `{}`)
	if code, _, _ := h.run("ai", "providers", "list", "--workspace", "t-5"); code != exitFailure {
		t.Fatalf("malformed: %d", code)
	}
	h.json("GET /v1/customer/tenants/t-5/ai/providers", 403, `{"error":{"code":"permission_required"}}`)
	if code, _, _ := h.run("ai", "providers", "list", "--workspace", "t-5"); code != exitAccessDenied {
		t.Fatalf("403: %d", code)
	}
	notFound := selectedHarness(t)
	notFound.json("GET /v1/customer/tenants/t-1/ai/providers", 404, `{"error":{"code":"not_found"}}`)
	if code, _, _ := notFound.run("ai", "providers", "list"); code != exitNotFound {
		t.Fatalf("404: %d", code)
	}
}

func TestAIRejectsFilesAndBoundsBeforeAuthentication(t *testing.T) {
	h := newHarness(t)
	bad := filepath.Join(t.TempDir(), "bad.json")
	_ = os.WriteFile(bad, []byte(`{"unknown":true}`), 0o600)
	for _, args := range [][]string{{"ai", "usage", "calls", "--limit", "201"}, {"ai", "usage", "summary", "--from", "not-a-date"}, {"ai", "providers", "models", ""}, {"ai", "settings", "update", "--file", bad}} {
		if code, _, _ := h.run(args...); code != exitUsage {
			t.Errorf("%q = %d", args, code)
		}
	}
	if len(h.seenAuth) != 0 {
		t.Fatal("invalid input made a request")
	}
	key := filepath.Join(t.TempDir(), "bad-key")
	if err := os.WriteFile(key, []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := h.run("ai", "providers", "add", "--kind", "example", "--key-file", key); code != exitUsage || strings.Contains(errOut, "short") {
		t.Fatalf("bad key: %d %q", code, errOut)
	}
	oversized := filepath.Join(t.TempDir(), "oversized-key")
	if err := os.WriteFile(oversized, []byte(strings.Repeat("x", 513)), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := h.run("ai", "providers", "add", "--kind", "example", "--key-file", oversized); code != exitUsage {
		t.Fatalf("oversized key: %d", code)
	}
	for _, raw := range []string{"null", `{"mode":"off"} {}`, `{"agents":{"unknown":{"enabled":false,"provider_id":null,"model":null}}}`} {
		file := filepath.Join(t.TempDir(), "invalid-settings.json")
		if err := os.WriteFile(file, []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
		if code, _, _ := h.run("ai", "settings", "update", "--file", file); code != exitUsage {
			t.Fatalf("settings %q: %d", raw, code)
		}
	}
}

func TestAITextOutputIncludesOperationalFields(t *testing.T) {
	provider := api.AIProvider{ProviderID: "aip_1", Kind: "example", Label: "Connection", HasKey: true, Status: "ok", CreatedAt: "created", UpdatedAt: "updated"}
	if text := renderAIProviders("t-1", []api.AIProvider{provider}); !strings.Contains(text, "STATUS") || !strings.Contains(text, "updated") {
		t.Fatalf("providers: %q", text)
	}
	settings := &api.AISettings{Mode: "byo", Autonomy: "ask", Currency: "USD", AllowedRoles: []string{"owner"}, AllowedNetworks: []string{}, Agents: map[string]api.AIAgentSettings{"assistant": {Enabled: true}}}
	if text := renderAISettings("t-1", settings); !strings.Contains(text, "Monthly token limit") || !strings.Contains(text, "Allowed networks") || !strings.Contains(text, "Agents:") {
		t.Fatalf("settings: %q", text)
	}
	totals := api.AIUsageAggregate{Requests: 1, InputTokens: 2, Errors: 3}
	summary := &api.AIUsageSummary{From: "from", To: "to", Totals: &totals, Series: []api.AIUsageAggregate{{Key: "day", Requests: 4}}}
	if text := renderAIUsageSummary("t-1", summary); !strings.Contains(text, "input 2") || !strings.Contains(text, "errors 3") || !strings.Contains(text, "Series:") {
		t.Fatalf("summary: %q", text)
	}
	if text := renderAILimit("t-1", &api.AILimit{PeriodStart: "from", PeriodEnd: "to", UsedTokens: 1}); !strings.Contains(text, "Used cost") || !strings.Contains(text, "Limit kind") {
		t.Fatalf("limit: %q", text)
	}
}
