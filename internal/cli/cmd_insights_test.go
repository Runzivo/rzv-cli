package cli

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

const insightSummaryJSON = `{"summary":{"window_start":"2026-10-15T00:00:00Z","window_end":"2026-10-21T12:00:00Z","window_days":7,"timezone":"UTC","totals":{"jobs":3,"minutes":4.5,"jobs_with_minutes":2},"jobs_by_day":[{"date":"2026-10-20","total":3,"status":{"queued":1,"in_progress":0,"completed":1,"failed":1,"cancelled":0},"conclusions":{"success":1,"failure":1,"cancelled":0,"other":0,"pending":1}}],"minutes_by_day":[{"date":"2026-10-20","minutes":4.5,"runners":[{"runner_label":"small","vcpu_count":2,"mem_size_mib":8192,"minutes":4.5,"jobs":2}]}],"duration":{"samples":2,"median_seconds":12,"p90_seconds":20},"queue_wait":{"samples":0,"median_seconds":null,"p90_seconds":null}}}`

func insightHeatmapJSON() string {
	cells := make([]string, 168)
	for index := range cells {
		cells[index] = fmt.Sprintf(`{"weekday":%d,"hour":%d,"jobs":1,"started_jobs":0,"median_queue_wait_seconds":null,"p90_queue_wait_seconds":null}`, index/24+1, index%24)
	}
	return `{"heatmap":{"window_start":"2026-10-15T00:00:00Z","window_end":"2026-10-21T12:00:00Z","window_days":7,"timezone":"UTC","cells":[` + strings.Join(cells, ",") + `]}}`
}

func TestInsightsAndLogsCommands(t *testing.T) {
	h := selectedHarness(t)
	h.json("GET /v1/customer/tenants/t-1/job/logs", 200, `{"logs":{"job_id":"j-1","state":"available","reason":null,"content":"line\nliteral\\u000anext\u001b[31m","truncated":true,"content_bytes":5,"max_bytes":10,"github_url":"https://example.test/\u001b"}}`)
	if code, out, _ := h.run("jobs", "logs", "j-1"); code != 0 || !strings.Contains(out, "line\nliteral\\u000anext\\u001b[31m") || !strings.Contains(out, "[log truncated]") || !strings.Contains(out, "Full log URL: https://example.test/\\u001b") || strings.Contains(out, "\x1b") {
		t.Fatalf("logs: %d %q", code, out)
	}
	h.json("GET /v1/customer/tenants/t-1/insights/summary", 200, insightSummaryJSON)
	h.json("GET /v1/customer/tenants/t-1/insights/heatmap", 200, insightHeatmapJSON())
	h.json("GET /v1/customer/tenants/t-1/insights/slow-jobs", 200, `{"slow_jobs":{"window_start":"2026-10-15T00:00:00Z","window_end":"2026-10-21T12:00:00Z","window_days":7,"timezone":"UTC","limit":2,"workflows":[{"repository_id":"1","repository":"acme/app","workflow":"CI","runs":2,"median_duration_seconds":15,"p90_duration_seconds":30,"max_duration_seconds":40,"last_runner_label":"small"}]}}`)

	cases := []struct {
		name string
		args []string
		want []string
	}{
		{"summary text", []string{"insights", "summary", "--days", "7"}, []string{"Duration (2 samples): median 12s, p90 20s", "Queue wait (0 samples): median -, p90 -", "Jobs by day:", "DATE"}},
		{"heatmap text", []string{"insights", "heatmap", "--days", "7"}, []string{"Insights heatmap for workspace t-1 (7 days, UTC):", "STARTED JOBS", "MEDIAN WAIT", "00:00"}},
		{"slow jobs text", []string{"insights", "slow-jobs", "--days", "7", "--limit", "2"}, []string{"Slow jobs for workspace t-1 (7 days, UTC):", "RANK", "P90 DURATION", "acme/app"}},
		{"logs text", []string{"jobs", "logs", "j-1"}, []string{"Logs for job j-1 (available):"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, out, errOut := h.run(tc.args...)
			if code != 0 || errOut != "" {
				t.Fatalf("%s: code=%d out=%q err=%q", tc.name, code, out, errOut)
			}
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Fatalf("%s missing %q in %q", tc.name, want, out)
				}
			}
		})
	}
	for _, args := range [][]string{{"insights", "summary", "--json"}, {"insights", "heatmap", "--json"}, {"insights", "slow-jobs", "--limit", "2", "--json"}, {"jobs", "logs", "j-1", "--json"}} {
		code, out, errOut := h.run(args...)
		if code != 0 || errOut != "" || !strings.Contains(out, `"workspace_id":"t-1"`) {
			t.Fatalf("json %v: code=%d out=%q err=%q", args, code, out, errOut)
		}
	}
	h.json("GET /v1/customer/tenants/t-1/job/logs", 200, `{"logs":{"job_id":"j-1","state":"unavailable","reason":"provider busy","content":null,"truncated":false,"content_bytes":0,"max_bytes":10,"github_url":"https://example.test/full"}}`)
	if code, out, _ := h.run("jobs", "logs", "j-1"); code != 0 || !strings.Contains(out, "Logs are not available. Reason: provider busy\nFull log URL: https://example.test/full") {
		t.Fatalf("unavailable log state: %d %q", code, out)
	}

	h.json("GET /v1/customer/tenants/t-1/job/logs", 503, `{"error":{"code":"job_logs_unavailable"}}`)
	if code, _, _ := h.run("jobs", "logs", "j-1"); code != exitUnavailable {
		t.Fatalf("unavailable logs: %d", code)
	}
	h.json("GET /v1/customer/tenants/t-1/insights/summary", 200, `{"summary":{"window_start":"a","window_end":"b","window_days":7,"timezone":"UTC"}}`)
	if code, _, _ := h.run("insights", "summary"); code != exitFailure {
		t.Fatalf("malformed summary: %d", code)
	}
	h.json("GET /v1/customer/tenants/t-1/insights/heatmap", http.StatusForbidden, `{"error":{"code":"tenant_membership_required"}}`)
	if code, _, _ := h.run("insights", "heatmap"); code != exitAccessDenied {
		t.Fatalf("heatmap forbidden: %d", code)
	}
	h.json("GET /v1/customer/tenants/t-1/job/logs", http.StatusNotFound, `{"error":{"code":"not_found"}}`)
	if code, _, _ := h.run("jobs", "logs", "missing"); code != exitNotFound {
		t.Fatalf("missing logs: %d", code)
	}
}

func TestInsightsValidationPrecedesAuthentication(t *testing.T) {
	h := newHarness(t)
	for _, args := range [][]string{
		{"insights", "nope"},
		{"insights", "summary", "--days", "91"},
		{"insights", "slow-jobs", "--limit", "26"},
	} {
		if code, _, _ := h.run(args...); code != exitUsage {
			t.Fatalf("%v: %d", args, code)
		}
	}
	if len(h.seenAuth) != 0 {
		t.Fatalf("validation made HTTP requests: %v", h.seenAuth)
	}
}

func TestInsightsAPIKey(t *testing.T) {
	h := newHarness(t)
	h.env["RZV_API_KEY"] = "rzv_test_key"
	h.env["RZV_API_URL"] = h.api.URL
	h.json("GET /v1/customer/tenants/t-5/insights/summary", 200, insightSummaryJSON)
	if code, out, errOut := h.run("insights", "summary", "--workspace", "t-5"); code != 0 || errOut != "" || !strings.Contains(out, "workspace t-5") {
		t.Fatalf("api key insights: %d %q %q", code, out, errOut)
	}
	if len(h.seenAuth) == 0 || h.seenAuth[len(h.seenAuth)-1] != "Bearer rzv_test_key" {
		t.Fatalf("API key not sent: %v", h.seenAuth)
	}
}
