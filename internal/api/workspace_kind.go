package api

import (
	"context"
	"net/http"
)

// WorkspaceKindResult is confirmCustomerWorkspaceKind's answer: the kind that
// is set, or with a dry run the kind that would be set. ChangeID is present
// only when the change was written.
type WorkspaceKindResult struct {
	Kind       string `json:"kind"`
	KindSource string `json:"kind_source"`
	ChangeID   string `json:"change_id,omitempty"`
}

// ConfirmWorkspaceKind calls confirmCustomerWorkspaceKind (owner only, needs a
// recent sign-in, no API keys). Without confirm the service answers what it
// would set and writes nothing; the field is left out of the body then, as
// false is its default.
func (c *Client) ConfirmWorkspaceKind(ctx context.Context, tenantID, kind string, confirm bool) (*WorkspaceKindResult, error) {
	request := struct {
		Kind    string `json:"kind"`
		Confirm bool   `json:"confirm,omitempty"`
	}{Kind: kind, Confirm: confirm}
	var result WorkspaceKindResult
	if err := c.do(ctx, http.MethodPost, tenantPath(tenantID, "/kind"), nil, request, &result); err != nil {
		return nil, err
	}
	if err := required(result.Kind == kind && nonEmpty(result.KindSource) && (!confirm || nonEmpty(result.ChangeID))); err != nil {
		return nil, err
	}
	return &result, nil
}
