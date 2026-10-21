package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestInsightsAndLogsWire(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer test-credential" {
			t.Errorf("unexpected request: %s %v", r.Method, r.Header)
		}
		switch r.URL.Path {
		case "/v1/customer/tenants/t/job/logs":
			if got := r.URL.Query(); got.Get("jobId") != "j 1" || len(got) != 1 {
				t.Errorf("logs query: %q", got.Encode())
			}
			_, _ = w.Write([]byte(`{"logs":{"job_id":"j 1","state":"available","reason":null,"content":"line","truncated":false,"content_bytes":4,"max_bytes":10,"github_url":null}}`))
		case "/v1/customer/tenants/t/insights/summary":
			if got := r.URL.Query(); got.Get("days") != "7" || len(got) != 1 {
				t.Errorf("summary query: %q", got.Encode())
			}
			_, _ = w.Write([]byte(insightSummaryBody()))
		case "/v1/customer/tenants/t/insights/heatmap":
			if got := r.URL.Query(); got.Get("days") != "7" || len(got) != 1 {
				t.Errorf("heatmap query: %q", got.Encode())
			}
			_, _ = w.Write([]byte(insightHeatmapBody()))
		case "/v1/customer/tenants/t/insights/slow-jobs":
			if got := r.URL.Query(); got.Get("days") != "7" || got.Get("limit") != "2" || len(got) != 2 {
				t.Errorf("slow jobs query: %q", got.Encode())
			}
			_, _ = w.Write([]byte(`{"slow_jobs":{"window_start":"a","window_end":"b","window_days":7,"timezone":"UTC","limit":2,"workflows":[]}}`))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	})
	ctx := context.Background()
	if _, err := c.JobLogs(ctx, "t", "j 1"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.InsightsSummary(ctx, "t", 7); err != nil {
		t.Fatal(err)
	}
	if got, err := c.InsightsHeatmap(ctx, "t", 7); err != nil || len(got.Cells) != 168 {
		t.Fatalf("heatmap: %#v, %v", got, err)
	}
	if _, err := c.InsightsSlowJobs(ctx, "t", 7, 2); err != nil {
		t.Fatal(err)
	}
}

func insightSummaryBody() string {
	return `{"summary":{"window_start":"a","window_end":"b","window_days":7,"timezone":"UTC","totals":{"jobs":1,"minutes":2,"jobs_with_minutes":1},"jobs_by_day":[{"date":"2026-10-20","total":0,"status":{"queued":0,"in_progress":0,"completed":0,"failed":0,"cancelled":0},"conclusions":{"success":0,"failure":0,"cancelled":0,"other":0,"pending":0}}],"minutes_by_day":[{"date":"2026-10-20","minutes":0,"runners":[{"runner_label":"r","vcpu_count":0,"mem_size_mib":0,"minutes":0,"jobs":0}]}],"duration":{"samples":0,"median_seconds":null,"p90_seconds":null},"queue_wait":{"samples":0,"median_seconds":null,"p90_seconds":null}}}`
}

func insightHeatmapBody() string {
	var cells strings.Builder
	cells.WriteByte('[')
	for weekday := 1; weekday <= 7; weekday++ {
		for hour := 0; hour < 24; hour++ {
			if cells.Len() > 1 {
				cells.WriteByte(',')
			}
			fmt.Fprintf(&cells, `{"weekday":%d,"hour":%d,"jobs":0,"started_jobs":0,"median_queue_wait_seconds":null,"p90_queue_wait_seconds":null}`, weekday, hour)
		}
	}
	cells.WriteByte(']')
	return `{"heatmap":{"window_start":"a","window_end":"b","window_days":7,"timezone":"UTC","cells":` + cells.String() + `}}`
}

func TestInsightsAndLogsRejectMalformedRequiredShapes(t *testing.T) {
	cases := []struct {
		name string
		body string
		call func(*Client) error
	}{
		{"logs", `{"logs":{"job_id":"j","state":"available","content":null,"max_bytes":1}}`, func(c *Client) error { _, err := c.JobLogs(context.Background(), "t", "j"); return err }},
		{"summary", `{"summary":{"window_start":"a","window_end":"b","window_days":7,"timezone":"UTC"}}`, func(c *Client) error { _, err := c.InsightsSummary(context.Background(), "t", 7); return err }},
		{"summary totals", strings.Replace(insightSummaryBody(), `"totals":{"jobs":1,"minutes":2,"jobs_with_minutes":1}`, `"totals":{}`, 1), func(c *Client) error { _, err := c.InsightsSummary(context.Background(), "t", 7); return err }},
		{"summary status", strings.Replace(insightSummaryBody(), `"status":{"queued":0,"in_progress":0,"completed":0,"failed":0,"cancelled":0}`, `"status":{}`, 1), func(c *Client) error { _, err := c.InsightsSummary(context.Background(), "t", 7); return err }},
		{"summary conclusions", strings.Replace(insightSummaryBody(), `"conclusions":{"success":0,"failure":0,"cancelled":0,"other":0,"pending":0}`, `"conclusions":{}`, 1), func(c *Client) error { _, err := c.InsightsSummary(context.Background(), "t", 7); return err }},
		{"summary runner", strings.Replace(insightSummaryBody(), `{"runner_label":"r","vcpu_count":0,"mem_size_mib":0,"minutes":0,"jobs":0}`, `{}`, 1), func(c *Client) error { _, err := c.InsightsSummary(context.Background(), "t", 7); return err }},
		{"heatmap", `{"heatmap":{"window_start":"a","window_end":"b","window_days":7,"timezone":"UTC","cells":[]}}`, func(c *Client) error { _, err := c.InsightsHeatmap(context.Background(), "t", 7); return err }},
		{"heatmap order", strings.Replace(insightHeatmapBody(), `"weekday":1`, `"weekday":7`, 1), func(c *Client) error { _, err := c.InsightsHeatmap(context.Background(), "t", 7); return err }},
		{"heatmap cell", strings.Replace(insightHeatmapBody(), `{"weekday":1,"hour":0,"jobs":0,"started_jobs":0,"median_queue_wait_seconds":null,"p90_queue_wait_seconds":null}`, `{"weekday":1,"hour":0}`, 1), func(c *Client) error { _, err := c.InsightsHeatmap(context.Background(), "t", 7); return err }},
		{"slow jobs", `{"slow_jobs":{"window_start":"a","window_end":"b","window_days":7,"timezone":"UTC","limit":2}}`, func(c *Client) error { _, err := c.InsightsSlowJobs(context.Background(), "t", 7, 2); return err }},
		{"slow workflow", `{"slow_jobs":{"window_start":"a","window_end":"b","window_days":7,"timezone":"UTC","limit":2,"workflows":[{}]}}`, func(c *Client) error { _, err := c.InsightsSlowJobs(context.Background(), "t", 7, 2); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(tc.body)) })
			var apiErr *Error
			if err := tc.call(c); !errors.As(err, &apiErr) || apiErr.Code != "malformed_response" {
				t.Fatalf("want malformed_response, got %v", err)
			}
		})
	}
}
