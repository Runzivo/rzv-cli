package cli

import (
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"github.com/Runzivo/rzv-cli/internal/api"
)

const billingUsage = `Usage:
  rzv billing [--workspace <id>] [--json]             Minute balance and packs
  rzv billing purchases [--workspace <id>] [--json]   Recent minute-pack orders
  rzv billing checkout --pack <id> [--operation-id <id>] [--no-browser] [--workspace <id>] [--json]
  rzv billing portal <order-reference> [--no-browser] [--workspace <id>] [--json]

Hosted links are transient. Checkout navigation does not confirm payment; run
rzv billing or rzv billing purchases afterwards to refresh.`

func init() {
	register("billing", "Show billing, start checkout, or open a purchase portal", runBilling)
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
	case cmd.args[0] == "checkout":
		return billingCheckout(a, cmd)
	case cmd.args[0] == "portal":
		return billingPortal(a, cmd)
	default:
		return usageError("%s", billingUsage)
	}
}

func billingCheckout(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account", "pack", "operation-id", "no-browser"); err != nil {
		return err
	}
	if err := cmd.expectArgs(1, "rzv billing checkout --pack <id> [--operation-id <id>] [--no-browser] [--workspace <id>] [--json]"); err != nil {
		return err
	}
	packID, err := cmd.required("pack")
	if err != nil {
		return err
	}
	operationID, err := cmd.option("operation-id")
	if err != nil {
		return err
	}
	if operationID == "" {
		operationID, err = newOperationID()
		if err != nil {
			return unavailableError("Could not generate a checkout operation ID.")
		}
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	context := checkoutAttempt{workspaceID: id, packID: packID, operationID: operationID}
	checkout, err := client.Checkout(a.ctx, id, api.CheckoutInput{PackID: packID, ClientOperationID: operationID})
	if err != nil {
		return checkoutError(err, context)
	}
	if err := hostedURL(checkout.CheckoutURL); err != nil {
		return checkoutUnknownError("The customer API returned an invalid checkout URL. Retry with the same --operation-id, never a new one.", context)
	}
	a.output(cmd, map[string]any{"workspace_id": id, "operation_id": operationID, "checkout": checkout}, renderCheckout(id, operationID, checkout))
	if !cmd.json() && !cmd.flags["no-browser"] && a.OpenBrowser != nil {
		if err := a.OpenBrowser(checkout.CheckoutURL); err != nil {
			fmt.Fprintf(a.Stderr, "rzv: Could not open the checkout URL. Open the URL printed above manually.\n")
		}
	}
	return nil
}

func billingPortal(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account", "no-browser"); err != nil {
		return err
	}
	if err := cmd.expectArgs(2, "rzv billing portal <order-reference> [--no-browser] [--workspace <id>] [--json]"); err != nil {
		return err
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	portal, err := client.BillingPortal(a.ctx, id, cmd.args[1])
	if err != nil {
		return billingPortalError(err)
	}
	if err := hostedURL(portal.CustomerPortalLink); err != nil {
		return newError("invalid_billing_portal_link", exitFailure, "The customer API returned an invalid billing portal link.")
	}
	a.output(cmd, map[string]any{"workspace_id": id, "customer_portal_link": portal.CustomerPortalLink}, "Billing portal link (transient, do not save):\n"+portal.CustomerPortalLink)
	if !cmd.json() && !cmd.flags["no-browser"] && a.OpenBrowser != nil {
		if err := a.OpenBrowser(portal.CustomerPortalLink); err != nil {
			fmt.Fprintln(a.Stderr, "rzv: Could not open the billing portal link. Open the URL printed above manually.")
		}
	}
	return nil
}

type checkoutAttempt struct{ workspaceID, packID, operationID string }

func (c checkoutAttempt) detail(message string) string {
	return fmt.Sprintf("%s (workspace %s, pack %s, operation-id %s)", message, display(c.workspaceID), display(c.packID), display(c.operationID))
}

func checkoutError(err error, attempt checkoutAttempt) error {
	var apiErr *api.Error
	if !errors.As(err, &apiErr) {
		return checkoutUnknownError("The checkout attempt's outcome is unknown. Retry with the same --operation-id.", attempt)
	}
	switch {
	case apiErr.Status == 401:
		return authError("%s", attempt.detail("Credentials were rejected or expired. Run `rzv login` again."))
	case apiErr.Status == 403 && apiErr.Code == "payer_verification_required":
		return newError(apiErr.Code, exitAccessDenied, "%s", attempt.detail("Your signed-in identity needs a verified email matching this purchase's payer."))
	case apiErr.Status == 403:
		return newError(apiErr.Code, exitAccessDenied, "%s", attempt.detail("The requested workspace is not authorized to start checkout."))
	case apiErr.Status == 400:
		return usageError("%s", attempt.detail("--pack was rejected by the billing configuration."))
	case apiErr.Status == 409 && apiErr.Code == "idempotency_key_conflict":
		return conflictError("%s", attempt.detail("--operation-id was already used with a different --pack. Retry with the original pack or choose a new operation ID."))
	case apiErr.Status == 409 && apiErr.Code == "billing_reconciliation_required":
		return conflictError("%s", attempt.detail("This operation has no saved checkout URL yet. Do not retry automatically; rerun with this same --operation-id."))
	case apiErr.Status == 409 && (apiErr.Code == "billing_country_required" || apiErr.Code == "country_not_supported"):
		return conflictError("%s", attempt.detail("Billing cannot start until this workspace has a configured, supported billing country."))
	case apiErr.Status == 503 && apiErr.Code == "billing_not_configured":
		return newError(apiErr.Code, exitUnavailable, "%s", attempt.detail("Billing checkout is not configured. Retry with the same --operation-id."))
	default:
		exit := exitFailure
		if apiErr.Status == 0 || apiErr.Status >= 500 {
			exit = exitUnavailable
		}
		return newError(apiErr.Code, exit, "%s", attempt.detail("The checkout attempt's outcome is unknown. Retry with the same --operation-id, never a new one."))
	}
}

func checkoutUnknownError(message string, attempt checkoutAttempt) error {
	return newError("checkout_outcome_unknown", exitFailure, "%s", attempt.detail(message))
}

func billingPortalError(err error) error {
	var apiErr *api.Error
	if errors.As(err, &apiErr) && apiErr.Status == 403 && apiErr.Code == "payer_verification_required" {
		return newError(apiErr.Code, exitAccessDenied, "Your signed-in identity needs a verified email matching this purchase's payer.")
	}
	return apiError(err, "The purchase was not found in this workspace.")
}

func newOperationID() (string, error) {
	data := make([]byte, 16)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	data[6] = data[6]&0x0f | 0x40
	data[8] = data[8]&0x3f | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", data[:4], data[4:6], data[6:8], data[8:10], data[10:]), nil
}

func hostedURL(raw string) error {
	if len(raw) == 0 || len(raw) > 2048 {
		return errors.New("length")
	}
	for _, r := range raw {
		if unicode.IsControl(r) {
			return errors.New("control character")
		}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Host == "" || u.Host != u.Hostname() {
		return errors.New("syntax")
	}
	host := u.Hostname()
	if host != strings.ToLower(host) || strings.HasSuffix(host, ".") || host == "localhost" || strings.HasSuffix(host, ".localhost") || net.ParseIP(host) != nil || !dnsName(host) {
		return errors.New("host")
	}
	return nil
}

func dnsName(host string) bool {
	if len(host) > 253 || !strings.Contains(host, ".") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !(r == '-' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
				return false
			}
		}
	}
	last := strings.Split(host, ".")[len(strings.Split(host, "."))-1]
	if numericDNSLabel(last) {
		return false
	}
	return true
}

func numericDNSLabel(label string) bool {
	digits := len(label) > 0
	for _, r := range label {
		if r < '0' || r > '9' {
			digits = false
			break
		}
	}
	if digits {
		return true
	}
	if !strings.HasPrefix(label, "0x") || len(label) == 2 {
		return false
	}
	for _, r := range label[2:] {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

func renderCheckout(id, operationID string, checkout *api.Checkout) string {
	return strings.Join([]string{"Started checkout for workspace " + display(id) + ".", "Operation ID: " + display(operationID), "Order reference: " + display(checkout.OrderReference), "Checkout URL (transient): " + checkout.CheckoutURL, "This URL is navigation only; it does not confirm payment. Run `rzv billing` or `rzv billing purchases` afterwards to refresh."}, "\n")
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
