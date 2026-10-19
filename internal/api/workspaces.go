package api

import (
	"context"
	"net/http"
)

// Membership is one workspace the caller belongs to.
type Membership struct {
	TenantID           string `json:"tenant_id"`
	TenantSlug         string `json:"tenant_slug"`
	TenantName         string `json:"tenant_name"`
	Role               string `json:"role"`
	GitHubAccountID    string `json:"github_account_id,omitempty"`
	GitHubAccountLogin string `json:"github_account_login,omitempty"`
}

// Workspace is a newly created (or replayed) workspace.
type Workspace struct {
	TenantID   string `json:"tenant_id"`
	TenantSlug string `json:"tenant_slug"`
	TenantName string `json:"tenant_name"`
	Role       string `json:"role"`
}

// Memberships calls listCustomerMemberships. A person's token only.
func (c *Client) Memberships(ctx context.Context) ([]Membership, error) {
	var body struct {
		Memberships *[]Membership `json:"memberships"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/customer/memberships", nil, nil, &body); err != nil {
		return nil, err
	}
	if err := required(body.Memberships != nil); err != nil {
		return nil, err
	}
	for _, m := range *body.Memberships {
		if err := required(nonEmpty(m.TenantID)); err != nil {
			return nil, err
		}
	}
	return *body.Memberships, nil
}

// CreateWorkspaceInput is the body of createCustomerWorkspace.
type CreateWorkspaceInput struct {
	Slug           string `json:"slug"`
	Name           string `json:"name"`
	IdempotencyKey string `json:"idempotency_key"`
}

// CreateWorkspace calls createCustomerWorkspace. Retrying with the same
// idempotency key, slug and name returns the original workspace.
func (c *Client) CreateWorkspace(ctx context.Context, input CreateWorkspaceInput) (*Workspace, error) {
	var body struct {
		Workspace *Workspace `json:"workspace"`
	}
	if err := c.do(ctx, http.MethodPost, "/v1/customer/workspaces", nil, input, &body); err != nil {
		return nil, err
	}
	if err := required(body.Workspace != nil && nonEmpty(body.Workspace.TenantID)); err != nil {
		return nil, err
	}
	return body.Workspace, nil
}
