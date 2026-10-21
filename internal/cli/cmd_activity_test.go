package cli

import (
	"net/http"
	"strings"
	"testing"
)

const cliActivityJSON = `{"events":[{"id":"1","occurred_at":"2026-01-01T00:00:00Z","category":"login","action":"USER.LOGIN","description":"Signed in","result":"failure","error_code":"DENIED","actor":{"type":"api_key","label":"Customer","email":null,"api_key_id":null},"auth_method":"oidc_token","target":null,"ip":null,"user_agent":null,"request_id":null,"session_ref":null}],"next_cursor":null,"scope":"workspace","retention_days":180}`
const cliAuditJSON = `{"period_days":30,"generated_at":"2026-01-01T00:00:00Z","history_recorded":false,"components":[{"id":"runners","name":"Runners","state":"unknown","detail":"No signal","last_signal_at":null,"uptime_percent":null}],"assurances":{"jobs_run":1,"fresh_microvms":2,"destroyed":3,"clean_teardowns":4,"forced_stops":5,"awaiting_teardown":6},"evidence":[{"job_id":"7.1","repository":null,"lease_id":"lease","booted_at":"2026-01-01T00:00:00Z","destroyed_at":null,"teardown":"pending"}]}`
const cliCSV = "occurred_at,category,action,description,result,error_code,actor_type,actor,actor_email,api_key_id,auth_method,target_type,target_id,ip,user_agent,request_id,session_ref\n"

func TestActivityCommands(t *testing.T) {
	h := selectedHarness(t)
	h.json("GET /v1/customer/tenants/t-1/activity", 200, cliActivityJSON)
	h.routes["GET /v1/customer/tenants/t-1/activity/export"] = func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("cursor") != "" {
			t.Error("cursor exported")
		}
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte(cliCSV))
	}
	h.json("GET /v1/customer/tenants/t-1/audit-status", 200, cliAuditJSON)
	if code, out, _ := h.run("activity", "list", "--category", "login", "--limit", "4"); code != 0 || !strings.Contains(out, "ACTOR TYPE") || !strings.Contains(out, "api_key") || !strings.Contains(out, "DENIED") {
		t.Fatalf("list %d %q", code, out)
	}
	if code, out, _ := h.run("activity", "export", "--query", "sign"); code != 0 || out != cliCSV {
		t.Fatalf("export %d %q", code, out)
	}
	if code, out, _ := h.run("activity", "export", "--json"); code != 0 || !strings.Contains(out, "csv") {
		t.Fatalf("export json %d %q", code, out)
	}
	if code, out, _ := h.run("activity", "status", "--period-days", "30"); code != 0 || !strings.Contains(out, "uptime unknown") || !strings.Contains(out, "clean teardowns 4") || !strings.Contains(out, "job 7.1") || !strings.Contains(out, "Generated:") {
		t.Fatalf("status %d %q", code, out)
	}
}
func TestActivityInvalidBeforeNetwork(t *testing.T) {
	h := newHarness(t)
	for _, args := range [][]string{{"activity", "list", "--limit", "0"}, {"activity", "list", "--since", "bad"}, {"activity", "export", "--cursor", "x"}, {"activity", "status", "--period-days", "8"}} {
		if code, _, _ := h.run(args...); code != exitUsage {
			t.Errorf("%q = %d", args, code)
		}
	}
	if len(h.seenAuth) != 0 {
		t.Fatal("invalid input called API")
	}
}

func TestActivityUsesWorkspaceAPIKey(t *testing.T) {
	h := newHarness(t)
	h.env["RZV_API_KEY"] = "test-audit-key"
	h.env["RZV_API_URL"] = h.api.URL
	h.json("GET /v1/customer/tenants/t-5/activity", http.StatusOK, cliActivityJSON)
	if code, _, errOut := h.run("activity", "list", "--workspace", "t-5"); code != 0 || errOut != "" {
		t.Fatalf("exit=%d stderr=%q", code, errOut)
	}
	if len(h.seenAuth) != 1 || h.seenAuth[0] != "Bearer test-audit-key" {
		t.Fatalf("auth=%q", h.seenAuth)
	}
}
