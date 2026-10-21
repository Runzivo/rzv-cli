package cli

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/Runzivo/rzv-cli/internal/api"
)

const billingUsage = `Usage:
  rzv billing [--workspace <id>] [--json]             Minute balance and packs
  rzv billing purchases [--workspace <id>] [--json]   Recent minute-pack orders
  rzv billing codes redeem --code-file <path> [--workspace <id>] [--json]

Buying minutes and the payment provider's billing portal are in the
Runzivo portal.`

const maxRedeemCodeBytes = 64

func init() {
	register("billing", "Show billing, purchases, or redeem a code", runBilling)
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
	case len(cmd.args) > 1 && cmd.args[0] == "codes" && cmd.args[1] == "redeem":
		return billingCodesRedeem(a, cmd)
	default:
		return usageError("%s", billingUsage)
	}
}

func billingCodesRedeem(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account", "code-file"); err != nil {
		return err
	}
	if err := cmd.expectArgs(2, "rzv billing codes redeem --code-file <path> [--workspace <id>] [--json]"); err != nil {
		return err
	}
	path, err := cmd.required("code-file")
	if err != nil {
		return err
	}
	code, err := readRedeemCode(path)
	if err != nil {
		return err
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	result, err := client.RedeemCode(a.ctx, id, code)
	if err != nil {
		return apiError(err, "")
	}
	a.output(cmd, map[string]any{"workspace_id": id, "redemption": result.Redemption, "replayed": result.Replayed}, renderRedemption(id, result))
	return nil
}

func readRedeemCode(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", usageError("--code-file could not be read.")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxRedeemCodeBytes+3))
	if err != nil {
		return "", usageError("--code-file could not be read.")
	}
	if len(data) > maxRedeemCodeBytes+2 {
		return "", usageError("--code-file must contain one non-empty code of at most 64 bytes.")
	}
	if strings.HasSuffix(string(data), "\r\n") {
		data = data[:len(data)-2]
	} else if strings.HasSuffix(string(data), "\n") {
		data = data[:len(data)-1]
	}
	code := string(data)
	if code == "" || strings.TrimSpace(code) == "" || strings.ContainsAny(code, "\r\n") || len(code) > maxRedeemCodeBytes {
		return "", usageError("--code-file must contain one non-empty code of at most 64 bytes.")
	}
	return code, nil
}

func renderRedemption(id string, result *api.RedemptionResult) string {
	redemption := result.Redemption
	replayed := "no"
	if result.Replayed {
		replayed = "yes"
	}
	return strings.Join([]string{
		"Code redeemed for workspace " + display(id) + ":",
		"ID: " + display(redemption.ID),
		"Kind: " + display(redemption.Kind),
		"Code: " + display(redemption.Code),
		"Status: " + display(redemption.Status),
		fmt.Sprintf("Equivalent minutes: %d", redemption.EquivalentMinutes),
		"Credit expires: " + displayOrDash(redemption.CreditExpiresAt),
		"Replayed: " + replayed,
	}, "\n")
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
