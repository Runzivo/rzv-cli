package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
)

// Repository is a repository a GitHub App installation covers.
// RepositoryID is the immutable key; Name is nil until Runzivo has it.
type Repository struct {
	RepositoryID string  `json:"repository_id"`
	Name         *string `json:"name"`
	Authorized   bool    `json:"authorized"`
	Archived     bool    `json:"archived,omitempty"`
	ArchivedAt   *string `json:"archived_at,omitempty"`
}

// Installation is a GitHub App installation linked to a workspace.
// InstallationID is set only by the plural read.
type Installation struct {
	InstallationID          string       `json:"installation_id,omitempty"`
	State                   string       `json:"state"`
	AccountName             *string      `json:"account_name"`
	ManageURL               *string      `json:"manage_url"`
	RunnerPermissionMissing bool         `json:"runner_permission_missing,omitempty"`
	SetupState              string       `json:"setup_state,omitempty"`
	NextRetryAt             *string      `json:"next_retry_at,omitempty"`
	AccountArchived         bool         `json:"account_archived,omitempty"`
	AccountArchivedAt       *string      `json:"account_archived_at,omitempty"`
	Repositories            []Repository `json:"repositories"`
}

// CompleteWorkspaceConnectionInput is the body of
// completeCustomerGitHubWorkspaceConnection. UserAccessToken is sent once to
// prove installation authority and must never be stored or printed.
type CompleteWorkspaceConnectionInput struct {
	State           string `json:"state"`
	InstallationID  int64  `json:"installation_id"`
	UserAccessToken string `json:"user_access_token"`
}

// WorkspaceConnection is the result of creating, finding, or requesting
// access to a GitHub-backed workspace.
type WorkspaceConnection struct {
	Outcome      string     `json:"outcome"`
	Workspace    *Workspace `json:"workspace,omitempty"`
	AccountLogin string     `json:"account_login"`
	AccountType  string     `json:"account_type"`
	Notice       string     `json:"notice,omitempty"`
}

// StartWorkspaceConnection starts a state-bound GitHub-backed workspace flow.
func (c *Client) StartWorkspaceConnection(ctx context.Context) (*LinkStart, error) {
	var body LinkStart
	if err := c.do(ctx, http.MethodPost, "/v1/customer/github/workspaces/start", nil, nil, &body); err != nil {
		return nil, err
	}
	if err := required(nonEmpty(body.State, body.ExpiresAt)); err != nil {
		return nil, err
	}
	return &body, nil
}

// CompleteWorkspaceConnection verifies an installation and returns the
// server's create, existing, access-required, or pending decision.
func (c *Client) CompleteWorkspaceConnection(ctx context.Context, input CompleteWorkspaceConnectionInput) (*WorkspaceConnection, error) {
	var body WorkspaceConnection
	if err := c.do(ctx, http.MethodPost, "/v1/customer/github/workspaces/complete", nil, input, &body); err != nil {
		return nil, err
	}
	if err := required(nonEmpty(body.Outcome, body.AccountLogin, body.AccountType)); err != nil {
		return nil, err
	}
	switch body.Outcome {
	case "created", "existing", "installation_pending":
		if err := required(body.Workspace != nil && nonEmpty(body.Workspace.TenantID, body.Workspace.TenantSlug, body.Workspace.TenantName, body.Workspace.Role)); err != nil {
			return nil, err
		}
	case "access_required":
		if err := required(body.Workspace == nil); err != nil {
			return nil, err
		}
	default:
		return nil, required(false)
	}
	return &body, nil
}

func (i *Installation) valid() bool { return i.State != "" && i.Repositories != nil }

func (i *Installation) validListEntry() bool {
	if i.InstallationID == "" || !i.valid() {
		return false
	}
	for _, repository := range i.Repositories {
		if repository.RepositoryID == "" {
			return false
		}
	}
	return true
}

// Installations calls listCustomerGitHubInstallations: every linked
// installation, oldest first, empty when none is linked.
func (c *Client) Installations(ctx context.Context, tenantID string) ([]Installation, error) {
	var body struct {
		Installations json.RawMessage `json:"installations"`
	}
	if err := c.do(ctx, http.MethodGet, tenantPath(tenantID, "/github-installations"), nil, nil, &body); err != nil {
		return nil, err
	}
	if err := required(len(body.Installations) > 0 && string(body.Installations) != "null"); err != nil {
		return nil, err
	}
	var rawInstallations []json.RawMessage
	if err := json.Unmarshal(body.Installations, &rawInstallations); err != nil {
		return nil, &Error{Status: http.StatusOK, Code: "malformed_response"}
	}
	installations := make([]Installation, 0, len(rawInstallations))
	for _, raw := range rawInstallations {
		var installation Installation
		if err := json.Unmarshal(raw, &installation); err != nil || !validInstallationListJSON(raw) || !installation.validListEntry() {
			return nil, &Error{Status: http.StatusOK, Code: "malformed_response"}
		}
		installations = append(installations, installation)
	}
	return installations, nil
}

func validInstallationListJSON(raw json.RawMessage) bool {
	var installation map[string]json.RawMessage
	if json.Unmarshal(raw, &installation) != nil || installation == nil {
		return false
	}
	if !hasNullableString(installation, "account_name") || !hasNullableString(installation, "manage_url") {
		return false
	}
	repositories, ok := installation["repositories"]
	if !ok || string(repositories) == "null" {
		return false
	}
	var entries []json.RawMessage
	if json.Unmarshal(repositories, &entries) != nil {
		return false
	}
	for _, rawEntry := range entries {
		var repository map[string]json.RawMessage
		if json.Unmarshal(rawEntry, &repository) != nil || repository == nil || !hasNullableString(repository, "name") || !hasBoolean(repository, "authorized") {
			return false
		}
	}
	return true
}

func hasNullableString(fields map[string]json.RawMessage, name string) bool {
	raw, ok := fields[name]
	if !ok || string(raw) == "null" {
		return ok
	}
	var value string
	return json.Unmarshal(raw, &value) == nil
}

func hasBoolean(fields map[string]json.RawMessage, name string) bool {
	raw, ok := fields[name]
	if !ok || string(raw) == "null" {
		return false
	}
	var value bool
	return json.Unmarshal(raw, &value) == nil
}

// Installation calls getCustomerGitHubInstallation. It returns nil, nil
// when no installation is linked (404 installation_not_linked).
func (c *Client) Installation(ctx context.Context, tenantID string) (*Installation, error) {
	var body struct {
		Installation *Installation `json:"github_installation"`
	}
	err := c.do(ctx, http.MethodGet, tenantPath(tenantID, "/github-installation"), nil, nil, &body)
	var apiErr *Error
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound && apiErr.Code == "installation_not_linked" {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := required(body.Installation != nil && body.Installation.valid()); err != nil {
		return nil, err
	}
	return body.Installation, nil
}

// LinkStart is a short-lived state for linking a GitHub App installation.
type LinkStart struct {
	State     string `json:"state"`
	ExpiresAt string `json:"expires_at"`
}

// StartInstallationLink calls startCustomerGitHubInstallationLink.
func (c *Client) StartInstallationLink(ctx context.Context, tenantID string) (*LinkStart, error) {
	var body LinkStart
	if err := c.do(ctx, http.MethodPost, tenantPath(tenantID, "/github-installation/start"), nil, nil, &body); err != nil {
		return nil, err
	}
	if err := required(nonEmpty(body.State, body.ExpiresAt)); err != nil {
		return nil, err
	}
	return &body, nil
}

// CompleteLinkInput is the body of completeCustomerGitHubInstallationLink.
// UserAccessToken is the person's GitHub user token and must never be
// printed.
type CompleteLinkInput struct {
	State           string `json:"state"`
	InstallationID  int64  `json:"installation_id"`
	UserAccessToken string `json:"user_access_token"`
}

// Link outcomes besides success.
const (
	LinkAuthorityDenied     = "authority_denied"
	LinkStateInvalid        = "link_state_invalid"
	LinkAuthorityUnverified = "authority_unverified"
)

// CompleteInstallationLink calls completeCustomerGitHubInstallationLink. On
// a documented refusal it returns a nil installation and the outcome.
func (c *Client) CompleteInstallationLink(ctx context.Context, tenantID string, input CompleteLinkInput) (*Installation, string, error) {
	var body struct {
		Installation *Installation `json:"github_installation"`
	}
	err := c.do(ctx, http.MethodPost, tenantPath(tenantID, "/github-installation/complete"), nil, input, &body)
	var apiErr *Error
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.Status == 403 && apiErr.Code == "installation_authority_denied":
			return nil, LinkAuthorityDenied, nil
		case apiErr.Status == 409 && apiErr.Code == "link_state_invalid":
			return nil, LinkStateInvalid, nil
		case apiErr.Status == 503 && apiErr.Code == "installation_authority_unverified":
			return nil, LinkAuthorityUnverified, nil
		}
	}
	if err != nil {
		return nil, "", err
	}
	if err := required(body.Installation != nil && body.Installation.valid()); err != nil {
		return nil, "", err
	}
	return body.Installation, "linked", nil
}

// RunnerProfile is one published runner size and whether this repository
// can use it now. Label is the exact runs-on value.
type RunnerProfile struct {
	Label      string  `json:"label"`
	VCPUCount  int64   `json:"vcpu_count"`
	MemSizeMiB int64   `json:"mem_size_mib"`
	Available  bool    `json:"available"`
	Reason     *string `json:"reason"`
}

// RunnerProfiles is getCustomerRunnerProfiles' result.
type RunnerProfiles struct {
	Profiles            []RunnerProfile `json:"profiles"`
	WorkflowLabelFormat string          `json:"workflow_label_format"`
}

// RunnerProfiles calls getCustomerRunnerProfiles. It returns nil, nil when
// the repository is not authorized for the workspace (404
// repository_not_authorized).
func (c *Client) RunnerProfiles(ctx context.Context, tenantID string, repositoryID int64) (*RunnerProfiles, error) {
	var body struct {
		RunnerProfiles *RunnerProfiles `json:"runner_profiles"`
	}
	query := map[string][]string{"repository_id": {strconv.FormatInt(repositoryID, 10)}}
	err := c.do(ctx, http.MethodGet, tenantPath(tenantID, "/runner-profiles"), query, nil, &body)
	var apiErr *Error
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound && apiErr.Code == "repository_not_authorized" {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := required(body.RunnerProfiles != nil && body.RunnerProfiles.Profiles != nil); err != nil {
		return nil, err
	}
	return body.RunnerProfiles, nil
}
