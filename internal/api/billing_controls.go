package api

import (
	"context"
	"encoding/json"
	"net/http"
)

// AutoRecharge is the workspace's auto-recharge settings.
type AutoRecharge struct {
	Supported          bool    `json:"supported"`
	Enabled            bool    `json:"enabled"`
	ThresholdMinutes   int64   `json:"threshold_minutes"`
	PackID             string  `json:"pack_id"`
	PaymentMethodSaved bool    `json:"payment_method_saved"`
	InFlight           bool    `json:"in_flight"`
	LastFailureCode    string  `json:"last_failure_code,omitempty"`
	LastFailureAt      *string `json:"last_failure_at"`
}

// AutoRechargeInput is the body of PUT auto-recharge.
type AutoRechargeInput struct {
	Enabled          bool   `json:"enabled"`
	ThresholdMinutes int64  `json:"threshold_minutes"`
	PackID           string `json:"pack_id"`
}

// Invoices is listCustomerBillingInvoices' result. Each row keeps the
// server's own shape, so the CLI never guesses at fields it does not own.
type Invoices struct {
	Invoices []json.RawMessage `json:"invoices"`
	Limit    int64             `json:"limit"`
}

// CheckoutResult is createCustomerCheckout's result. The URL is navigation
// only, never payment evidence; refresh purchases to see the order.
type CheckoutResult struct {
	OrderReference string `json:"order_reference"`
	CheckoutURL    string `json:"checkout_url"`
	PaymentMode    string `json:"payment_mode"`
}

// Invoices calls listCustomerBillingInvoices (billing:read).
func (c *Client) Invoices(ctx context.Context, tenantID string) (*Invoices, error) {
	var body Invoices
	if err := c.do(ctx, http.MethodGet, tenantPath(tenantID, "/billing/invoices"), nil, nil, &body); err != nil {
		return nil, err
	}
	if err := required(body.Invoices != nil); err != nil {
		return nil, err
	}
	return &body, nil
}

// AutoRecharge calls getCustomerBillingAutoRecharge (billing:read).
func (c *Client) AutoRecharge(ctx context.Context, tenantID string) (*AutoRecharge, error) {
	var body struct {
		AutoRecharge *AutoRecharge `json:"auto_recharge"`
	}
	if err := c.do(ctx, http.MethodGet, tenantPath(tenantID, "/billing/auto-recharge"), nil, nil, &body); err != nil {
		return nil, err
	}
	if err := required(body.AutoRecharge != nil); err != nil {
		return nil, err
	}
	return body.AutoRecharge, nil
}

// SetAutoRecharge calls updateCustomerBillingAutoRecharge (billing:write).
func (c *Client) SetAutoRecharge(ctx context.Context, tenantID string, input AutoRechargeInput) (*AutoRecharge, error) {
	var body struct {
		AutoRecharge *AutoRecharge `json:"auto_recharge"`
	}
	if err := c.do(ctx, http.MethodPut, tenantPath(tenantID, "/billing/auto-recharge"), nil, input, &body); err != nil {
		return nil, err
	}
	if err := required(body.AutoRecharge != nil); err != nil {
		return nil, err
	}
	return body.AutoRecharge, nil
}

// BillingCountry calls the workspace billing country read (billing:read).
// An empty result means no country has been chosen.
func (c *Client) BillingCountry(ctx context.Context, tenantID string) (string, error) {
	var body struct {
		BillingCountry string `json:"billing_country"`
	}
	if err := c.do(ctx, http.MethodGet, tenantPath(tenantID, "/billing/country"), nil, nil, &body); err != nil {
		return "", err
	}
	return body.BillingCountry, nil
}

// ResetBillingCountry calls resetCustomerBillingCountry (billing:write): it
// returns an unused billing profile to unset, and the API refuses with 409
// billing_country_in_use once the workspace has a billing order. The result is
// the country afterwards, empty when unset.
func (c *Client) ResetBillingCountry(ctx context.Context, tenantID string) (string, error) {
	var body struct {
		BillingCountry *string `json:"billing_country"`
	}
	if err := c.do(ctx, http.MethodDelete, tenantPath(tenantID, "/billing/country"), nil, nil, &body); err != nil {
		return "", err
	}
	if err := required(body.BillingCountry != nil); err != nil {
		return "", err
	}
	return *body.BillingCountry, nil
}

// SetBillingCountry calls the workspace billing country write (billing:write).
func (c *Client) SetBillingCountry(ctx context.Context, tenantID, country string) (string, error) {
	var body struct {
		BillingCountry string `json:"billing_country"`
	}
	if err := c.do(ctx, http.MethodPut, tenantPath(tenantID, "/billing/country"), nil, map[string]string{"country": country}, &body); err != nil {
		return "", err
	}
	return body.BillingCountry, nil
}

// Marketplace calls the GitHub Marketplace plan read (billing:read). The
// plan view keeps the server's own shape.
func (c *Client) Marketplace(ctx context.Context, tenantID string) (map[string]any, error) {
	var body struct {
		Marketplace map[string]any `json:"marketplace"`
	}
	if err := c.do(ctx, http.MethodGet, tenantPath(tenantID, "/billing/marketplace"), nil, nil, &body); err != nil {
		return nil, err
	}
	if err := required(body.Marketplace != nil); err != nil {
		return nil, err
	}
	return body.Marketplace, nil
}

// Checkout calls createCustomerCheckout (all:all for keys, billing:write for
// people). operationID must be reused for any retry of the same purchase.
func (c *Client) Checkout(ctx context.Context, tenantID, packID, operationID string) (*CheckoutResult, error) {
	var body CheckoutResult
	request := map[string]string{"pack_id": packID, "client_operation_id": operationID}
	if err := c.do(ctx, http.MethodPost, tenantPath(tenantID, "/billing/checkout"), nil, request, &body); err != nil {
		return nil, err
	}
	if err := required(body.CheckoutURL != ""); err != nil {
		return nil, err
	}
	return &body, nil
}

// ProviderPortal calls the provider billing portal link (billing:write). The
// link is a bearer-like capability: callers must not store it.
func (c *Client) ProviderPortal(ctx context.Context, tenantID string) (string, error) {
	var body struct {
		BillingPortalURL string `json:"billing_portal_url"`
	}
	if err := c.do(ctx, http.MethodPost, tenantPath(tenantID, "/billing/portal"), nil, struct{}{}, &body); err != nil {
		return "", err
	}
	if err := required(body.BillingPortalURL != ""); err != nil {
		return "", err
	}
	return body.BillingPortalURL, nil
}
