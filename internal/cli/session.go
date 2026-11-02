package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Runzivo/rzv-cli/internal/api"
	"github.com/Runzivo/rzv-cli/internal/config"
	"github.com/Runzivo/rzv-cli/internal/oidc"
)

// apiKey returns RZV_API_KEY when set. A workspace API key replaces the
// stored sign-in for workspace-scoped commands.
func (a *app) apiKey() string {
	return strings.TrimSpace(a.Getenv("RZV_API_KEY"))
}

// requireHuman refuses commands that need a person's sign-in while an API
// key is set, so a key is never silently ignored.
func (a *app) requireHuman() error {
	if a.apiKey() != "" {
		return accessError("RZV_API_KEY cannot be used for this command. Unset it to use your own sign-in.")
	}
	return nil
}

// credentials returns the stored sign-in, refusing an expired token before
// any network call.
func (a *app) credentials() (*config.Credentials, error) {
	if a.Store == nil {
		return nil, authError("Sign in with `rzv login` first.")
	}
	creds, err := a.Store.Read()
	if err != nil {
		return nil, err
	}
	if creds == nil {
		return nil, authError("Sign in with `rzv login` first.")
	}
	if claims, err := oidc.DecodeUnverified(creds.AccessToken); err == nil {
		if exp := claims.Time("exp"); !exp.IsZero() && !a.Now().Before(exp) {
			return nil, authError("Your sign-in has expired. Run `rzv login` again.")
		}
	}
	return creds, nil
}

func (a *app) client(baseURL, credential string) (*api.Client, error) {
	client, err := api.New(baseURL, credential, userAgent(), a.HTTPClient)
	if err != nil {
		return nil, newError("invalid_configuration", exitUsage, "%s", err.Error())
	}
	return client, nil
}

// humanClient is a client authenticated with the stored sign-in.
func (a *app) humanClient() (*api.Client, *config.Credentials, error) {
	if err := a.requireHuman(); err != nil {
		return nil, nil, err
	}
	creds, err := a.credentials()
	if err != nil {
		return nil, nil, err
	}
	client, err := a.client(creds.APIBaseURL, creds.AccessToken)
	return client, creds, err
}

// workspaceOption reads --workspace, or its older spelling --account.
func workspaceOption(cmd *command) (string, error) {
	ws, err := cmd.option("workspace")
	if err != nil {
		return "", err
	}
	acct, err := cmd.option("account")
	if err != nil {
		return "", err
	}
	if ws != "" && acct != "" {
		return "", usageError("Use --workspace or --account, not both.")
	}
	if ws != "" {
		return ws, nil
	}
	return acct, nil
}

// workspaceClient resolves the workspace for a workspace-scoped command and
// returns a client for it. With a person's sign-in, membership is checked
// before the call; the API authorizes the workspace again regardless. With
// RZV_API_KEY, the key is bound to one workspace by the API.
func (a *app) workspaceClient(cmd *command) (string, *api.Client, error) {
	explicit, err := workspaceOption(cmd)
	if err != nil {
		return "", nil, err
	}
	if key := a.apiKey(); key != "" {
		baseURL, selected, err := a.apiKeyTarget()
		if err != nil {
			return "", nil, err
		}
		id := explicit
		if id == "" {
			id = a.defaultWorkspace()
		}
		if id == "" {
			id = selected
		}
		if id == "" {
			return "", nil, usageError("Pass --workspace <workspace-id> with RZV_API_KEY, or set RZV_WORKSPACE to the id.")
		}
		client, err := a.client(baseURL, key)
		return id, client, err
	}
	client, creds, err := a.humanClient()
	if err != nil {
		return "", nil, err
	}
	id := explicit
	if id == "" {
		id = a.defaultWorkspace()
	}
	if id == "" {
		id = creds.SelectedWorkspace
	}
	if id == "" {
		return "", nil, usageError("Select a workspace with `rzv workspaces select <workspace>`, pass --workspace, or set RZV_WORKSPACE.")
	}
	memberships, err := client.Memberships(a.ctx)
	if err != nil {
		return "", nil, apiError(err, "")
	}
	tenantID, err := a.matchWorkspace(memberships, id)
	if err != nil {
		return "", nil, err
	}
	if tenantID == "" {
		return "", nil, accessError("You are not a member of workspace %s.", display(id))
	}
	return tenantID, client, nil
}

// defaultWorkspace is RZV_WORKSPACE: the workspace (id, slug or name) used when a
// command names none.
func (a *app) defaultWorkspace() string {
	return strings.TrimSpace(a.Getenv("RZV_WORKSPACE"))
}

// matchWorkspace resolves an exact id before a case-insensitive slug or name.
// Ambiguity at either tier is an error; never select the first match.
func (a *app) matchWorkspace(memberships []api.Membership, ref string) (string, error) {
	for _, m := range memberships {
		if m.TenantID == ref {
			return m.TenantID, nil
		}
	}
	for _, field := range []func(api.Membership) string{
		func(m api.Membership) string { return m.TenantSlug },
		func(m api.Membership) string { return m.TenantName },
	} {
		var matches []api.Membership
		for _, m := range memberships {
			if ref != "" && strings.EqualFold(field(m), ref) {
				matches = append(matches, m)
			}
		}
		if len(matches) == 1 {
			return matches[0].TenantID, nil
		}
		if len(matches) > 1 {
			lines := []string{fmt.Sprintf(a.text("More than one workspace is named %s. Choose an id:", "Plusieurs espaces de travail portent le nom %s. Choisissez un identifiant :"), display(ref))}
			for _, m := range matches {
				lines = append(lines, "- "+workspaceLabel(memberships, m.TenantID))
			}
			return "", usageError("%s", strings.Join(lines, "\n"))
		}
	}
	return "", nil
}

func workspaceLabel(memberships []api.Membership, id string) string {
	for _, m := range memberships {
		if m.TenantID == id && m.TenantName != "" {
			return display(m.TenantName) + " [" + display(id) + "]"
		}
	}
	return display(id)
}

// apiKeyTarget picks the API origin for RZV_API_KEY: RZV_API_URL, else the
// stored sign-in's API, else the RZV_ENV preset (default prd).
func (a *app) apiKeyTarget() (string, string, error) {
	selected := ""
	var creds *config.Credentials
	if a.Store != nil {
		creds, _ = a.Store.Read()
	}
	if creds != nil {
		selected = creds.SelectedWorkspace
	}
	if url := strings.TrimSpace(a.Getenv("RZV_API_URL")); url != "" {
		return url, selected, nil
	}
	if creds != nil {
		return creds.APIBaseURL, selected, nil
	}
	env, err := config.LookupEnvironment(a.Getenv("RZV_ENV"))
	if err != nil {
		return "", "", usageError("%s", err.Error())
	}
	return env.APIBaseURL, selected, nil
}

// apiError maps a customer API failure to a CLI error. notFound, when not
// empty, is the message for a 404 not_found.
func apiError(err error, notFound string) error {
	var apiErr *api.Error
	if !errors.As(err, &apiErr) {
		return err
	}
	switch {
	case apiErr.Status == 0:
		return newError("customer_api_unavailable", exitUnavailable, "The customer API is unavailable.")
	case apiErr.Status == 401:
		return authError("Credentials were rejected or expired. Run `rzv login` again.")
	case apiErr.Status == 403 && apiErr.Code == "payer_verification_required":
		return newError(apiErr.Code, exitAccessDenied, "The signed-in email is not eligible to manage billing for this purchase.")
	case apiErr.Status == 403 && apiErr.Code == "api_key_owner_required":
		return newError(apiErr.Code, exitAccessDenied, "Only a workspace owner can create an all:all API key.")
	case apiErr.Status == 403 && apiErr.Code == "api_key_scope_required":
		return newError(apiErr.Code, exitAccessDenied, "This API key's scope does not allow this command.")
	case apiErr.Status == 403 && apiErr.Code == "grant_ceiling":
		return newError(apiErr.Code, exitAccessDenied, "You can only grant permissions you hold yourself.")
	case apiErr.Status == 403 && apiErr.Code == "owner_required":
		return newError(apiErr.Code, exitAccessDenied, "Only a workspace owner can change the Owners team.")
	case apiErr.Status == 403 && apiErr.Code == "code_not_eligible":
		return newError(apiErr.Code, exitAccessDenied, "This code is not eligible for this account.")
	case apiErr.Status == 403 && apiErr.Code == "insufficient_role":
		return newError(apiErr.Code, exitAccessDenied, "Your workspace role does not allow this command.")
	case apiErr.Status == 409 && apiErr.Code == "billing_country_in_use":
		return newError(apiErr.Code, exitFailure, "This workspace already has a credit purchase, so its billing country cannot be cleared. You can still change it with `rzv billing country set <code>`.")
	case apiErr.Status == 409 && apiErr.Code == "ticket_already_closed":
		return newError(apiErr.Code, exitFailure, "That ticket is already closed.")
	case apiErr.Status == 403 && apiErr.Code == "auto_recharge_enable_requires_person":
		return newError(apiErr.Code, exitAccessDenied, "Turning auto-recharge on needs a person signed in, not an API key.")
	case apiErr.Status == 403 && apiErr.Code == "reauth_required":
		return newError(apiErr.Code, exitAuth, "Sign in again with `rzv login`, then retry.")
	case apiErr.Status == 404 && apiErr.Code == "disk_not_found":
		return notFoundError("That persistent disk was not found in this workspace. List them with `rzv disks list`.")
	case apiErr.Status == 403 && apiErr.Code == "persistent_disks_not_entitled":
		return newError(apiErr.Code, exitAccessDenied, "This workspace has no persistent disk quota. Ask Runzivo support for one.")
	case apiErr.Status == 503 && apiErr.Code == "persistent_disks_unavailable":
		return newError(apiErr.Code, exitUnavailable, "Persistent disks are not available on this Runzivo environment yet.")
	case apiErr.Status == 409 && apiErr.Code == "disk_quota_exceeded":
		return newError(apiErr.Code, exitFailure, "That size would go over this workspace's disk quota. `rzv disks list` shows the quota.")
	case apiErr.Status == 409 && apiErr.Code == "disk_limit_reached":
		return newError(apiErr.Code, exitFailure, "This workspace already has its maximum number of disks. Delete one first.")
	case apiErr.Status == 409 && apiErr.Code == "disk_name_taken":
		return newError(apiErr.Code, exitFailure, "A disk with that name already exists in this workspace.")
	case apiErr.Status == 409 && apiErr.Code == "disk_attached":
		return newError(apiErr.Code, exitFailure, "That disk is attached to a sandbox. Stop the sandbox and run `rzv disks detach <disk-id>` first.")
	case apiErr.Status == 409 && apiErr.Code == "disk_not_attached":
		return newError(apiErr.Code, exitFailure, "That disk is not attached to a sandbox.")
	case apiErr.Status == 409 && apiErr.Code == "disk_wrong_state":
		return newError(apiErr.Code, exitFailure, "That disk is being deleted.")
	case apiErr.Status == 409 && apiErr.Code == "disk_host_mismatch":
		return newError(apiErr.Code, exitFailure, "That sandbox's files are on a different server from the disk, and neither can move. Create a new sandbox with `--disk` instead.")
	case apiErr.Status == 409 && apiErr.Code == "sandbox_has_disk":
		return newError(apiErr.Code, exitFailure, "That sandbox already has a persistent disk.")
	case apiErr.Status == 409 && apiErr.Code == "sandbox_must_be_stopped":
		return newError(apiErr.Code, exitFailure, "Stop the sandbox first with `rzv sandboxes stop <sandbox-id>`.")
	case apiErr.Status == 404 && apiErr.Code == "sandbox_not_found":
		return notFoundError("That sandbox was not found in this workspace. List them with `rzv sandboxes list`.")
	case apiErr.Status == 409 && apiErr.Code == "sandbox_limit_reached":
		return newError(apiErr.Code, exitFailure, "This workspace is already running its maximum number of sandboxes. Stop or delete one first.")
	case apiErr.Status == 409 && apiErr.Code == "credit_exhausted":
		return newError(apiErr.Code, exitFailure, "This workspace is out of credit for sandboxes. Add credit, then try again.")
	case apiErr.Status == 409 && apiErr.Code == "sandbox_wrong_state":
		return newError(apiErr.Code, exitFailure, "The sandbox is not in a state that allows this. Check `rzv sandboxes list`.")
	case apiErr.Status == 409 && apiErr.Code == "size_unavailable":
		return newError(apiErr.Code, exitFailure, "That size is not offered on this Runzivo environment. `rzv sandboxes list` shows the sizes on offer.")
	case apiErr.Status == 409 && apiErr.Code == "sandbox_expired":
		return newError(apiErr.Code, exitFailure, "The sandbox reached its eight hour lifetime and cannot start again. Create a new one.")
	case apiErr.Status == 409 && apiErr.Code == "sandbox_name_taken":
		return newError(apiErr.Code, exitFailure, "A sandbox with that name already exists in this workspace.")
	case apiErr.Status == 409 && apiErr.Code == "no_capacity":
		return newError(apiErr.Code, exitFailure, "No capacity is available to start the sandbox right now. Try again in a few minutes.")
	case apiErr.Status == 409 && apiErr.Code == "workspace_blocked":
		return newError(apiErr.Code, exitAccessDenied, "This workspace is blocked.")
	case apiErr.Status == 409 && apiErr.Code == "session_limit_reached":
		return newError(apiErr.Code, exitFailure, "This sandbox already has the most sessions open at once (4 commands and 2 terminals). Wait for one to end, then try again.")
	case apiErr.Status == 429 && apiErr.Code == "ticket_limit_reached":
		return newError(apiErr.Code, exitFailure, "This sandbox has 10 terminal tickets waiting to be used. Wait 30 seconds for them to expire, then try again.")
	case apiErr.Status == 503 && apiErr.Code == "sandboxes_unavailable":
		return newError(apiErr.Code, exitUnavailable, "Sandboxes are not available on this Runzivo environment yet.")
	case apiErr.Status == 503 && apiErr.Code == "sandbox_exec_unavailable":
		return newError(apiErr.Code, exitUnavailable, "Running commands in sandboxes is not available on this Runzivo environment yet.")
	case apiErr.Status == 403 && apiErr.Code == "permission_required":
		return newError(apiErr.Code, exitAccessDenied, "Your workspace role does not allow this change.")
	case apiErr.Status == 403 && (apiErr.Code == "workspace_blocked" || apiErr.Code == "workspace_closed"):
		return newError(apiErr.Code, exitAccessDenied, "This workspace is %s.", strings.TrimPrefix(apiErr.Code, "workspace_"))
	case apiErr.Status == 403:
		return accessError("The requested workspace is not authorized for this account.")
	case apiErr.Status == 404 && apiErr.Code == "not_a_member":
		return notFoundError("That person is not a member of this workspace. Invite them first.")
	case apiErr.Status == 404 && apiErr.Code == "not_found" && notFound != "":
		return notFoundError("%s", notFound)
	case apiErr.Status >= 500:
		return newError(apiErr.Code, exitUnavailable, "The customer API request failed (%d %s).", apiErr.Status, apiErr.Code)
	default:
		return newError(apiErr.Code, exitFailure, "The customer API request failed (%d %s).", apiErr.Status, apiErr.Code)
	}
}
