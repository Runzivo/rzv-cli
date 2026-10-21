package api

import (
	"context"
	"net/http"
	"net/url"
)

// Balance is the workspace's minute balance for the current period.
type Balance struct {
	FreeAllowanceMinutes      int64   `json:"free_allowance_minutes"`
	FreeConsumedMinutes       float64 `json:"free_consumed_minutes"`
	FreeReservedMinutes       float64 `json:"free_reserved_minutes"`
	FreeAvailable             float64 `json:"free_available"`
	PurchasedRemainingMinutes float64 `json:"purchased_remaining_minutes"`
	PurchasedReservedMinutes  float64 `json:"purchased_reserved_minutes"`
	PurchasedAvailable        float64 `json:"purchased_available"`
	ReservedMinutes           float64 `json:"reserved_minutes"`
	PeriodStart               string  `json:"period_start"`
	PeriodEnd                 string  `json:"period_end"`
	ReviewHold                bool    `json:"review_hold"`
}

// CheckoutInput starts or replays one hosted checkout attempt.
type CheckoutInput struct {
	PackID            string `json:"pack_id"`
	ClientOperationID string `json:"client_operation_id"`
}

// Checkout is a transient hosted checkout link. Its URL is not payment proof.
type Checkout struct {
	OrderReference string `json:"order_reference"`
	CheckoutURL    string `json:"checkout_url"`
	PaymentMode    string `json:"payment_mode"`
}

// BillingPortal is a transient per-purchase billing-management link.
type BillingPortal struct {
	CustomerPortalLink string `json:"customer_portal_link"`
}

// Pack is a minute pack the workspace can buy.
type Pack struct {
	ID          string `json:"id"`
	Currency    string `json:"currency"`
	AmountCents int64  `json:"amount_cents"`
	Minutes     int64  `json:"minutes"`
}

// Billing is getCustomerBilling's result. Balance is nil exactly when no
// allowance is configured.
type Billing struct {
	AllowanceConfigured bool     `json:"allowance_configured"`
	CheckoutAvailable   bool     `json:"checkout_available"`
	PaymentMode         string   `json:"payment_mode"`
	LivePaymentsEnabled bool     `json:"live_payments_enabled"`
	Balance             *Balance `json:"balance"`
	Packs               []Pack   `json:"packs"`
}

// Billing calls getCustomerBilling.
func (c *Client) Billing(ctx context.Context, tenantID string) (*Billing, error) {
	var body struct {
		Billing *Billing `json:"billing"`
	}
	if err := c.do(ctx, http.MethodGet, tenantPath(tenantID, "/billing"), nil, nil, &body); err != nil {
		return nil, err
	}
	if err := required(body.Billing != nil && body.Billing.Packs != nil); err != nil {
		return nil, err
	}
	return body.Billing, nil
}

// Purchase is one minute-pack order.
type Purchase struct {
	OrderReference           string  `json:"order_reference"`
	Minutes                  int64   `json:"minutes"`
	Currency                 string  `json:"currency"`
	AmountCents              int64   `json:"amount_cents"`
	Status                   string  `json:"status"`
	CreatedAt                string  `json:"created_at"`
	CompletedAt              *string `json:"completed_at"`
	CanManageProviderBilling bool    `json:"can_manage_provider_billing"`
}

// Purchases is listCustomerBillingPurchases' result.
type Purchases struct {
	Purchases []Purchase `json:"purchases"`
	Limit     int64      `json:"limit"`
}

// Purchases calls listCustomerBillingPurchases (recent orders).
func (c *Client) Purchases(ctx context.Context, tenantID string) (*Purchases, error) {
	var body Purchases
	if err := c.do(ctx, http.MethodGet, tenantPath(tenantID, "/billing/purchases"), nil, nil, &body); err != nil {
		return nil, err
	}
	if err := required(body.Purchases != nil); err != nil {
		return nil, err
	}
	return &body, nil
}

// Checkout starts or replays a hosted checkout for one pack and operation ID.
func (c *Client) Checkout(ctx context.Context, tenantID string, input CheckoutInput) (*Checkout, error) {
	var body Checkout
	if err := c.do(ctx, http.MethodPost, tenantPath(tenantID, "/billing/checkout"), nil, input, &body); err != nil {
		return nil, err
	}
	if err := required(nonEmpty(body.OrderReference, body.CheckoutURL, body.PaymentMode)); err != nil {
		return nil, err
	}
	return &body, nil
}

// BillingPortal creates a transient hosted billing-management link for an order.
func (c *Client) BillingPortal(ctx context.Context, tenantID, orderReference string) (*BillingPortal, error) {
	var body BillingPortal
	path := tenantPath(tenantID, "/billing/purchases/"+url.PathEscape(orderReference)+"/portal")
	if err := c.do(ctx, http.MethodPost, path, nil, nil, &body); err != nil {
		return nil, err
	}
	if err := required(nonEmpty(body.CustomerPortalLink)); err != nil {
		return nil, err
	}
	return &body, nil
}
