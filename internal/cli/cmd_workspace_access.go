package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/Runzivo/rzv-cli/internal/api"
)

const workspaceAccessUsage = `Usage:
  rzv workspaces settings show [--workspace <id>] [--json]
  rzv workspaces settings update [--name <name>] [--logo <PNG-or-JPEG>] [--remove-logo] [--workspace <id>] [--json]
  rzv workspaces members list [--workspace <id>] [--json]
  rzv workspaces members role <identity-id> --role <role> [--workspace <id>] [--json]
  rzv workspaces members remove <identity-id> [--workspace <id>] [--json]
  rzv workspaces invitations list [--workspace <id>] [--json]
  rzv workspaces invitations invite --email <email> --role <role> [--team-ids <id,...>] [--workspace <id>] [--json]
  rzv workspaces invitations resend <invitation-id> [--workspace <id>] [--json]
  rzv workspaces invitations revoke <invitation-id> [--workspace <id>] [--json]
  rzv workspaces invitations teams <invitation-id> --team-ids <id,...> [--workspace <id>] [--json]
  rzv workspaces invitations accept (--token <token> | --token-file <path>) [--json]

Use existing custom team IDs with --team-ids. An empty --team-ids value clears
the invitation's custom teams. Invitation acceptance needs your own sign-in and
does not require a selected workspace.`

func workspaceAccess(a *app, cmd *command) error {
	if len(cmd.args) < 2 {
		return usageError("%s", workspaceAccessUsage)
	}
	switch cmd.args[0] {
	case "settings":
		switch cmd.args[1] {
		case "show":
			return workspaceSettingsShow(a, cmd)
		case "update":
			return workspaceSettingsUpdate(a, cmd)
		}
	case "members":
		switch cmd.args[1] {
		case "list":
			return workspaceMembersList(a, cmd)
		case "role":
			return workspaceMembersRole(a, cmd)
		case "remove":
			return workspaceMembersRemove(a, cmd)
		}
	case "invitations":
		switch cmd.args[1] {
		case "list":
			return workspaceInvitationsList(a, cmd)
		case "invite":
			return workspaceInvitationsInvite(a, cmd)
		case "resend":
			return workspaceInvitationsResend(a, cmd)
		case "revoke":
			return workspaceInvitationsRevoke(a, cmd)
		case "teams":
			return workspaceInvitationsTeams(a, cmd)
		case "accept":
			return workspaceInvitationsAccept(a, cmd)
		}
	}
	return usageError("%s", workspaceAccessUsage)
}

func workspaceSettingsShow(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	if err := cmd.expectArgs(2, "rzv workspaces settings show [--workspace <id>] [--json]"); err != nil {
		return err
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	settings, err := client.WorkspaceSettings(a.ctx, id)
	if err != nil {
		return apiError(err, "Workspace settings were not found.")
	}
	a.output(cmd, map[string]any{"workspace_id": id, "settings": settings}, renderWorkspaceSettings(id, settings))
	return nil
}

func workspaceSettingsUpdate(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account", "name", "logo", "remove-logo"); err != nil {
		return err
	}
	if err := cmd.expectArgs(2, "rzv workspaces settings update [--name <name>] [--logo <PNG-or-JPEG>] [--remove-logo] [--workspace <id>] [--json]"); err != nil {
		return err
	}
	name, err := cmd.option("name")
	if err != nil {
		return err
	}
	logoPath, err := cmd.option("logo")
	if err != nil {
		return err
	}
	removeLogo := cmd.flags["remove-logo"]
	if name == "" && logoPath == "" && !removeLogo {
		return usageError("Pass --name, --logo, or --remove-logo.")
	}
	if logoPath != "" && removeLogo {
		return usageError("Use --logo or --remove-logo, not both.")
	}
	patch := api.WorkspaceSettingsPatch{RemoveLogo: removeLogo}
	if name != "" {
		name = strings.TrimSpace(name)
		if name == "" || len(name) > 128 {
			return usageError("--name must contain at most 128 bytes of non-space text.")
		}
		patch.Name = &name
	}
	if logoPath != "" {
		logo, err := readWorkspaceLogo(logoPath)
		if err != nil {
			return err
		}
		patch.Logo = logo
	}
	if err := validateWorkspaceSettingsPatch(patch); err != nil {
		return err
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	settings, err := client.UpdateWorkspaceSettings(a.ctx, id, patch)
	if err != nil {
		return apiError(err, "Workspace settings were not found.")
	}
	a.output(cmd, map[string]any{"workspace_id": id, "settings": settings}, "Updated workspace settings.\n"+renderWorkspaceSettings(id, settings))
	return nil
}

func validateWorkspaceSettingsPatch(patch api.WorkspaceSettingsPatch) error {
	data, err := json.Marshal(patch)
	if err != nil || len(data) > 256<<10 {
		return usageError("Workspace settings update is too large.")
	}
	return nil
}

func readWorkspaceLogo(path string) (*api.WorkspaceLogo, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, usageError("Cannot read --logo: %s.", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (192<<10)+1))
	if err != nil {
		return nil, usageError("Cannot read --logo: %s.", err)
	}
	if len(data) == 0 || len(data) > 192<<10 {
		return nil, usageError("--logo must be a PNG or JPEG no larger than 192 KiB.")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 512 || config.Height > 512 {
		return nil, usageError("--logo must be a PNG or JPEG between 1 and 512 pixels in each dimension.")
	}
	if _, _, err = image.Decode(bytes.NewReader(data)); err != nil {
		return nil, usageError("--logo must be a valid PNG or JPEG.")
	}
	contentType := ""
	switch format {
	case "png":
		contentType = "image/png"
	case "jpeg":
		contentType = "image/jpeg"
	default:
		return nil, usageError("--logo must be a PNG or JPEG.")
	}
	return &api.WorkspaceLogo{ContentType: contentType, Base64: base64.StdEncoding.EncodeToString(data)}, nil
}

func renderWorkspaceSettings(id string, settings *api.WorkspaceSettings) string {
	logo := "none"
	if settings.Logo != nil {
		logo = settings.Logo.ContentType
	}
	return fmt.Sprintf("Workspace settings for %s:\nName: %s\nLogo: %s\nCan update: %t", display(id), display(settings.Name), display(logo), settings.CanUpdate)
}

func workspaceMembersList(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	if err := cmd.expectArgs(2, "rzv workspaces members list [--workspace <id>] [--json]"); err != nil {
		return err
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	members, err := client.WorkspaceMembers(a.ctx, id)
	if err != nil {
		return apiError(err, "Workspace members were not found.")
	}
	a.output(cmd, map[string]any{"workspace_id": id, "members": members}, renderWorkspaceMembers(id, members))
	return nil
}

func renderWorkspaceMembers(id string, members *api.WorkspaceMembers) string {
	var table bytes.Buffer
	w := tabwriter.NewWriter(&table, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "EMAIL\tIDENTITY ID\tROLE\tCHANGE ROLE\tREMOVE\tALLOWED ROLES")
	for _, member := range members.Members {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%t\t%t\t%s\n", displayOrDash(member.Email), display(member.IdentityID), display(member.Role), member.CanChangeRole, member.CanRemove, display(strings.Join(member.AllowedRoles, ",")))
	}
	_ = w.Flush()
	lines := []string{fmt.Sprintf("Members for workspace %s (invite: %t, manage invitations: %t):", display(id), members.CanInvite, members.CanManageInvitations)}
	if len(members.Members) == 0 {
		lines = append(lines, "No members.")
	} else {
		lines = append(lines, strings.TrimSuffix(table.String(), "\n"))
	}
	return strings.Join(lines, "\n")
}

func workspaceMembersRole(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account", "role"); err != nil {
		return err
	}
	if err := cmd.expectArgs(3, "rzv workspaces members role <identity-id> --role <role> [--workspace <id>] [--json]"); err != nil {
		return err
	}
	role, err := cmd.required("role")
	if err != nil {
		return err
	}
	if err := workspaceRole(role); err != nil {
		return err
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	member, err := client.UpdateWorkspaceMemberRole(a.ctx, id, cmd.args[2], role)
	if err != nil {
		return apiError(err, "The member was not found in this workspace.")
	}
	a.output(cmd, map[string]any{"workspace_id": id, "member": member}, fmt.Sprintf("Updated member %s to role %s.", display(member.IdentityID), display(member.Role)))
	return nil
}

func workspaceMembersRemove(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	if err := cmd.expectArgs(3, "rzv workspaces members remove <identity-id> [--workspace <id>] [--json]"); err != nil {
		return err
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	identityID := cmd.args[2]
	if err := client.RemoveWorkspaceMember(a.ctx, id, identityID); err != nil {
		return apiError(err, "The member was not found in this workspace.")
	}
	a.output(cmd, map[string]any{"workspace_id": id, "removed_identity_id": identityID}, "Removed member "+display(identityID)+".")
	return nil
}

func workspaceInvitationsList(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	if err := cmd.expectArgs(2, "rzv workspaces invitations list [--workspace <id>] [--json]"); err != nil {
		return err
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	invitations, err := client.WorkspaceInvitations(a.ctx, id)
	if err != nil {
		return apiError(err, "Workspace invitations were not found.")
	}
	a.output(cmd, map[string]any{"workspace_id": id, "invitations": invitations}, renderWorkspaceInvitations(id, invitations))
	return nil
}

func renderWorkspaceInvitations(id string, invitations []api.WorkspaceInvitation) string {
	if len(invitations) == 0 {
		return "No invitations for workspace " + display(id) + "."
	}
	var table bytes.Buffer
	w := tabwriter.NewWriter(&table, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "EMAIL\tINVITATION ID\tROLE\tSTATUS\tTEAMS\tRESEND\tREVOKE")
	for _, invitation := range invitations {
		teams := make([]string, len(invitation.Teams))
		for i, team := range invitation.Teams {
			teams[i] = team.Name + " [" + team.TeamID + "]"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%t\t%t\n", display(invitation.Email), display(invitation.InvitationID), display(invitation.Role), display(invitation.Status), display(strings.Join(teams, ",")), invitation.CanResend, invitation.CanRevoke)
	}
	_ = w.Flush()
	lines := []string{"Invitations for workspace " + display(id) + ":", strings.TrimSuffix(table.String(), "\n")}
	return strings.Join(lines, "\n")
}

func workspaceInvitationsInvite(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account", "email", "role", "team-ids"); err != nil {
		return err
	}
	if err := cmd.expectArgs(2, "rzv workspaces invitations invite --email <email> --role <role> [--team-ids <id,...>] [--workspace <id>] [--json]"); err != nil {
		return err
	}
	email, err := cmd.required("email")
	if err != nil {
		return err
	}
	role, err := cmd.required("role")
	if err != nil {
		return err
	}
	if err := workspaceRole(role); err != nil {
		return err
	}
	teamIDs, present, err := teamIDsOption(cmd, false)
	if err != nil {
		return err
	}
	input := api.CreateWorkspaceInvitationInput{Email: email, Role: role}
	if present {
		input.TeamIDs = teamIDs
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	invitation, err := client.CreateWorkspaceInvitation(a.ctx, id, input)
	if err != nil {
		return apiError(err, "Workspace invitations were not found.")
	}
	a.output(cmd, map[string]any{"workspace_id": id, "invitation": invitation}, "Invited "+display(invitation.Email)+".")
	return nil
}

// workspaceRole checks the public role enum only. The service still enforces
// whether the caller can assign a valid role.
func workspaceRole(role string) error {
	switch role {
	case "owner", "admin", "member":
		return nil
	default:
		return usageError("--role must be owner, admin, or member.")
	}
}

func teamIDsOption(cmd *command, required bool) ([]string, bool, error) {
	raw, present := cmd.options["team-ids"]
	if !present {
		if required {
			return nil, false, usageError("--team-ids is required; pass an empty value to clear custom teams.")
		}
		return nil, false, nil
	}
	if strings.TrimSpace(raw) == "" {
		return []string{}, true, nil
	}
	ids := []string{}
	seen := map[string]bool{}
	for _, id := range strings.Split(raw, ",") {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			return nil, true, usageError("--team-ids must be a comma-separated list without empty or repeated IDs.")
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) > 20 {
		return nil, true, usageError("--team-ids accepts at most 20 IDs.")
	}
	return ids, true, nil
}

func workspaceInvitationsResend(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	if err := cmd.expectArgs(3, "rzv workspaces invitations resend <invitation-id> [--workspace <id>] [--json]"); err != nil {
		return err
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	invitation, err := client.ResendWorkspaceInvitation(a.ctx, id, cmd.args[2])
	if err != nil {
		return apiError(err, "The invitation was not found in this workspace.")
	}
	a.output(cmd, map[string]any{"workspace_id": id, "invitation": invitation}, "Resent invitation to "+display(invitation.Email)+".")
	return nil
}

func workspaceInvitationsRevoke(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	if err := cmd.expectArgs(3, "rzv workspaces invitations revoke <invitation-id> [--workspace <id>] [--json]"); err != nil {
		return err
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	invitationID := cmd.args[2]
	if err := client.RevokeWorkspaceInvitation(a.ctx, id, invitationID); err != nil {
		return apiError(err, "The invitation was not found in this workspace.")
	}
	a.output(cmd, map[string]any{"workspace_id": id, "revoked_invitation_id": invitationID}, "Revoked invitation "+display(invitationID)+".")
	return nil
}

func workspaceInvitationsTeams(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account", "team-ids"); err != nil {
		return err
	}
	if err := cmd.expectArgs(3, "rzv workspaces invitations teams <invitation-id> --team-ids <id,...> [--workspace <id>] [--json]"); err != nil {
		return err
	}
	teamIDs, _, err := teamIDsOption(cmd, true)
	if err != nil {
		return err
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	invitation, err := client.UpdateWorkspaceInvitationTeams(a.ctx, id, cmd.args[2], teamIDs)
	if err != nil {
		return apiError(err, "The invitation was not found in this workspace.")
	}
	a.output(cmd, map[string]any{"workspace_id": id, "invitation": invitation}, "Updated custom teams for invitation "+display(invitation.InvitationID)+".")
	return nil
}

func workspaceInvitationsAccept(a *app, cmd *command) error {
	if err := cmd.allow("token", "token-file"); err != nil {
		return err
	}
	if err := cmd.expectArgs(2, "rzv workspaces invitations accept (--token <token> | --token-file <path>) [--json]"); err != nil {
		return err
	}
	if err := a.requireHuman(); err != nil {
		return err
	}
	token, err := invitationToken(cmd)
	if err != nil {
		return err
	}
	client, _, err := a.humanClient()
	if err != nil {
		return err
	}
	membership, err := client.AcceptWorkspaceInvitation(a.ctx, token)
	if err != nil {
		return acceptWorkspaceInvitationError(err)
	}
	a.output(cmd, map[string]any{"membership": membership}, fmt.Sprintf("Accepted invitation to workspace %s [%s].", display(membership.TenantName), display(membership.TenantID)))
	return nil
}

func acceptWorkspaceInvitationError(err error) error {
	var apiErr *api.Error
	if errors.As(err, &apiErr) && apiErr.Status == 403 && apiErr.Code == "invitation_recipient_verification_required" {
		return newError(apiErr.Code, exitAccessDenied, "Your signed-in identity needs a verified email that matches this invitation's recipient.")
	}
	return apiError(err, "The invitation was not found or is no longer available.")
}

func invitationToken(cmd *command) (string, error) {
	token, err := cmd.option("token")
	if err != nil {
		return "", err
	}
	path, err := cmd.option("token-file")
	if err != nil {
		return "", err
	}
	if (token == "" && path == "") || (token != "" && path != "") {
		return "", usageError("Pass exactly one of --token or --token-file.")
	}
	if path == "" {
		return token, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return "", usageError("Cannot read --token-file: %s.", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (64<<10)+1))
	if err != nil {
		return "", usageError("Cannot read --token-file: %s.", err)
	}
	if len(data) > 64<<10 {
		return "", usageError("--token-file is larger than 64 KiB.")
	}
	token = strings.TrimSpace(string(data))
	if token == "" {
		return "", usageError("--token-file is empty.")
	}
	return token, nil
}
