package cli

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/Runzivo/rzv-cli/internal/api"
)

const (
	marketplaceClaimableUsage = "rzv billing marketplace claimable [--workspace <id>] [--json]"
	marketplaceClaimUsage     = "rzv billing marketplace claim --account-id <id> [--workspace <id>] [--json]"
)

// runBillingMarketplaceClaimable lists the held GitHub Marketplace purchases
// the workspace can claim. It is a read: no recent sign-in is needed.
func runBillingMarketplaceClaimable(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	if err := cmd.expectArgs(2, marketplaceClaimableUsage); err != nil {
		return err
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	claimable, err := client.MarketplaceClaimable(a.ctx, id)
	if err != nil {
		return a.marketplaceError(err)
	}
	a.output(cmd, map[string]any{"workspace_id": id, "claimable": claimable}, a.renderMarketplaceClaimable(claimable))
	return nil
}

// runBillingMarketplaceClaim links a held purchase to the workspace. A stale
// sign-in ends with the same `rzv login` guidance as the other billing
// writes; the command is safe to repeat afterwards.
func runBillingMarketplaceClaim(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account", "account-id"); err != nil {
		return err
	}
	if err := cmd.expectArgs(2, marketplaceClaimUsage); err != nil {
		return err
	}
	text, err := cmd.required("account-id")
	if err != nil {
		return err
	}
	accountID, err := strconv.ParseInt(text, 10, 64)
	if err != nil || accountID < 1 {
		return usageError("%s", a.text("--account-id must be a positive whole number (the GitHub account id from `rzv billing marketplace claimable`).", "--account-id doit être un entier positif (l’identifiant du compte GitHub affiché par `rzv billing marketplace claimable`)."))
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	claim, err := client.ClaimMarketplaceHold(a.ctx, id, accountID)
	if err != nil {
		return a.marketplaceError(err)
	}
	a.output(cmd, map[string]any{"workspace_id": id, "marketplace_claim": claim}, a.renderMarketplaceClaim(id, claim))
	return nil
}

func (a *app) renderMarketplaceClaimable(claimable []api.MarketplaceClaimable) string {
	if len(claimable) == 0 {
		return a.text("No GitHub Marketplace plan is waiting for this workspace.", "Aucun forfait GitHub Marketplace n’attend cet espace de travail.")
	}
	var table bytes.Buffer
	w := tabwriter.NewWriter(&table, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, a.text("ACCOUNT ID\tACCOUNT", "ID DU COMPTE\tCOMPTE"))
	for _, item := range claimable {
		_, _ = fmt.Fprintf(w, "%d\t%s\n", item.AccountID, displayOrDashString(item.AccountLogin))
	}
	_ = w.Flush()
	return strings.TrimSuffix(table.String(), "\n")
}

func (a *app) renderMarketplaceClaim(workspaceID string, claim *api.MarketplaceClaim) string {
	login := displayOrDashString(claim.AccountLogin)
	if claim.AlreadyLinked {
		return fmt.Sprintf(a.text("The GitHub Marketplace purchase of %s (account %d) is already linked to workspace %s. Nothing changed.", "L’achat GitHub Marketplace de %s (compte %d) est déjà lié à l’espace de travail %s. Rien n’a changé."), login, claim.AccountID, display(workspaceID))
	}
	return fmt.Sprintf(a.text("Linked the GitHub Marketplace purchase of %s (account %d) to workspace %s. The plan is issued on the next billing check.", "L’achat GitHub Marketplace de %s (compte %d) est lié à l’espace de travail %s. Le forfait sera émis à la prochaine vérification de facturation."), login, claim.AccountID, display(workspaceID))
}

// marketplaceError maps the refusals of the two Marketplace claim operations
// to one-line messages. Anything else, including reauth_required, takes the
// shared mapping in apiError.
func (a *app) marketplaceError(err error) error {
	var apiErr *api.Error
	if !errors.As(err, &apiErr) {
		return err
	}
	switch {
	case apiErr.Status == 403 && apiErr.Code == "workspace_owner_required":
		return newError(apiErr.Code, exitAccessDenied, "%s", a.text("Only a workspace owner can list or claim GitHub Marketplace plans.", "Seul un propriétaire de l’espace de travail peut lister ou réclamer des forfaits GitHub Marketplace."))
	case apiErr.Status == 403 && apiErr.Code == "api_key_scope_required":
		return newError(apiErr.Code, exitAccessDenied, "%s", a.text("GitHub Marketplace claims need a person signed in with `rzv login`; API keys cannot use them.", "Les réclamations GitHub Marketplace exigent une personne connectée avec `rzv login`; les clés d’API ne peuvent pas les utiliser."))
	case apiErr.Status == 409 && apiErr.Code == "marketplace_claim_not_owner":
		return newError(apiErr.Code, exitFailure, "%s", a.text("This workspace has no active connection of that GitHub account. Connect the account to this workspace, then try again.", "Cet espace de travail n’a aucune connexion active de ce compte GitHub. Connectez le compte à cet espace de travail, puis réessayez."))
	case apiErr.Status == 409 && apiErr.Code == "marketplace_claim_no_hold":
		return newError(apiErr.Code, exitFailure, "%s", a.text("That purchase is not waiting to be claimed (it may be linked elsewhere or cancelled). `rzv billing marketplace claimable` lists what can be claimed.", "Cet achat n’attend pas d’être réclamé (il est peut-être lié ailleurs ou annulé). `rzv billing marketplace claimable` liste ce qui peut être réclamé."))
	case apiErr.Status == 409 && apiErr.Code == "marketplace_workspace_has_plan":
		return newError(apiErr.Code, exitFailure, "%s", a.text("This workspace already has another active plan, so it cannot claim this purchase.", "Cet espace de travail a déjà un autre forfait actif; il ne peut pas réclamer cet achat."))
	case apiErr.Status == 503 && apiErr.Code == "billing_not_configured":
		return newError(apiErr.Code, exitUnavailable, "%s", a.text("Billing is not available on this Runzivo environment yet.", "La facturation n’est pas encore disponible sur cet environnement Runzivo."))
	}
	return apiError(err, "")
}
