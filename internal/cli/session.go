package cli

import (
	"errors"
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
			id = selected
		}
		if id == "" {
			return "", nil, usageError("Pass --workspace <workspace-id> with RZV_API_KEY.")
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
		id = creds.SelectedWorkspace
	}
	if id == "" {
		return "", nil, usageError("Select a workspace with `rzv workspaces select <workspace-id>` or pass --workspace.")
	}
	memberships, err := client.Memberships(a.ctx)
	if err != nil {
		return "", nil, apiError(err, "")
	}
	for _, m := range memberships {
		if m.TenantID == id {
			return id, client, nil
		}
	}
	return "", nil, accessError("You are not a member of workspace %s.", display(id))
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
	case apiErr.Status == 403 && apiErr.Code == "insufficient_role":
		return newError(apiErr.Code, exitAccessDenied, "Your workspace role does not allow this command.")
	case apiErr.Status == 403 && apiErr.Code == "auto_recharge_enable_requires_person":
		return newError(apiErr.Code, exitAccessDenied, "Turning auto-recharge on needs a person signed in, not an API key.")
	case apiErr.Status == 403 && apiErr.Code == "reauth_required":
		return newError(apiErr.Code, exitAuth, "Sign in again with `rzv login`, then retry.")
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
