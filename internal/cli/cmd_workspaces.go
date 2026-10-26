package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/Runzivo/rzv-cli/internal/api"
)

const workspacesUsage = `Usage:
  rzv workspaces list [--json]                         Workspaces you belong to
  rzv workspaces select <workspace-id> [--json]        Use one for later commands
  rzv workspaces create --slug <slug> --name <name>    Create a workspace and select it
  rzv workspaces settings ...                           Read or update workspace settings
  rzv workspaces members ...                            List or manage workspace members
  rzv workspaces invitations ...                        Manage workspace invitations

"rzv accounts ..." is accepted as an older name for "rzv workspaces ...".

` + workspaceAccessUsage

func init() {
	register("workspaces", "Manage workspaces and workspace access", runWorkspaces)
	commandUsage["workspaces"] = workspacesUsage
}

func runWorkspaces(a *app, cmd *command) error {
	if len(cmd.args) == 0 {
		return usageError("%s", workspacesUsage)
	}
	switch cmd.args[0] {
	case "list":
		return workspacesList(a, cmd)
	case "select":
		return workspacesSelect(a, cmd)
	case "create":
		return workspacesCreate(a, cmd)
	case "settings", "members", "invitations":
		return workspaceAccess(a, cmd)
	default:
		return usageError("%s", workspacesUsage)
	}
}

func workspacesList(a *app, cmd *command) error {
	if err := cmd.allow(); err != nil {
		return err
	}
	if err := cmd.expectArgs(1, "rzv workspaces list [--json]"); err != nil {
		return err
	}
	client, creds, err := a.humanClient()
	if err != nil {
		return err
	}
	memberships, err := client.Memberships(a.ctx)
	if err != nil {
		return apiError(err, "")
	}
	var selected any
	if creds.SelectedWorkspace != "" {
		selected = creds.SelectedWorkspace
	}
	a.output(cmd, map[string]any{"workspaces": memberships, "selected_workspace_id": selected},
		renderWorkspaces(memberships, creds.SelectedWorkspace))
	return nil
}

func renderWorkspaces(memberships []api.Membership, selected string) string {
	if len(memberships) == 0 {
		return "No workspaces are available. Create one with `rzv workspaces create` or in the Runzivo portal."
	}
	lines := []string{"Workspaces:"}
	for _, m := range memberships {
		mark := ""
		if m.TenantID == selected {
			mark = " (selected)"
		}
		lines = append(lines, fmt.Sprintf("- %s [%s] (%s)%s", display(m.TenantName), display(m.TenantID), display(m.Role), mark))
	}
	return strings.Join(lines, "\n")
}

func workspacesSelect(a *app, cmd *command) error {
	if err := cmd.allow(); err != nil {
		return err
	}
	if err := cmd.expectArgs(2, "rzv workspaces select <workspace-id> [--json]"); err != nil {
		return err
	}
	id := cmd.args[1]
	client, creds, err := a.humanClient()
	if err != nil {
		return err
	}
	memberships, err := client.Memberships(a.ctx)
	if err != nil {
		return apiError(err, "")
	}
	found := false
	for _, m := range memberships {
		// Accept the slug too; store the id.
		if m.TenantID == id || m.TenantSlug == id {
			id, found = m.TenantID, true
			break
		}
	}
	if !found {
		return accessError("You are not a member of workspace %s.", display(id))
	}
	creds.SelectedWorkspace = id
	if err := a.Store.Write(creds); err != nil {
		return err
	}
	a.output(cmd, map[string]string{"selected_workspace_id": id}, "Selected workspace: "+display(id))
	return nil
}

func workspacesCreate(a *app, cmd *command) error {
	if err := cmd.allow("slug", "name"); err != nil {
		return err
	}
	if err := cmd.expectArgs(1, "rzv workspaces create --slug <slug> --name <name> [--json]"); err != nil {
		return err
	}
	slug, err := cmd.required("slug")
	if err != nil {
		return err
	}
	name, err := cmd.required("name")
	if err != nil {
		return err
	}
	client, creds, err := a.humanClient()
	if err != nil {
		return err
	}
	// Derived from slug and name, so retrying the same command replays the
	// original creation instead of conflicting.
	sum := sha256.Sum256([]byte(slug + "\n" + name))
	workspace, err := client.CreateWorkspace(a.ctx, api.CreateWorkspaceInput{Slug: slug, Name: name, IdempotencyKey: hex.EncodeToString(sum[:])})
	if err != nil {
		return createWorkspaceError(err, slug)
	}
	creds.SelectedWorkspace = workspace.TenantID
	if err := a.Store.Write(creds); err != nil {
		return err
	}
	a.output(cmd, map[string]any{"workspace": workspace, "selected_workspace_id": workspace.TenantID},
		fmt.Sprintf("Created workspace %s [%s] and selected it.", display(workspace.TenantName), display(workspace.TenantID)))
	return nil
}

func createWorkspaceError(err error, slug string) error {
	var apiErr *api.Error
	if !errors.As(err, &apiErr) {
		return err
	}
	switch {
	case apiErr.Status == 403 && apiErr.Code == "signups_closed":
		return newError("registration_closed", exitAccessDenied, "Self-service sign-up is closed. Existing customers can still sign in and use their workspaces.")
	case apiErr.Status == 403 && apiErr.Code == "captcha_verification_failed":
		return accessError("Workspace sign-up needs a CAPTCHA check. Create the workspace in the Runzivo portal instead.")
	case apiErr.Status == 503 && apiErr.Code == "captcha_verification_unavailable":
		return newError(apiErr.Code, exitUnavailable, "Workspace protection is temporarily unavailable. Try again shortly.")
	case apiErr.Status == 409 && apiErr.Code == "workspace_slug_taken":
		return conflictError("The workspace slug %s is already taken. Choose a different --slug.", display(slug))
	case apiErr.Status == 409 && apiErr.Code == "idempotency_key_conflict":
		return conflictError("An earlier attempt with this slug used a different name. Retry with the original --name, or choose a new --slug.")
	case apiErr.Status == 400:
		return usageError("The slug or name was rejected. --slug must be lowercase letters, digits and inner hyphens (at most 63 characters); --name must not be empty (at most 255 characters).")
	}
	return apiError(err, "")
}
