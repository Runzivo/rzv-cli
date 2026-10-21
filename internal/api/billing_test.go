package api

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestBillingLinksRejectMissingRequiredFields(t *testing.T) {
	cases := []struct {
		name string
		call func(*Client) error
		body string
	}{
		{
			name: "checkout missing order reference",
			call: func(c *Client) error {
				_, err := c.Checkout(context.Background(), "t", CheckoutInput{PackID: "pack", ClientOperationID: "operation"})
				return err
			},
			body: `{"checkout_url":"https://checkout.example.test/path","payment_mode":"test"}`,
		},
		{
			name: "checkout missing URL",
			call: func(c *Client) error {
				_, err := c.Checkout(context.Background(), "t", CheckoutInput{PackID: "pack", ClientOperationID: "operation"})
				return err
			},
			body: `{"order_reference":"order","payment_mode":"test"}`,
		},
		{
			name: "checkout missing payment mode",
			call: func(c *Client) error {
				_, err := c.Checkout(context.Background(), "t", CheckoutInput{PackID: "pack", ClientOperationID: "operation"})
				return err
			},
			body: `{"order_reference":"order","checkout_url":"https://checkout.example.test/path"}`,
		},
		{
			name: "portal missing link",
			call: func(c *Client) error {
				_, err := c.BillingPortal(context.Background(), "t", "order")
				return err
			},
			body: `{}`,
		},
		{
			name: "portal empty link",
			call: func(c *Client) error {
				_, err := c.BillingPortal(context.Background(), "t", "order")
				return err
			},
			body: `{"customer_portal_link":" "}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			})
			var apiErr *Error
			if err := tc.call(client); !errors.As(err, &apiErr) || apiErr.Status != http.StatusOK || apiErr.Code != "malformed_response" {
				t.Fatalf("got %v, want malformed response error", err)
			}
		})
	}
}
