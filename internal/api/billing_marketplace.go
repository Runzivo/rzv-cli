package api

import (
	"context"
	"net/http"
)

// MarketplaceClaimable is one held GitHub Marketplace purchase that the
// workspace can claim.
type MarketplaceClaimable struct {
	AccountID    int64  `json:"account_id"`
	AccountLogin string `json:"account_login"`
}

// MarketplaceClaim is claimCustomerMarketplaceHold's result. AlreadyLinked is
// true when the workspace had claimed the purchase before; nothing changed.
type MarketplaceClaim struct {
	AccountID     int64  `json:"account_id"`
	AccountLogin  string `json:"account_login"`
	AlreadyLinked bool   `json:"already_linked"`
}

// MarketplaceClaimable calls listCustomerMarketplaceClaimable (owner only, no
// API keys): the held purchases this workspace can claim, possibly none.
func (c *Client) MarketplaceClaimable(ctx context.Context, tenantID string) ([]MarketplaceClaimable, error) {
	var body struct {
		Claimable []MarketplaceClaimable `json:"claimable"`
	}
	if err := c.do(ctx, http.MethodGet, tenantPath(tenantID, "/billing/marketplace/claimable"), nil, nil, &body); err != nil {
		return nil, err
	}
	if err := required(body.Claimable != nil); err != nil {
		return nil, err
	}
	for _, item := range body.Claimable {
		if err := required(item.AccountID > 0); err != nil {
			return nil, err
		}
	}
	return body.Claimable, nil
}

// ClaimMarketplaceHold calls claimCustomerMarketplaceHold (owner only, needs a
// recent sign-in, no API keys): it links a held purchase to the workspace.
func (c *Client) ClaimMarketplaceHold(ctx context.Context, tenantID string, accountID int64) (*MarketplaceClaim, error) {
	var body struct {
		MarketplaceClaim *MarketplaceClaim `json:"marketplace_claim"`
	}
	request := map[string]int64{"account_id": accountID}
	if err := c.do(ctx, http.MethodPost, tenantPath(tenantID, "/billing/marketplace/claim"), nil, request, &body); err != nil {
		return nil, err
	}
	if err := required(body.MarketplaceClaim != nil && body.MarketplaceClaim.AccountID == accountID); err != nil {
		return nil, err
	}
	return body.MarketplaceClaim, nil
}
