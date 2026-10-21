package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type ActivityFilter struct {
	Category, ActorType, Result, Query, Since, Until, Cursor string
	Limit                                                    int
}

func (f ActivityFilter) values(paging bool) url.Values {
	v := url.Values{}
	for _, pair := range [][2]string{{"category", f.Category}, {"actor_type", f.ActorType}, {"result", f.Result}, {"q", f.Query}, {"since", f.Since}, {"until", f.Until}} {
		if pair[1] != "" {
			v.Set(pair[0], pair[1])
		}
	}
	if paging && f.Limit != 0 {
		v.Set("limit", fmt.Sprint(f.Limit))
	}
	if paging && f.Cursor != "" {
		v.Set("cursor", f.Cursor)
	}
	return v
}

type ActivityActor struct {
	Type     string  `json:"type"`
	Label    string  `json:"label"`
	Email    *string `json:"email"`
	APIKeyID *string `json:"api_key_id"`
	Issuer   string  `json:"issuer"`
	Subject  string  `json:"subject"`
}
type ActivityTarget struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}
type ActivityEvent struct {
	ID          string          `json:"id"`
	OccurredAt  string          `json:"occurred_at"`
	Category    string          `json:"category"`
	Action      string          `json:"action"`
	Description string          `json:"description"`
	Result      string          `json:"result"`
	ErrorCode   *string         `json:"error_code"`
	Actor       ActivityActor   `json:"actor"`
	AuthMethod  string          `json:"auth_method"`
	Target      *ActivityTarget `json:"target"`
	IP          *string         `json:"ip"`
	UserAgent   *string         `json:"user_agent"`
	RequestID   *string         `json:"request_id"`
	SessionRef  *string         `json:"session_ref"`
}
type ActivityPage struct {
	Events        []ActivityEvent `json:"events"`
	NextCursor    *string         `json:"next_cursor"`
	Scope         string          `json:"scope"`
	RetentionDays int             `json:"retention_days"`
}

func (p *ActivityPage) UnmarshalJSON(data []byte) error {
	type plain ActivityPage
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if _, ok := raw["events"]; !ok {
		return fmt.Errorf("missing events")
	}
	if _, ok := raw["next_cursor"]; !ok {
		return fmt.Errorf("missing next_cursor")
	}
	if _, ok := raw["retention_days"]; !ok {
		return fmt.Errorf("missing retention_days")
	}
	var events []map[string]json.RawMessage
	if err := json.Unmarshal(raw["events"], &events); err != nil {
		return err
	}
	for _, event := range events {
		for _, key := range []string{"id", "occurred_at", "category", "action", "description", "result", "error_code", "actor", "auth_method", "target", "ip", "user_agent", "request_id", "session_ref"} {
			if !present(event, key) {
				return fmt.Errorf("missing event %s", key)
			}
		}
		for _, key := range []string{"id", "occurred_at", "category", "action", "description", "result", "actor", "auth_method"} {
			if !nonNull(event[key]) {
				return fmt.Errorf("null event %s", key)
			}
		}
		var actor map[string]json.RawMessage
		if err := json.Unmarshal(event["actor"], &actor); err != nil {
			return err
		}
		for _, key := range []string{"type", "label", "email", "api_key_id"} {
			if !present(actor, key) {
				return fmt.Errorf("missing actor %s", key)
			}
		}
		for _, key := range []string{"type", "label"} {
			if !nonNull(actor[key]) {
				return fmt.Errorf("null actor %s", key)
			}
		}
		if nonNull(event["target"]) {
			var target map[string]json.RawMessage
			if err := json.Unmarshal(event["target"], &target); err != nil {
				return err
			}
			for _, key := range []string{"type", "id"} {
				if !present(target, key) || !nonNull(target[key]) {
					return fmt.Errorf("invalid target %s", key)
				}
			}
		}
	}
	return json.Unmarshal(data, (*plain)(p))
}

func (c *Client) Activity(ctx context.Context, tenantID string, filter ActivityFilter) (*ActivityPage, error) {
	var out ActivityPage
	if err := c.do(ctx, http.MethodGet, tenantPath(tenantID, "/activity"), filter.values(true), nil, &out); err != nil {
		return nil, err
	}
	if err := required(out.Events != nil && out.RetentionDays > 0); err != nil {
		return nil, err
	}
	for _, event := range out.Events {
		if err := required(nonEmpty(event.ID, event.OccurredAt, event.Category, event.Action, event.Result, event.Actor.Type, event.Actor.Label, event.AuthMethod)); err != nil {
			return nil, err
		}
	}
	return &out, nil
}

func (c *Client) ExportActivity(ctx context.Context, tenantID string, filter ActivityFilter) (string, error) {
	data, err := c.doCSV(ctx, tenantPath(tenantID, "/activity/export"), filter.values(false))
	return string(data), err
}

type AuditComponent struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	State         string   `json:"state"`
	Detail        string   `json:"detail"`
	LastSignalAt  *string  `json:"last_signal_at"`
	UptimePercent *float64 `json:"uptime_percent"`
}
type ExecutionAssurance struct {
	JobsRun          int64 `json:"jobs_run"`
	FreshMicroVMs    int64 `json:"fresh_microvms"`
	Destroyed        int64 `json:"destroyed"`
	CleanTeardowns   int64 `json:"clean_teardowns"`
	ForcedStops      int64 `json:"forced_stops"`
	AwaitingTeardown int64 `json:"awaiting_teardown"`
}
type LeaseEvidence struct {
	JobID       string  `json:"job_id"`
	Repository  *string `json:"repository"`
	LeaseID     string  `json:"lease_id"`
	BootedAt    string  `json:"booted_at"`
	DestroyedAt *string `json:"destroyed_at"`
	Teardown    string  `json:"teardown"`
}
type AuditStatus struct {
	PeriodDays      int                `json:"period_days"`
	GeneratedAt     string             `json:"generated_at"`
	Components      []AuditComponent   `json:"components"`
	HistoryRecorded bool               `json:"history_recorded"`
	Assurances      ExecutionAssurance `json:"assurances"`
	Evidence        []LeaseEvidence    `json:"evidence"`
}

func (s *AuditStatus) UnmarshalJSON(data []byte) error {
	type plain AuditStatus
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for _, key := range []string{"period_days", "generated_at", "components", "history_recorded", "assurances", "evidence"} {
		if _, ok := raw[key]; !ok {
			return fmt.Errorf("missing %s", key)
		}
	}
	var assurances map[string]json.RawMessage
	if err := json.Unmarshal(raw["assurances"], &assurances); err != nil {
		return err
	}
	for _, key := range []string{"jobs_run", "fresh_microvms", "destroyed", "clean_teardowns", "forced_stops", "awaiting_teardown"} {
		if !present(assurances, key) || !nonNull(assurances[key]) {
			return fmt.Errorf("missing %s", key)
		}
	}
	if !nonNull(raw["history_recorded"]) {
		return fmt.Errorf("null history_recorded")
	}
	var components []map[string]json.RawMessage
	if err := json.Unmarshal(raw["components"], &components); err != nil {
		return err
	}
	for _, component := range components {
		for _, key := range []string{"id", "name", "state", "detail", "last_signal_at", "uptime_percent"} {
			if !present(component, key) {
				return fmt.Errorf("missing component %s", key)
			}
		}
	}
	var evidence []map[string]json.RawMessage
	if err := json.Unmarshal(raw["evidence"], &evidence); err != nil {
		return err
	}
	for _, item := range evidence {
		for _, key := range []string{"job_id", "repository", "lease_id", "booted_at", "destroyed_at", "teardown"} {
			if !present(item, key) {
				return fmt.Errorf("missing evidence %s", key)
			}
		}
		for _, key := range []string{"job_id", "lease_id", "booted_at", "teardown"} {
			if !nonNull(item[key]) {
				return fmt.Errorf("null evidence %s", key)
			}
		}
	}
	return json.Unmarshal(data, (*plain)(s))
}

func present(values map[string]json.RawMessage, key string) bool { _, ok := values[key]; return ok }
func nonNull(value json.RawMessage) bool                         { return len(value) > 0 && string(value) != "null" }

func (c *Client) AuditStatus(ctx context.Context, tenantID string, periodDays int) (*AuditStatus, error) {
	q := url.Values{}
	if periodDays != 0 {
		q.Set("period_days", fmt.Sprint(periodDays))
	}
	var out AuditStatus
	if err := c.do(ctx, http.MethodGet, tenantPath(tenantID, "/audit-status"), q, nil, &out); err != nil {
		return nil, err
	}
	if err := required((out.PeriodDays == 7 || out.PeriodDays == 30 || out.PeriodDays == 60) && out.GeneratedAt != "" && out.Components != nil && out.Evidence != nil && out.Assurances.JobsRun >= 0 && out.Assurances.FreshMicroVMs >= 0 && out.Assurances.Destroyed >= 0 && out.Assurances.CleanTeardowns >= 0 && out.Assurances.ForcedStops >= 0 && out.Assurances.AwaitingTeardown >= 0); err != nil {
		return nil, err
	}
	for _, component := range out.Components {
		if err := required(nonEmpty(component.ID, component.Name, component.State, component.Detail)); err != nil {
			return nil, err
		}
	}
	for _, evidence := range out.Evidence {
		if err := required(nonEmpty(evidence.JobID, evidence.LeaseID, evidence.BootedAt, evidence.Teardown)); err != nil {
			return nil, err
		}
	}
	return &out, nil
}
