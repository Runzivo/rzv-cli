package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Runzivo/rzv-cli/internal/api"
)

const billingUsage = `Usage:
  rzv billing [--workspace <id>] [--json]                       Minute balance and packs
  rzv billing purchases [--workspace <id>] [--json]             Recent minute-pack orders
  rzv billing invoices [--workspace <id>] [--json]              Receipts and invoices
  rzv billing auto-recharge [--workspace <id>] [--json]         Auto-recharge settings
  rzv billing auto-recharge set --enabled <true|false> --threshold <minutes> --pack <id> [--workspace <id>] [--json]
  rzv billing country [--workspace <id>] [--json]               Workspace billing country
  rzv billing country set <code> [--workspace <id>] [--json]    Set the billing country (future purchases)
  rzv billing marketplace [--workspace <id>] [--json]           GitHub Marketplace plan, if any
  rzv billing checkout --pack <id> --operation-id <id> [--workspace <id>] [--json]
                                                                Start a hosted checkout
  rzv billing portal [--workspace <id>] [--json]                Open the provider billing portal link

Checkout and portal print a link. Reuse --operation-id when you retry the same
purchase. A link is a sign-in capability: do not store it. Refresh purchases to
see an order's result. Live payments stay off until Runzivo enables them.`

func init() {
	register("billing", "Show the minute balance, purchases and billing controls", runBilling)
	commandUsage["billing"] = billingUsage
}

func runBilling(a *app, cmd *command) error {
	switch {
	case len(cmd.args) == 0:
		if err := cmd.allow("workspace", "account"); err != nil {
			return err
		}
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
		if err := cmd.allow("workspace", "account"); err != nil {
			return err
		}
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
	case cmd.args[0] == "invoices":
		return runBillingInvoices(a, cmd)
	case cmd.args[0] == "auto-recharge":
		return runBillingAutoRecharge(a, cmd)
	case cmd.args[0] == "country":
		return runBillingCountry(a, cmd)
	case cmd.args[0] == "marketplace":
		return runBillingMarketplace(a, cmd)
	case cmd.args[0] == "checkout":
		return runBillingCheckout(a, cmd)
	case cmd.args[0] == "portal":
		return runBillingPortal(a, cmd)
	default:
		return usageError("%s", billingUsage)
	}
}

func runBillingInvoices(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	if err := cmd.expectArgs(1, "rzv billing invoices [--workspace <id>] [--json]"); err != nil {
		return err
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	invoices, err := client.Invoices(a.ctx, id)
	if err != nil {
		return apiError(err, "")
	}
	a.output(cmd, map[string]any{"workspace_id": id, "invoices": invoices.Invoices, "limit": invoices.Limit}, renderInvoices(id, len(invoices.Invoices)))
	return nil
}

func runBillingAutoRecharge(a *app, cmd *command) error {
	if len(cmd.args) > 1 && cmd.args[1] == "set" {
		if err := cmd.allow("workspace", "account", "enabled", "threshold", "pack"); err != nil {
			return err
		}
		if err := cmd.expectArgs(2, "rzv billing auto-recharge set --enabled <true|false> --threshold <minutes> --pack <id>"); err != nil {
			return err
		}
		enabledText, err := cmd.required("enabled")
		if err != nil {
			return err
		}
		enabled, err := strconv.ParseBool(enabledText)
		if err != nil {
			return usageError("--enabled must be true or false.")
		}
		thresholdText, err := cmd.required("threshold")
		if err != nil {
			return err
		}
		threshold, err := strconv.ParseInt(thresholdText, 10, 64)
		if err != nil || threshold < 0 {
			return usageError("--threshold must be a whole number of minutes.")
		}
		packID, err := cmd.required("pack")
		if err != nil {
			return err
		}
		id, client, err := a.workspaceClient(cmd)
		if err != nil {
			return err
		}
		settings, err := client.SetAutoRecharge(a.ctx, id, api.AutoRechargeInput{Enabled: enabled, ThresholdMinutes: threshold, PackID: packID})
		if err != nil {
			return apiError(err, "")
		}
		a.output(cmd, map[string]any{"workspace_id": id, "auto_recharge": settings}, renderAutoRecharge(id, settings))
		return nil
	}
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	if err := cmd.expectArgs(1, "rzv billing auto-recharge [--workspace <id>] [--json]"); err != nil {
		return err
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	settings, err := client.AutoRecharge(a.ctx, id)
	if err != nil {
		return apiError(err, "")
	}
	a.output(cmd, map[string]any{"workspace_id": id, "auto_recharge": settings}, renderAutoRecharge(id, settings))
	return nil
}

func runBillingCountry(a *app, cmd *command) error {
	if len(cmd.args) > 1 && cmd.args[1] == "set" {
		if err := cmd.allow("workspace", "account"); err != nil {
			return err
		}
		if err := cmd.expectArgs(3, "rzv billing country set <code> [--workspace <id>] [--json]"); err != nil {
			return err
		}
		id, client, err := a.workspaceClient(cmd)
		if err != nil {
			return err
		}
		country, err := client.SetBillingCountry(a.ctx, id, cmd.args[2])
		if err != nil {
			return apiError(err, "")
		}
		a.output(cmd, map[string]any{"workspace_id": id, "billing_country": country}, "Billing country for workspace "+display(id)+": "+display(country))
		return nil
	}
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	if err := cmd.expectArgs(1, "rzv billing country [--workspace <id>] [--json]"); err != nil {
		return err
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	country, err := client.BillingCountry(a.ctx, id)
	if err != nil {
		return apiError(err, "")
	}
	a.output(cmd, map[string]any{"workspace_id": id, "billing_country": country}, "Billing country for workspace "+display(id)+": "+display(country))
	return nil
}

func runBillingMarketplace(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	if err := cmd.expectArgs(1, "rzv billing marketplace [--workspace <id>] [--json]"); err != nil {
		return err
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	plan, err := client.Marketplace(a.ctx, id)
	if err != nil {
		return apiError(err, "")
	}
	a.output(cmd, map[string]any{"workspace_id": id, "marketplace": plan}, "Marketplace plan for workspace "+display(id)+"; use --json for the details.")
	return nil
}

func runBillingCheckout(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account", "pack", "operation-id"); err != nil {
		return err
	}
	if err := cmd.expectArgs(1, "rzv billing checkout --pack <id> --operation-id <id>"); err != nil {
		return err
	}
	packID, err := cmd.required("pack")
	if err != nil {
		return err
	}
	operationID, err := cmd.required("operation-id")
	if err != nil {
		return err
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	result, err := client.Checkout(a.ctx, id, packID, operationID)
	if err != nil {
		return apiError(err, "")
	}
	text := fmt.Sprintf("Checkout started for order %s (%s mode). Open this link to pay:\n%s\nRetry with --operation-id %s if this command fails.", display(result.OrderReference), display(result.PaymentMode), result.CheckoutURL, operationID)
	a.output(cmd, map[string]any{"workspace_id": id, "operation_id": operationID, "checkout": result}, text)
	return nil
}

func runBillingPortal(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	if err := cmd.expectArgs(1, "rzv billing portal [--workspace <id>] [--json]"); err != nil {
		return err
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	link, err := client.ProviderPortal(a.ctx, id)
	if err != nil {
		return apiError(err, "")
	}
	a.output(cmd, map[string]any{"workspace_id": id, "billing_portal_url": link}, "Billing portal link (do not store it):\n"+link)
	return nil
}

func renderInvoices(id string, count int) string {
	if count == 0 {
		return "No invoices for workspace " + display(id) + "."
	}
	return fmt.Sprintf("%d invoice(s) for workspace %s; use --json for the details.", count, display(id))
}

func renderAutoRecharge(id string, s *api.AutoRecharge) string {
	state := "off"
	if s.Enabled {
		state = "on"
	}
	lines := []string{
		fmt.Sprintf("Auto-recharge for workspace %s: %s", display(id), state),
		fmt.Sprintf("Threshold: %d minutes; pack: %s", s.ThresholdMinutes, display(s.PackID)),
		fmt.Sprintf("Saved payment method: %t", s.PaymentMethodSaved),
	}
	if s.LastFailureCode != "" {
		lines = append(lines, "Last failure: "+s.LastFailureCode)
	}
	return strings.Join(lines, "\n")
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
