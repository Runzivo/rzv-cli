package api

import (
	"context"
	"net/http"
	"net/url"
)

// APIKeyScopes are the scopes a workspace API key can hold. all:all must
// be the only scope and only an owner can create it.
var APIKeyScopes = []string{"all:all", "audit:read", "jobs:read", "logs:read", "usage:read", "runners:read", "integrations:read", "billing:read", "sandboxes:read", "sandboxes:write"}

// APIKey is a workspace API key's metadata. The secret is never returned
// after creation.
type APIKey struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Prefix      string   `json:"prefix"`
	Scopes      []string `json:"scopes"`
	Permissions []string `json:"permissions"`
	CreatedAt   string   `json:"created_at"`
	LastUsedAt  *string  `json:"last_used_at"`
	RevokedAt   *string  `json:"revoked_at"`
	ExpiresAt   *string  `json:"expires_at"`
}

// APIKeys is listCustomerAPIKeys' result.
type APIKeys struct {
	APIKeys       []APIKey `json:"api_keys"`
	AllowedScopes []string `json:"allowed_scopes"`
	Capabilities  struct {
		CanCreate bool `json:"can_create"`
		CanRename bool `json:"can_rename"`
		CanRevoke bool `json:"can_revoke"`
	} `json:"capabilities"`
}

func keyPath(tenantID, keyID string) string {
	return tenantPath(tenantID, "/api-keys/"+url.PathEscape(keyID))
}

// APIKeys calls listCustomerAPIKeys.
func (c *Client) APIKeys(ctx context.Context, tenantID string) (*APIKeys, error) {
	var body APIKeys
	if err := c.do(ctx, http.MethodGet, tenantPath(tenantID, "/api-keys"), nil, nil, &body); err != nil {
		return nil, err
	}
	if err := required(body.APIKeys != nil); err != nil {
		return nil, err
	}
	return &body, nil
}

// CreateAPIKeyInput is the body of createCustomerAPIKey. ExpiresAt is
// RFC 3339; empty means 365 days, the longest allowed.
type CreateAPIKeyInput struct {
	Name           string   `json:"name"`
	Scopes         []string `json:"scopes,omitempty"`
	Permissions    []string `json:"permissions,omitempty"`
	ExpiresAt      string   `json:"expires_at,omitempty"`
	IdempotencyKey string   `json:"idempotency_key"`
}

// CreatedAPIKey holds the one-time secret.
type CreatedAPIKey struct {
	APIKey APIKey `json:"api_key"`
	Secret string `json:"secret"`
}

// CreateAPIKey calls createCustomerAPIKey.
func (c *Client) CreateAPIKey(ctx context.Context, tenantID string, input CreateAPIKeyInput) (*CreatedAPIKey, error) {
	var body CreatedAPIKey
	if err := c.do(ctx, http.MethodPost, tenantPath(tenantID, "/api-keys"), nil, input, &body); err != nil {
		return nil, err
	}
	if err := required(nonEmpty(body.APIKey.ID, body.Secret)); err != nil {
		return nil, err
	}
	return &body, nil
}

// RenameAPIKey calls renameCustomerAPIKey.
func (c *Client) RenameAPIKey(ctx context.Context, tenantID, keyID, name string) (*APIKey, error) {
	var body struct {
		APIKey *APIKey `json:"api_key"`
	}
	if err := c.do(ctx, http.MethodPatch, keyPath(tenantID, keyID), nil, map[string]string{"name": name}, &body); err != nil {
		return nil, err
	}
	if err := required(body.APIKey != nil && nonEmpty(body.APIKey.ID)); err != nil {
		return nil, err
	}
	return body.APIKey, nil
}

// RevokeAPIKey calls revokeCustomerAPIKey.
func (c *Client) RevokeAPIKey(ctx context.Context, tenantID, keyID string) error {
	return c.do(ctx, http.MethodDelete, keyPath(tenantID, keyID), nil, nil, nil)
}
