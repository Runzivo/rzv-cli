package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type ClosureRequester struct {
	Email *string `json:"email"`
}
type ClosureRequest struct {
	ID            string           `json:"id"`
	Status        string           `json:"status"`
	Reason        string           `json:"reason"`
	RequestedBy   ClosureRequester `json:"requested_by"`
	RequestedAt   string           `json:"requested_at"`
	DecidedAt     *string          `json:"decided_at"`
	DecidedByType *string          `json:"decided_by_type"`
	DecisionNote  *string          `json:"decision_note"`
	Version       int64            `json:"version"`
}

func (r *ClosureRequest) UnmarshalJSON(data []byte) error {
	type plain ClosureRequest
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for _, key := range []string{"id", "status", "reason", "requested_by", "requested_at", "decided_at", "decided_by_type", "decision_note", "version"} {
		if !closurePresent(raw, key) {
			return fmt.Errorf("missing %s", key)
		}
	}
	for _, key := range []string{"id", "status", "reason", "requested_by", "requested_at", "version"} {
		if !closureNonNull(raw[key]) {
			return fmt.Errorf("null %s", key)
		}
	}
	var requester map[string]json.RawMessage
	if err := json.Unmarshal(raw["requested_by"], &requester); err != nil {
		return err
	}
	if !closurePresent(requester, "email") {
		return fmt.Errorf("missing email")
	}
	return json.Unmarshal(data, (*plain)(r))
}
func closurePresent(raw map[string]json.RawMessage, key string) bool { _, ok := raw[key]; return ok }
func closureNonNull(value json.RawMessage) bool                      { return len(value) > 0 && string(value) != "null" }

type ClosureRequestInput struct {
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"idempotency_key"`
}

func closurePath(tenantID string) string { return tenantPath(tenantID, "/closure-requests") }
func (c *Client) ClosureRequests(ctx context.Context, tenantID string) ([]ClosureRequest, error) {
	var out struct {
		Requests *[]ClosureRequest `json:"requests"`
	}
	if err := c.do(ctx, http.MethodGet, closurePath(tenantID), nil, nil, &out); err != nil {
		return nil, err
	}
	if err := required(out.Requests != nil); err != nil {
		return nil, err
	}
	for _, r := range *out.Requests {
		if err := required(validClosure(r)); err != nil {
			return nil, err
		}
	}
	return *out.Requests, nil
}
func (c *Client) RequestClosure(ctx context.Context, tenantID string, input ClosureRequestInput) (*ClosureRequest, error) {
	var out struct {
		Request *ClosureRequest `json:"request"`
	}
	if err := c.do(ctx, http.MethodPost, closurePath(tenantID), nil, input, &out); err != nil {
		return nil, err
	}
	if err := required(out.Request != nil && validClosure(*out.Request)); err != nil {
		return nil, err
	}
	return out.Request, nil
}
func (c *Client) CancelClosure(ctx context.Context, tenantID, id string, version int64) (*ClosureRequest, error) {
	var out struct {
		Request *ClosureRequest `json:"request"`
	}
	if err := c.do(ctx, http.MethodPost, closurePath(tenantID)+"/"+url.PathEscape(id)+"/cancel", nil, map[string]int64{"expected_version": version}, &out); err != nil {
		return nil, err
	}
	if err := required(out.Request != nil && validClosure(*out.Request)); err != nil {
		return nil, err
	}
	return out.Request, nil
}
func validClosure(r ClosureRequest) bool {
	return nonEmpty(r.ID, r.Status, r.Reason, r.RequestedAt) && r.Version >= 1 && (r.Status == "pending" || r.Status == "completed" || r.Status == "cancelled")
}
