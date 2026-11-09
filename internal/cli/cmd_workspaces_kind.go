package cli

import (
	"errors"
	"fmt"

	"github.com/Runzivo/rzv-cli/internal/api"
)

const confirmKindUsage = "rzv workspaces confirm-kind --kind <personal|organization> [--yes] [--workspace <id>] [--json]"

// workspacesConfirmKind lets an owner confirm whether a workspace is personal
// or an organization. Without --yes it sends a dry run, which the service
// answers without writing anything. With --yes it writes the change. Once
// confirmed, only Runzivo staff can change the kind.
func workspacesConfirmKind(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account", "kind", "yes"); err != nil {
		return err
	}
	if err := cmd.expectArgs(1, confirmKindUsage); err != nil {
		return err
	}
	kind, err := cmd.required("kind")
	if err != nil {
		return err
	}
	if kind != "personal" && kind != "organization" {
		return usageError("%s", a.text("--kind must be personal or organization.", "--kind doit être personal ou organization."))
	}
	confirm := cmd.flags["yes"]
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	result, err := client.ConfirmWorkspaceKind(a.ctx, id, kind, confirm)
	if err != nil {
		return a.confirmKindError(err)
	}
	a.output(cmd, map[string]any{"workspace_id": id, "confirmed": confirm, "workspace_kind": result}, a.renderConfirmKind(id, confirm, result))
	return nil
}

func (a *app) renderConfirmKind(workspaceID string, confirmed bool, result *api.WorkspaceKindResult) string {
	if !confirmed {
		return fmt.Sprintf(a.text(
			"Workspace %s would be confirmed as %s. Nothing was changed.\nOnce the type is confirmed, only Runzivo staff can change it. Run again with --yes to confirm.",
			"L’espace de travail %s serait confirmé comme %s. Rien n’a été modifié.\nUne fois le type confirmé, seul le personnel de Runzivo peut le modifier. Relancez la commande avec --yes pour confirmer."),
			display(workspaceID), a.kindName(result.Kind))
	}
	return fmt.Sprintf(a.text(
		"Confirmed workspace %s as %s (change %s). Only Runzivo staff can change the type now.",
		"L’espace de travail %s a été confirmé comme %s (modification %s). Seul le personnel de Runzivo peut maintenant en modifier le type."),
		display(workspaceID), a.kindName(result.Kind), display(result.ChangeID))
}

// kindName is the kind in the sentence "confirmed as ...". The service answer
// is escaped like every server string.
func (a *app) kindName(kind string) string {
	switch kind {
	case "personal":
		return a.text("a personal workspace", "espace personnel")
	case "organization":
		return a.text("an organization workspace", "espace d’organisation")
	}
	return display(kind)
}

// kindLabel is a workspace's kind for `rzv workspaces list`: "personal
// (unconfirmed)" while the kind is only inferred, and empty when the service
// does not report one.
func (a *app) kindLabel(m api.Membership) string {
	if m.Kind == "" {
		return ""
	}
	label := display(m.Kind)
	switch m.Kind {
	case "personal":
		label = a.text("personal", "personnel")
	case "organization":
		label = a.text("organization", "organisation")
	}
	if m.KindSource == "inferred" {
		label += a.text(" (unconfirmed)", " (non confirmé)")
	}
	return label
}

// confirmKindError maps the refusals of confirmCustomerWorkspaceKind to
// one-line messages. Anything else, including reauth_required, takes the
// shared mapping in apiError.
func (a *app) confirmKindError(err error) error {
	var apiErr *api.Error
	if !errors.As(err, &apiErr) {
		return err
	}
	switch {
	case apiErr.Status == 400 && apiErr.Code == "invalid_request":
		return newError(apiErr.Code, exitUsage, "%s", a.text("The service rejected the request. --kind must be personal or organization.", "Le service a rejeté la demande. --kind doit être personal ou organization."))
	case apiErr.Status == 403 && apiErr.Code == "tenant_membership_required":
		return newError(apiErr.Code, exitAccessDenied, "%s", a.text("You are not a member of this workspace.", "Vous n’êtes pas membre de cet espace de travail."))
	case apiErr.Status == 403 && apiErr.Code == "workspace_owner_required":
		return newError(apiErr.Code, exitAccessDenied, "%s", a.text("Only a workspace owner can confirm the workspace type.", "Seul un propriétaire de l’espace de travail peut en confirmer le type."))
	case apiErr.Status == 403 && apiErr.Code == "api_key_scope_required":
		return newError(apiErr.Code, exitAccessDenied, "%s", a.text("Confirming a workspace's type needs a person signed in with `rzv login`; API keys cannot do it.", "Pour confirmer le type d’un espace de travail, une personne doit être connectée avec `rzv login`; les clés d’API ne peuvent pas le faire."))
	case apiErr.Status == 409 && apiErr.Code == "kind_already_confirmed":
		return newError(apiErr.Code, exitFailure, "%s", a.text("This workspace's type is already confirmed. Only Runzivo staff can change it.", "Le type de cet espace de travail est déjà confirmé. Seul le personnel de Runzivo peut le modifier."))
	case apiErr.Status == 409 && apiErr.Code == "personal_workspace_exists":
		return newError(apiErr.Code, exitFailure, "%s", a.text("You already own another personal workspace, so this one cannot also be personal. Use --kind organization only if this workspace belongs to an organization. Otherwise, contact Runzivo support before confirming.", "Vous possédez déjà un autre espace personnel; celui-ci ne peut donc pas l’être aussi. Utilisez --kind organization uniquement si cet espace appartient à une organisation. Sinon, communiquez avec le soutien de Runzivo avant de confirmer."))
	case apiErr.Status == 409 && apiErr.Code == "personal_workspace_has_members":
		return newError(apiErr.Code, exitFailure, "%s", a.text("A personal workspace has one member, and this workspace has more. Use --kind organization only if this workspace belongs to an organization. Otherwise, contact Runzivo support before confirming.", "Un espace personnel n’a qu’un seul membre, et celui-ci en a plusieurs. Utilisez --kind organization uniquement si cet espace appartient à une organisation. Sinon, communiquez avec le soutien de Runzivo avant de confirmer."))
	case apiErr.Status == 409 && apiErr.Code == "personal_requires_user_account":
		return newError(apiErr.Code, exitFailure, "%s", a.text("This workspace has a connected account that is not a personal account, so it cannot be personal. Use --kind organization only if this workspace belongs to an organization. Otherwise, contact Runzivo support before confirming.", "Cet espace de travail a un compte connecté qui n’est pas un compte personnel; il ne peut donc pas être personnel. Utilisez --kind organization uniquement si cet espace appartient à une organisation. Sinon, communiquez avec le soutien de Runzivo avant de confirmer."))
	case apiErr.Status == 404 && apiErr.Code == "not_found":
		return newError(apiErr.Code, exitUnavailable, "%s", a.text("Confirming a workspace's type is not available in this Runzivo environment yet.", "Confirmer le type d’un espace de travail n’est pas encore possible dans cet environnement Runzivo."))
	}
	return apiError(err, "")
}
