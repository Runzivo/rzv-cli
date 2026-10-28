package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

type JobLogs struct {
	JobID        string  `json:"job_id"`
	State        string  `json:"state"`
	Reason       *string `json:"reason"`
	Content      *string `json:"content"`
	Truncated    bool    `json:"truncated"`
	ContentBytes int     `json:"content_bytes"`
	MaxBytes     int     `json:"max_bytes"`
	GitHubURL    *string `json:"github_url"`
}

func (v JobLogs) valid() bool {
	return nonEmpty(v.JobID, v.State) && (v.State == "available" || v.State == "not_started" || v.State == "expired" || v.State == "unavailable") && v.MaxBytes > 0 && v.ContentBytes >= 0 && ((v.State == "available") == (v.Content != nil))
}

type InsightsWindow struct {
	WindowStart string `json:"window_start"`
	WindowEnd   string `json:"window_end"`
	WindowDays  int    `json:"window_days"`
	Timezone    string `json:"timezone"`
}

func (v InsightsWindow) valid() bool {
	return nonEmpty(v.WindowStart, v.WindowEnd) && v.WindowDays >= 1 && v.WindowDays <= 90 && v.Timezone == "UTC"
}

type InsightsTotals struct {
	Jobs            int64   `json:"jobs"`
	Minutes         float64 `json:"minutes"`
	JobsWithMinutes int64   `json:"jobs_with_minutes"`
}
type InsightsPercentiles struct {
	Samples       int64    `json:"samples"`
	MedianSeconds *float64 `json:"median_seconds"`
	P90Seconds    *float64 `json:"p90_seconds"`
}
type InsightsStatusCounts struct {
	Queued     int64 `json:"queued"`
	InProgress int64 `json:"in_progress"`
	Completed  int64 `json:"completed"`
	Failed     int64 `json:"failed"`
	Cancelled  int64 `json:"cancelled"`
}
type InsightsConclusionCounts struct {
	Success   int64 `json:"success"`
	Failure   int64 `json:"failure"`
	Cancelled int64 `json:"cancelled"`
	Other     int64 `json:"other"`
	Pending   int64 `json:"pending"`
}
type InsightsJobsDay struct {
	Date        string                   `json:"date"`
	Total       int64                    `json:"total"`
	Status      InsightsStatusCounts     `json:"status"`
	Conclusions InsightsConclusionCounts `json:"conclusions"`
}
type InsightsRunnerMinutes struct {
	RunnerLabel string  `json:"runner_label"`
	VCPUCount   int64   `json:"vcpu_count"`
	MemSizeMiB  int64   `json:"mem_size_mib"`
	Minutes     float64 `json:"minutes"`
	Jobs        int64   `json:"jobs"`
}
type InsightsMinutesDay struct {
	Date    string                  `json:"date"`
	Minutes float64                 `json:"minutes"`
	Runners []InsightsRunnerMinutes `json:"runners"`
}
type InsightsSummary struct {
	InsightsWindow
	Totals       InsightsTotals       `json:"totals"`
	JobsByDay    []InsightsJobsDay    `json:"jobs_by_day"`
	MinutesByDay []InsightsMinutesDay `json:"minutes_by_day"`
	Duration     InsightsPercentiles  `json:"duration"`
	QueueWait    InsightsPercentiles  `json:"queue_wait"`
}
type InsightsHeatmapCell struct {
	Weekday     int      `json:"weekday"`
	Hour        int      `json:"hour"`
	Jobs        int64    `json:"jobs"`
	StartedJobs int64    `json:"started_jobs"`
	MedianWait  *float64 `json:"median_queue_wait_seconds"`
	P90Wait     *float64 `json:"p90_queue_wait_seconds"`
}
type InsightsHeatmap struct {
	InsightsWindow
	Cells []InsightsHeatmapCell `json:"cells"`
}
type InsightsSlowWorkflow struct {
	RepositoryID          string  `json:"repository_id"`
	Repository            *string `json:"repository"`
	Workflow              *string `json:"workflow"`
	Runs                  int64   `json:"runs"`
	MedianDurationSeconds float64 `json:"median_duration_seconds"`
	P90DurationSeconds    float64 `json:"p90_duration_seconds"`
	MaxDurationSeconds    float64 `json:"max_duration_seconds"`
	LastRunnerLabel       *string `json:"last_runner_label"`
}
type InsightsSlowJobs struct {
	InsightsWindow
	Limit     int                    `json:"limit"`
	Workflows []InsightsSlowWorkflow `json:"workflows"`
}

func insightQuery(days, limit int) url.Values {
	q := url.Values{}
	if days != 0 {
		q.Set("days", strconv.Itoa(days))
	}
	if limit != 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	return q
}

func decodeRequiredObject(raw json.RawMessage, out any, fields ...string) bool {
	return requiredObjectFields(raw, fields...) && json.Unmarshal(raw, out) == nil
}

func requiredObjectFields(raw json.RawMessage, fields ...string) bool {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || object == nil {
		return false
	}
	for _, field := range fields {
		if _, found := object[field]; !found {
			return false
		}
	}
	return true
}

func requiredArray(raw json.RawMessage) ([]json.RawMessage, bool) {
	var values []json.RawMessage
	if json.Unmarshal(raw, &values) != nil || values == nil {
		return nil, false
	}
	return values, true
}

func summaryShapeValid(raw json.RawMessage) bool {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil ||
		!requiredObjectFields(object["totals"], "jobs", "minutes", "jobs_with_minutes") ||
		!requiredObjectFields(object["duration"], "samples", "median_seconds", "p90_seconds") ||
		!requiredObjectFields(object["queue_wait"], "samples", "median_seconds", "p90_seconds") {
		return false
	}
	jobs, ok := requiredArray(object["jobs_by_day"])
	if !ok {
		return false
	}
	for _, job := range jobs {
		var day map[string]json.RawMessage
		if json.Unmarshal(job, &day) != nil || !requiredObjectFields(job, "date", "total", "status", "conclusions") ||
			!requiredObjectFields(day["status"], "queued", "in_progress", "completed", "failed", "cancelled") ||
			!requiredObjectFields(day["conclusions"], "success", "failure", "cancelled", "other", "pending") {
			return false
		}
	}
	minutes, ok := requiredArray(object["minutes_by_day"])
	if !ok {
		return false
	}
	for _, minute := range minutes {
		var day map[string]json.RawMessage
		if json.Unmarshal(minute, &day) != nil || !requiredObjectFields(minute, "date", "minutes", "runners") {
			return false
		}
		runners, ok := requiredArray(day["runners"])
		if !ok {
			return false
		}
		for _, runner := range runners {
			if !requiredObjectFields(runner, "runner_label", "vcpu_count", "mem_size_mib", "minutes", "jobs") {
				return false
			}
		}
	}
	return true
}

func heatmapShapeValid(raw json.RawMessage) bool {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil {
		return false
	}
	cells, ok := requiredArray(object["cells"])
	if !ok {
		return false
	}
	for _, cell := range cells {
		if !requiredObjectFields(cell, "weekday", "hour", "jobs", "started_jobs", "median_queue_wait_seconds", "p90_queue_wait_seconds") {
			return false
		}
	}
	return true
}

func slowJobsShapeValid(raw json.RawMessage) bool {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil {
		return false
	}
	workflows, ok := requiredArray(object["workflows"])
	if !ok {
		return false
	}
	for _, workflow := range workflows {
		if !requiredObjectFields(workflow, "repository_id", "repository", "workflow", "runs", "median_duration_seconds", "p90_duration_seconds", "max_duration_seconds", "last_runner_label") {
			return false
		}
	}
	return true
}

func (c *Client) JobLogs(ctx context.Context, tenantID, jobID string) (*JobLogs, error) {
	var v struct {
		Logs json.RawMessage `json:"logs"`
	}
	if err := c.do(ctx, http.MethodGet, tenantPath(tenantID, "/job/logs"), url.Values{"jobId": {jobID}}, nil, &v); err != nil {
		return nil, err
	}
	var logs JobLogs
	if err := required(decodeRequiredObject(v.Logs, &logs, "job_id", "state", "reason", "content", "truncated", "content_bytes", "max_bytes", "github_url") && logs.valid()); err != nil {
		return nil, err
	}
	return &logs, nil
}
func (c *Client) InsightsSummary(ctx context.Context, tenantID string, days int) (*InsightsSummary, error) {
	var v struct {
		Summary json.RawMessage `json:"summary"`
	}
	if err := c.do(ctx, http.MethodGet, tenantPath(tenantID, "/insights/summary"), insightQuery(days, 0), nil, &v); err != nil {
		return nil, err
	}
	var summary InsightsSummary
	if err := required(decodeRequiredObject(v.Summary, &summary, "window_start", "window_end", "window_days", "timezone", "totals", "jobs_by_day", "minutes_by_day", "duration", "queue_wait") && summaryShapeValid(v.Summary) && summary.InsightsWindow.valid() && summary.JobsByDay != nil && summary.MinutesByDay != nil); err != nil {
		return nil, err
	}
	return &summary, nil
}
func (c *Client) InsightsHeatmap(ctx context.Context, tenantID string, days int) (*InsightsHeatmap, error) {
	var v struct {
		Heatmap json.RawMessage `json:"heatmap"`
	}
	if err := c.do(ctx, http.MethodGet, tenantPath(tenantID, "/insights/heatmap"), insightQuery(days, 0), nil, &v); err != nil {
		return nil, err
	}
	var heatmap InsightsHeatmap
	if err := required(decodeRequiredObject(v.Heatmap, &heatmap, "window_start", "window_end", "window_days", "timezone", "cells") && heatmapShapeValid(v.Heatmap) && heatmap.InsightsWindow.valid() && len(heatmap.Cells) == 168); err != nil {
		return nil, err
	}
	for index, cell := range heatmap.Cells {
		if err := required(cell.Weekday == index/24+1 && cell.Hour == index%24); err != nil {
			return nil, err
		}
	}
	return &heatmap, nil
}
func (c *Client) InsightsSlowJobs(ctx context.Context, tenantID string, days, limit int) (*InsightsSlowJobs, error) {
	var v struct {
		Slow json.RawMessage `json:"slow_jobs"`
	}
	if err := c.do(ctx, http.MethodGet, tenantPath(tenantID, "/insights/slow-jobs"), insightQuery(days, limit), nil, &v); err != nil {
		return nil, err
	}
	var slow InsightsSlowJobs
	if err := required(decodeRequiredObject(v.Slow, &slow, "window_start", "window_end", "window_days", "timezone", "limit", "workflows") && slowJobsShapeValid(v.Slow) && slow.InsightsWindow.valid() && slow.Limit >= 1 && slow.Limit <= 25 && slow.Workflows != nil); err != nil {
		return nil, err
	}
	return &slow, nil
}
