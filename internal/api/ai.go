package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
)

func aiRequiredObject(raw []byte, names ...string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, errors.New("not object")
	}
	for _, name := range names {
		if _, ok := fields[name]; !ok {
			return nil, errors.New("required field")
		}
	}
	return fields, nil
}
func aiNonNull(fields map[string]json.RawMessage, name string) error {
	if string(fields[name]) == "null" {
		return errors.New("null required field")
	}
	return nil
}

type AIProvider struct {
	ProviderID   string  `json:"provider_id"`
	Kind         string  `json:"kind"`
	Label        string  `json:"label"`
	BaseURL      *string `json:"base_url"`
	KeyHint      *string `json:"key_hint"`
	HasKey       bool    `json:"has_key"`
	Status       string  `json:"status"`
	LastTestedAt *string `json:"last_tested_at"`
	LastError    *string `json:"last_error"`
	CreatedBy    *string `json:"created_by"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
}
type AIProviderInput struct {
	Kind    string  `json:"kind,omitempty"`
	Label   *string `json:"label,omitempty"`
	BaseURL *string `json:"base_url,omitempty"`
	APIKey  *string `json:"api_key,omitempty"`
}
type AIModel struct {
	ID            string  `json:"id"`
	DisplayName   string  `json:"display_name"`
	CreatedAt     *string `json:"created_at"`
	ContextWindow *int64  `json:"context_window"`
	Recommended   bool    `json:"recommended"`
}
type AITestResult struct {
	Status    string  `json:"status"`
	LatencyMS int64   `json:"latency_ms"`
	ErrorCode *string `json:"error_code"`
}
type AIAgentSettings struct {
	Enabled    bool    `json:"enabled"`
	ProviderID *string `json:"provider_id"`
	Model      *string `json:"model"`
}
type AISettings struct {
	Mode                  string                     `json:"mode"`
	Agents                map[string]AIAgentSettings `json:"agents"`
	Autonomy              string                     `json:"autonomy"`
	MonthlyTokenLimit     *int64                     `json:"monthly_token_limit"`
	MonthlyCostLimitCents *int64                     `json:"monthly_cost_limit_cents"`
	Currency              string                     `json:"currency"`
	AllowedRoles          []string                   `json:"allowed_roles"`
	BYOAvailable          bool                       `json:"byo_available"`
	RunzivoAvailable      bool                       `json:"runzivo_available"`
	CanManage             bool                       `json:"can_manage"`
	UpdatedAt             *string                    `json:"updated_at"`
	AllowedNetworks       []string                   `json:"allowed_networks"`
}
type AIUsageAggregate struct {
	Key                string `json:"key,omitempty"`
	Requests           int64  `json:"requests"`
	InputTokens        int64  `json:"input_tokens"`
	OutputTokens       int64  `json:"output_tokens"`
	CachedTokens       int64  `json:"cached_tokens"`
	CostEstimateMicros *int64 `json:"cost_estimate_micros"`
	UnpricedRequests   int64  `json:"unpriced_requests"`
	Errors             int64  `json:"errors"`
	Limited            int64  `json:"limited"`
	P50LatencyMS       *int64 `json:"p50_latency_ms"`
	P95LatencyMS       *int64 `json:"p95_latency_ms"`
}
type AIUsageSummary struct {
	From    string             `json:"from"`
	To      string             `json:"to"`
	GroupBy *string            `json:"group_by"`
	Totals  *AIUsageAggregate  `json:"totals"`
	Series  []AIUsageAggregate `json:"series"`
}
type AICall struct {
	ID                 int64   `json:"id"`
	CreatedAt          string  `json:"created_at"`
	Actor              *string `json:"identity_id"`
	Agent              string  `json:"agent"`
	Mode               string  `json:"mode"`
	ProviderKind       string  `json:"provider_kind"`
	Model              string  `json:"model"`
	InputTokens        int64   `json:"input_tokens"`
	OutputTokens       int64   `json:"output_tokens"`
	CachedTokens       *int64  `json:"cached_tokens"`
	CostEstimateMicros *int64  `json:"cost_estimate_micros"`
	LatencyMS          int64   `json:"latency_ms"`
	Outcome            string  `json:"outcome"`
	ErrorCode          *string `json:"error_code"`
}
type AILimit struct {
	PeriodStart    string  `json:"period_start"`
	PeriodEnd      string  `json:"period_end"`
	UsedTokens     int64   `json:"used_tokens"`
	UsedCostMicros *int64  `json:"used_cost_micros"`
	TokenLimit     *int64  `json:"token_limit"`
	CostLimitCents *int64  `json:"cost_limit_cents"`
	Limited        bool    `json:"limited"`
	LimitKind      *string `json:"limit_kind"`
}
type aiCallsResponse struct {
	Calls       *[]AICall `json:"calls"`
	Next        *string   `json:"next_cursor"`
	nextPresent bool
}

func (v *aiCallsResponse) UnmarshalJSON(raw []byte) error {
	_, err := aiRequiredObject(raw, "calls", "next_cursor")
	if err != nil {
		return err
	}
	v.nextPresent = true
	type plain aiCallsResponse
	return json.Unmarshal(raw, (*plain)(v))
}
func (v *AIProvider) UnmarshalJSON(raw []byte) error {
	fields, err := aiRequiredObject(raw, "provider_id", "kind", "label", "base_url", "key_hint", "has_key", "status", "last_tested_at", "last_error", "created_by", "created_at", "updated_at")
	if err != nil {
		return err
	}
	for _, name := range []string{"provider_id", "kind", "label", "has_key", "status", "created_at", "updated_at"} {
		if err = aiNonNull(fields, name); err != nil {
			return err
		}
	}
	type plain AIProvider
	return json.Unmarshal(raw, (*plain)(v))
}
func (v *AITestResult) UnmarshalJSON(raw []byte) error {
	fields, err := aiRequiredObject(raw, "status", "latency_ms", "error_code")
	if err != nil {
		return err
	}
	for _, name := range []string{"status", "latency_ms"} {
		if err = aiNonNull(fields, name); err != nil {
			return err
		}
	}
	type plain AITestResult
	return json.Unmarshal(raw, (*plain)(v))
}
func (v *AISettings) UnmarshalJSON(raw []byte) error {
	fields, err := aiRequiredObject(raw, "mode", "agents", "autonomy", "monthly_token_limit", "monthly_cost_limit_cents", "currency", "allowed_roles", "byo_available", "runzivo_available", "can_manage", "updated_at", "allowed_networks")
	if err != nil {
		return err
	}
	for _, name := range []string{"mode", "agents", "autonomy", "currency", "allowed_roles", "byo_available", "runzivo_available", "can_manage", "allowed_networks"} {
		if err = aiNonNull(fields, name); err != nil {
			return err
		}
	}
	type plain AISettings
	return json.Unmarshal(raw, (*plain)(v))
}
func (v *AIUsageSummary) UnmarshalJSON(raw []byte) error {
	fields, err := aiRequiredObject(raw, "from", "to", "group_by", "totals", "series")
	if err != nil {
		return err
	}
	for _, name := range []string{"from", "to", "totals", "series"} {
		if err = aiNonNull(fields, name); err != nil {
			return err
		}
	}
	type plain AIUsageSummary
	return json.Unmarshal(raw, (*plain)(v))
}
func (v *AILimit) UnmarshalJSON(raw []byte) error {
	fields, err := aiRequiredObject(raw, "period_start", "period_end", "used_tokens", "used_cost_micros", "token_limit", "cost_limit_cents", "limited", "limit_kind")
	if err != nil {
		return err
	}
	for _, name := range []string{"period_start", "period_end", "used_tokens", "limited"} {
		if err = aiNonNull(fields, name); err != nil {
			return err
		}
	}
	type plain AILimit
	return json.Unmarshal(raw, (*plain)(v))
}
func (v *AIModel) UnmarshalJSON(raw []byte) error {
	fields, err := aiRequiredObject(raw, "id", "display_name", "created_at", "context_window", "recommended")
	if err != nil {
		return err
	}
	if err = aiNonNull(fields, "id"); err != nil {
		return err
	}
	if err = aiNonNull(fields, "display_name"); err != nil {
		return err
	}
	if err = aiNonNull(fields, "recommended"); err != nil {
		return err
	}
	type plain AIModel
	return json.Unmarshal(raw, (*plain)(v))
}
func (v *AIAgentSettings) UnmarshalJSON(raw []byte) error {
	fields, err := aiRequiredObject(raw, "enabled", "provider_id", "model")
	if err != nil {
		return err
	}
	if err = aiNonNull(fields, "enabled"); err != nil {
		return err
	}
	type plain AIAgentSettings
	return json.Unmarshal(raw, (*plain)(v))
}
func (v *AIUsageAggregate) UnmarshalJSON(raw []byte) error {
	fields, err := aiRequiredObject(raw, "requests", "input_tokens", "output_tokens", "cached_tokens", "cost_estimate_micros", "unpriced_requests", "errors", "limited", "p50_latency_ms", "p95_latency_ms")
	if err != nil {
		return err
	}
	for _, name := range []string{"requests", "input_tokens", "output_tokens", "cached_tokens", "unpriced_requests", "errors", "limited"} {
		if err = aiNonNull(fields, name); err != nil {
			return err
		}
	}
	type plain AIUsageAggregate
	return json.Unmarshal(raw, (*plain)(v))
}
func (v *AICall) UnmarshalJSON(raw []byte) error {
	fields, err := aiRequiredObject(raw, "id", "created_at", "agent", "mode", "provider_kind", "model", "identity_id", "input_tokens", "output_tokens", "cached_tokens", "cost_estimate_micros", "latency_ms", "outcome", "error_code")
	if err != nil {
		return err
	}
	for _, name := range []string{"id", "created_at", "agent", "mode", "provider_kind", "model", "input_tokens", "output_tokens", "latency_ms", "outcome"} {
		if err = aiNonNull(fields, name); err != nil {
			return err
		}
	}
	type plain AICall
	return json.Unmarshal(raw, (*plain)(v))
}

func aiPath(id, rest string) string { return tenantPath(id, "/ai"+rest) }
func (v AIProvider) valid() bool {
	return nonEmpty(v.ProviderID, v.Kind, v.Status, v.CreatedAt, v.UpdatedAt)
}
func (v AITestResult) valid() bool { return nonEmpty(v.Status) }
func (v AISettings) valid() bool {
	return nonEmpty(v.Mode, v.Autonomy, v.Currency) && v.Agents != nil && v.AllowedRoles != nil && v.AllowedNetworks != nil
}
func (v AIUsageSummary) valid() bool {
	return nonEmpty(v.From, v.To) && v.Totals != nil && v.Series != nil
}
func (v AILimit) valid() bool { return nonEmpty(v.PeriodStart, v.PeriodEnd) }
func (v AICall) valid() bool {
	return v.ID > 0 && nonEmpty(v.CreatedAt, v.Agent, v.Mode, v.ProviderKind, v.Model, v.Outcome)
}
func (c *Client) AIProviders(ctx context.Context, id string) ([]AIProvider, error) {
	var v struct {
		Providers *[]AIProvider `json:"providers"`
	}
	err := c.do(ctx, http.MethodGet, aiPath(id, "/providers"), nil, nil, &v)
	if err != nil {
		return nil, err
	}
	if err = required(v.Providers != nil); err != nil {
		return nil, err
	}
	for _, provider := range *v.Providers {
		if err = required(provider.valid()); err != nil {
			return nil, err
		}
	}
	return *v.Providers, nil
}
func (c *Client) CreateAIProvider(ctx context.Context, id string, in AIProviderInput) (*AIProvider, error) {
	var v AIProvider
	err := c.do(ctx, http.MethodPost, aiPath(id, "/providers"), nil, in, &v)
	if err == nil {
		err = required(v.valid())
	}
	return &v, err
}
func (c *Client) UpdateAIProvider(ctx context.Context, id, p string, in AIProviderInput) (*AIProvider, error) {
	var v AIProvider
	err := c.do(ctx, http.MethodPut, aiPath(id, "/providers/"+url.PathEscape(p)), nil, in, &v)
	if err == nil {
		err = required(v.valid())
	}
	return &v, err
}
func (c *Client) DeleteAIProvider(ctx context.Context, id, p string) error {
	return c.do(ctx, http.MethodDelete, aiPath(id, "/providers/"+url.PathEscape(p)), nil, nil, nil)
}
func (c *Client) TestAIProvider(ctx context.Context, id, p string) (*AITestResult, error) {
	var v AITestResult
	err := c.do(ctx, http.MethodPost, aiPath(id, "/providers/"+url.PathEscape(p)+"/test"), nil, nil, &v)
	if err == nil {
		err = required(v.valid())
	}
	return &v, err
}
func (c *Client) AIModels(ctx context.Context, id, p string, refresh bool) ([]AIModel, *string, error) {
	var v struct {
		Models    *[]AIModel `json:"models"`
		FetchedAt *string    `json:"fetched_at"`
	}
	q := url.Values{}
	if refresh {
		q.Set("refresh", "true")
	}
	err := c.do(ctx, http.MethodGet, aiPath(id, "/providers/"+url.PathEscape(p)+"/models"), q, nil, &v)
	if err != nil {
		return nil, nil, err
	}
	if err = required(v.Models != nil && v.FetchedAt != nil); err != nil {
		return nil, nil, err
	}
	for _, model := range *v.Models {
		if err = required(nonEmpty(model.ID, model.DisplayName)); err != nil {
			return nil, nil, err
		}
	}
	return *v.Models, v.FetchedAt, nil
}
func (c *Client) AISettings(ctx context.Context, id string) (*AISettings, error) {
	var v AISettings
	err := c.do(ctx, http.MethodGet, aiPath(id, "/settings"), nil, nil, &v)
	if err == nil {
		err = required(v.valid())
	}
	return &v, err
}
func (c *Client) UpdateAISettings(ctx context.Context, id string, in any) (*AISettings, error) {
	var v AISettings
	err := c.do(ctx, http.MethodPut, aiPath(id, "/settings"), nil, in, &v)
	if err == nil {
		err = required(v.valid())
	}
	return &v, err
}
func (c *Client) AIUsageSummary(ctx context.Context, id string, q url.Values) (*AIUsageSummary, error) {
	var v AIUsageSummary
	err := c.do(ctx, http.MethodGet, aiPath(id, "/usage/summary"), q, nil, &v)
	if err == nil {
		err = required(v.valid())
	}
	return &v, err
}
func (c *Client) AICalls(ctx context.Context, id string, q url.Values) ([]AICall, *string, error) {
	var v aiCallsResponse
	err := c.do(ctx, http.MethodGet, aiPath(id, "/usage/calls"), q, nil, &v)
	if err != nil {
		return nil, nil, err
	}
	if err = required(v.Calls != nil && v.nextPresent); err != nil {
		return nil, nil, err
	}
	for _, call := range *v.Calls {
		if err = required(call.valid()); err != nil {
			return nil, nil, err
		}
	}
	return *v.Calls, v.Next, nil
}
func (c *Client) AILimit(ctx context.Context, id string) (*AILimit, error) {
	var v AILimit
	err := c.do(ctx, http.MethodGet, aiPath(id, "/usage/limit"), nil, nil, &v)
	if err == nil {
		err = required(v.valid())
	}
	return &v, err
}
