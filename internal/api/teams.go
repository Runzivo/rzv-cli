package api

import (
	"context"
	"net/http"
	"net/url"
)

// PermissionDefinition is one permission in the catalogue.
type PermissionDefinition struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Group       string `json:"group"`
}

// PermissionPreset is the permission set of a built-in role.
type PermissionPreset struct {
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Permissions []string `json:"permissions"`
}

// Permissions is getCustomerPermissions' result: the catalogue, the role
// presets and the caller's own permissions in the workspace.
type Permissions struct {
	Catalogue   []PermissionDefinition `json:"catalogue"`
	Presets     []PermissionPreset     `json:"presets"`
	Permissions []string               `json:"permissions"`
}

// TeamMember is one person in a team.
type TeamMember struct {
	IdentityID string  `json:"identity_id"`
	Email      *string `json:"email"`
	AddedBy    *string `json:"added_by"`
	CreatedAt  string  `json:"created_at"`
}

// Team is one workspace team. SystemKind is set for the locked system teams.
type Team struct {
	TeamID      string       `json:"team_id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	SystemKind  *string      `json:"system_kind"`
	Locked      bool         `json:"locked"`
	Permissions []string     `json:"permissions"`
	MemberCount int          `json:"member_count"`
	Members     []TeamMember `json:"members,omitempty"`
	CreatedAt   string       `json:"created_at"`
	UpdatedAt   string       `json:"updated_at"`
}

func teamPath(tenantID, teamID, rest string) string {
	return tenantPath(tenantID, "/teams/"+url.PathEscape(teamID)+rest)
}

// Permissions calls getCustomerPermissions.
func (c *Client) Permissions(ctx context.Context, tenantID string) (*Permissions, error) {
	var body Permissions
	if err := c.do(ctx, http.MethodGet, tenantPath(tenantID, "/permissions"), nil, nil, &body); err != nil {
		return nil, err
	}
	if err := required(body.Catalogue != nil && body.Permissions != nil); err != nil {
		return nil, err
	}
	return &body, nil
}

// Teams calls listCustomerTeams.
func (c *Client) Teams(ctx context.Context, tenantID string) ([]Team, error) {
	var body struct {
		Teams *[]Team `json:"teams"`
	}
	if err := c.do(ctx, http.MethodGet, tenantPath(tenantID, "/teams"), nil, nil, &body); err != nil {
		return nil, err
	}
	if err := required(body.Teams != nil); err != nil {
		return nil, err
	}
	for _, team := range *body.Teams {
		if err := required(nonEmpty(team.TeamID)); err != nil {
			return nil, err
		}
	}
	return *body.Teams, nil
}

// teamResult sends one team request and checks the team in the response.
func (c *Client) teamResult(ctx context.Context, method, path string, input any) (*Team, error) {
	var body struct {
		Team *Team `json:"team"`
	}
	if err := c.do(ctx, method, path, nil, input, &body); err != nil {
		return nil, err
	}
	if err := required(body.Team != nil && nonEmpty(body.Team.TeamID)); err != nil {
		return nil, err
	}
	return body.Team, nil
}

// Team calls getCustomerTeam.
func (c *Client) Team(ctx context.Context, tenantID, teamID string) (*Team, error) {
	return c.teamResult(ctx, http.MethodGet, teamPath(tenantID, teamID, ""), nil)
}

// CreateTeamInput is the body of createCustomerTeam.
type CreateTeamInput struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

// CreateTeam calls createCustomerTeam.
func (c *Client) CreateTeam(ctx context.Context, tenantID string, input CreateTeamInput) (*Team, error) {
	return c.teamResult(ctx, http.MethodPost, tenantPath(tenantID, "/teams"), input)
}

// UpdateTeam calls updateCustomerTeam. A nil field is left unchanged.
func (c *Client) UpdateTeam(ctx context.Context, tenantID, teamID string, name, description *string) (*Team, error) {
	return c.teamResult(ctx, http.MethodPatch, teamPath(tenantID, teamID, ""), map[string]*string{"name": name, "description": description})
}

// DeleteTeam calls deleteCustomerTeam.
func (c *Client) DeleteTeam(ctx context.Context, tenantID, teamID string) error {
	return c.do(ctx, http.MethodDelete, teamPath(tenantID, teamID, ""), nil, nil, nil)
}

// SetTeamGrants calls setCustomerTeamGrants, replacing the team's permissions.
func (c *Client) SetTeamGrants(ctx context.Context, tenantID, teamID string, grants []string) (*Team, error) {
	return c.teamResult(ctx, http.MethodPut, teamPath(tenantID, teamID, "/grants"), map[string][]string{"permissions": grants})
}

// AddTeamMember calls addCustomerTeamMember.
func (c *Client) AddTeamMember(ctx context.Context, tenantID, teamID, identityID string) (*Team, error) {
	return c.teamResult(ctx, http.MethodPost, teamPath(tenantID, teamID, "/members"), map[string]string{"identity_id": identityID})
}

// RemoveTeamMember calls removeCustomerTeamMember.
func (c *Client) RemoveTeamMember(ctx context.Context, tenantID, teamID, identityID string) error {
	return c.do(ctx, http.MethodDelete, teamPath(tenantID, teamID, "/members/"+url.PathEscape(identityID)), nil, nil, nil)
}
