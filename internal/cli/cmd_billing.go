package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Runzivo/rzv-cli/internal/api"
)

const billingUsage = `Usage:
  rzv billing [--workspace <id>] [--json]             Minute balance and packs
  rzv billing purchases [--workspace <id>] [--json]   Recent minute-pack orders

Buying minutes and the payment provider's billing portal are in the
Runzivo portal.`

func init() {
	register("billing", "Show the minute balance and recent purchases", runBilling)
	commandUsage["billing"] = billingUsage
}

func runBilling(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	switch {
	case len(cmd.args) == 0:
		id, client, err := a.workspaceClient(cmd)
		if err != nil {
			return err
		}
		billing, err := client.Billing(a.ctx, id)
		if err != nil {
			return apiError(err, "")
		}
		a.output(cmd, map[string]any{"workspace_id": id, "billing": billing}, renderBilling(id, billing))
		return nil
	case cmd.args[0] == "purchases":
		if err := cmd.expectArgs(1, "rzv billing purchases [--workspace <id>] [--json]"); err != nil {
			return err
		}
		id, client, err := a.workspaceClient(cmd)
		if err != nil {
			return err
		}
		purchases, err := client.Purchases(a.ctx, id)
		if err != nil {
			return apiError(err, "")
		}
		a.output(cmd, map[string]any{"workspace_id": id, "purchases": purchases.Purchases, "limit": purchases.Limit}, renderPurchases(id, purchases.Purchases))
		return nil
	default:
		return usageError("%s", billingUsage)
	}
}

func minutes(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

func renderBilling(id string, b *api.Billing) string {
	checkout := "no"
	if b.CheckoutAvailable {
		checkout = "yes"
	}
	lines := []string{
		fmt.Sprintf("Billing for workspace %s (%s mode):", display(id), display(b.PaymentMode)),
		"Checkout available: " + checkout,
	}
	if !b.AllowanceConfigured || b.Balance == nil {
		return strings.Join(append(lines, "No allowance is configured for this workspace."), "\n")
	}
	bal := b.Balance
	lines = append(lines,
		fmt.Sprintf("Free: %s available of %d (%s used, %s held)", minutes(bal.FreeAvailable), bal.FreeAllowanceMinutes, minutes(bal.FreeConsumedMinutes), minutes(bal.FreeReservedMinutes)),
		fmt.Sprintf("Purchased: %s available (%s remaining, %s held)", minutes(bal.PurchasedAvailable), minutes(bal.PurchasedRemainingMinutes), minutes(bal.PurchasedReservedMinutes)),
		"Held this period: "+minutes(bal.ReservedMinutes),
		fmt.Sprintf("Period: %s to %s", display(bal.PeriodStart), display(bal.PeriodEnd)),
	)
	if bal.ReviewHold {
		lines = append(lines, "Review hold: available minutes are zero until a manual review ends.")
	}
	if len(b.Packs) == 0 {
		return strings.Join(append(lines, "No minute packs are available."), "\n")
	}
	lines = append(lines, "Packs:")
	for _, p := range b.Packs {
		lines = append(lines, fmt.Sprintf("- %s: %d minutes for %d cents (%s)", display(p.ID), p.Minutes, p.AmountCents, display(p.Currency)))
	}
	return strings.Join(lines, "\n")
}

func renderPurchases(id string, purchases []api.Purchase) string {
	if len(purchases) == 0 {
		return "No purchases for workspace " + display(id) + "."
	}
	lines := []string{"Purchases for workspace " + display(id) + ":"}
	for _, p := range purchases {
		lines = append(lines, fmt.Sprintf("- %s %s: %d minutes for %d cents (%s) on %s", display(p.OrderReference), display(p.Status), p.Minutes, p.AmountCents, display(p.Currency), display(p.CreatedAt)))
	}
	return strings.Join(lines, "\n")
}
