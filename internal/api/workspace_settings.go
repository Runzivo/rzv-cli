package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// WorkspaceLogo is the encoded logo returned by, or sent to, workspace settings.
type WorkspaceLogo struct {
	ContentType string `json:"content_type"`
	Base64      string `json:"base64"`
}

// Docker layer cache size limits, in MiB, from the customer API contract.
const (
	DockerCacheMinSizeMiB = 64
	DockerCacheMaxSizeMiB = 16384
)

// WorkspaceDockerCache is the workspace's Docker layer cache preference.
// Available reports whether hosts in this environment can serve the cache; even
// then, assignments, quotas and host health can still force a cold run.
type WorkspaceDockerCache struct {
	Enabled   bool  `json:"enabled"`
	SizeMiB   int64 `json:"size_mib"`
	Available bool  `json:"available"`
}

func (d *WorkspaceDockerCache) UnmarshalJSON(data []byte) error {
	type plain WorkspaceDockerCache
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for _, key := range []string{"enabled", "size_mib", "available"} {
		if !present(raw, key) || !nonNull(raw[key]) {
			return fmt.Errorf("missing docker_cache %s", key)
		}
	}
	return json.Unmarshal(data, (*plain)(d))
}

// WorkspaceSettings is the unwrapped workspace settings response.
// DockerCache is nil when an older service omits docker_cache.
type WorkspaceSettings struct {
	TenantID    string                `json:"tenant_id"`
	Name        string                `json:"name"`
	Logo        *WorkspaceLogo        `json:"logo"`
	CanUpdate   bool                  `json:"can_update"`
	DockerCache *WorkspaceDockerCache `json:"docker_cache,omitempty"`
}

func (s *WorkspaceSettings) UnmarshalJSON(data []byte) error {
	type plain WorkspaceSettings
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if present(raw, "docker_cache") && !nonNull(raw["docker_cache"]) {
		return fmt.Errorf("null docker_cache")
	}
	return json.Unmarshal(data, (*plain)(s))
}

// WorkspaceDockerCachePatch changes only the cache fields that are set.
type WorkspaceDockerCachePatch struct {
	Enabled *bool  `json:"enabled,omitempty"`
	SizeMiB *int64 `json:"size_mib,omitempty"`
}

// WorkspaceSettingsPatch contains only fields the caller intends to change.
type WorkspaceSettingsPatch struct {
	Name        *string                    `json:"name,omitempty"`
	Logo        *WorkspaceLogo             `json:"logo,omitempty"`
	RemoveLogo  bool                       `json:"remove_logo,omitempty"`
	DockerCache *WorkspaceDockerCachePatch `json:"docker_cache,omitempty"`
}

// WorkspaceMember is one workspace member and the caller's capabilities for it.
type WorkspaceMember struct {
	IdentityID    string   `json:"identity_id"`
	Role          string   `json:"role"`
	Email         *string  `json:"email"`
	CanChangeRole bool     `json:"can_change_role"`
	CanRemove     bool     `json:"can_remove"`
	AllowedRoles  []string `json:"allowed_roles"`
}

// WorkspaceMembers is the unwrapped members response.
type WorkspaceMembers struct {
	CurrentIdentityID    string            `json:"current_identity_id"`
	CanManageInvitations bool              `json:"can_manage_invitations"`
	CanInvite            bool              `json:"can_invite"`
	Members              []WorkspaceMember `json:"members"`
}

// WorkspaceInvitationTeam is a custom team applied when an invitation is accepted.
type WorkspaceInvitationTeam struct {
	TeamID string `json:"team_id"`
	Name   string `json:"name"`
}

// WorkspaceInvitation is a workspace invitation and the caller's capabilities for it.
type WorkspaceInvitation struct {
	InvitationID string                    `json:"invitation_id"`
	Email        string                    `json:"email"`
	Role         string                    `json:"role"`
	Status       string                    `json:"status"`
	ExpiresAt    string                    `json:"expires_at"`
	CreatedAt    string                    `json:"created_at"`
	CanResend    bool                      `json:"can_resend"`
	CanRevoke    bool                      `json:"can_revoke"`
	Teams        []WorkspaceInvitationTeam `json:"teams"`
}

// CreateWorkspaceInvitationInput is the body for creating an invitation.
type CreateWorkspaceInvitationInput struct {
	Email   string   `json:"email"`
	Role    string   `json:"role"`
	TeamIDs []string `json:"team_ids,omitempty"`
}

func (c *Client) WorkspaceSettings(ctx context.Context, tenantID string) (*WorkspaceSettings, error) {
	var value WorkspaceSettings
	if err := c.do(ctx, http.MethodGet, tenantPath(tenantID, "/settings"), nil, nil, &value); err != nil {
		return nil, err
	}
	if err := required(validWorkspaceSettings(value)); err != nil {
		return nil, err
	}
	return &value, nil
}

func (c *Client) UpdateWorkspaceSettings(ctx context.Context, tenantID string, patch WorkspaceSettingsPatch) (*WorkspaceSettings, error) {
	var value WorkspaceSettings
	if err := c.do(ctx, http.MethodPatch, tenantPath(tenantID, "/settings"), nil, patch, &value); err != nil {
		return nil, err
	}
	if err := required(validWorkspaceSettings(value)); err != nil {
		return nil, err
	}
	return &value, nil
}

// validWorkspaceSettings checks required fields and, when the service sends
// it, the Docker cache size range.
func validWorkspaceSettings(value WorkspaceSettings) bool {
	if !nonEmpty(value.TenantID) {
		return false
	}
	cache := value.DockerCache
	return cache == nil || (cache.SizeMiB >= DockerCacheMinSizeMiB && cache.SizeMiB <= DockerCacheMaxSizeMiB)
}

func (c *Client) WorkspaceMembers(ctx context.Context, tenantID string) (*WorkspaceMembers, error) {
	var value WorkspaceMembers
	if err := c.do(ctx, http.MethodGet, tenantPath(tenantID, "/members"), nil, nil, &value); err != nil {
		return nil, err
	}
	if err := required(nonEmpty(value.CurrentIdentityID) && value.Members != nil); err != nil {
		return nil, err
	}
	for _, member := range value.Members {
		if err := required(nonEmpty(member.IdentityID)); err != nil {
			return nil, err
		}
	}
	return &value, nil
}

func (c *Client) UpdateWorkspaceMemberRole(ctx context.Context, tenantID, identityID, role string) (*WorkspaceMember, error) {
	var value WorkspaceMember
	path := tenantPath(tenantID, "/members/"+url.PathEscape(identityID))
	if err := c.do(ctx, http.MethodPatch, path, nil, struct {
		Role string `json:"role"`
	}{Role: role}, &value); err != nil {
		return nil, err
	}
	if err := required(nonEmpty(value.IdentityID)); err != nil {
		return nil, err
	}
	return &value, nil
}

func (c *Client) RemoveWorkspaceMember(ctx context.Context, tenantID, identityID string) error {
	return c.do(ctx, http.MethodDelete, tenantPath(tenantID, "/members/"+url.PathEscape(identityID)), nil, nil, nil)
}

func (c *Client) WorkspaceInvitations(ctx context.Context, tenantID string) ([]WorkspaceInvitation, error) {
	var value struct {
		Invitations *[]WorkspaceInvitation `json:"invitations"`
	}
	if err := c.do(ctx, http.MethodGet, tenantPath(tenantID, "/invitations"), nil, nil, &value); err != nil {
		return nil, err
	}
	if err := required(value.Invitations != nil); err != nil {
		return nil, err
	}
	for _, invitation := range *value.Invitations {
		if err := required(nonEmpty(invitation.InvitationID)); err != nil {
			return nil, err
		}
	}
	return *value.Invitations, nil
}

func (c *Client) CreateWorkspaceInvitation(ctx context.Context, tenantID string, input CreateWorkspaceInvitationInput) (*WorkspaceInvitation, error) {
	var value WorkspaceInvitation
	if err := c.do(ctx, http.MethodPost, tenantPath(tenantID, "/invitations"), nil, input, &value); err != nil {
		return nil, err
	}
	if err := required(nonEmpty(value.InvitationID)); err != nil {
		return nil, err
	}
	return &value, nil
}

func (c *Client) ResendWorkspaceInvitation(ctx context.Context, tenantID, invitationID string) (*WorkspaceInvitation, error) {
	var value WorkspaceInvitation
	path := tenantPath(tenantID, "/invitations/"+url.PathEscape(invitationID)+"/resend")
	if err := c.do(ctx, http.MethodPost, path, nil, nil, &value); err != nil {
		return nil, err
	}
	if err := required(nonEmpty(value.InvitationID)); err != nil {
		return nil, err
	}
	return &value, nil
}

func (c *Client) UpdateWorkspaceInvitationTeams(ctx context.Context, tenantID, invitationID string, teamIDs []string) (*WorkspaceInvitation, error) {
	var value WorkspaceInvitation
	path := tenantPath(tenantID, "/invitations/"+url.PathEscape(invitationID))
	body := struct {
		TeamIDs []string `json:"team_ids"`
	}{TeamIDs: teamIDs}
	if err := c.do(ctx, http.MethodPatch, path, nil, body, &value); err != nil {
		return nil, err
	}
	if err := required(nonEmpty(value.InvitationID)); err != nil {
		return nil, err
	}
	return &value, nil
}

func (c *Client) RevokeWorkspaceInvitation(ctx context.Context, tenantID, invitationID string) error {
	return c.do(ctx, http.MethodDelete, tenantPath(tenantID, "/invitations/"+url.PathEscape(invitationID)), nil, nil, nil)
}

// AcceptWorkspaceInvitation accepts an invitation for the authenticated person.
func (c *Client) AcceptWorkspaceInvitation(ctx context.Context, token string) (*Membership, error) {
	var value Membership
	body := struct {
		Token string `json:"token"`
	}{Token: token}
	if err := c.do(ctx, http.MethodPost, "/v1/customer/invitations/accept", nil, body, &value); err != nil {
		return nil, err
	}
	if err := required(nonEmpty(value.TenantID)); err != nil {
		return nil, err
	}
	return &value, nil
}
