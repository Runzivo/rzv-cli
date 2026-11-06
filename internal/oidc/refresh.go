package oidc

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrSessionEnded means the identity provider no longer accepts the refresh
// token: the session expired, was revoked or was signed out.
var ErrSessionEnded = authError("Your sign-in has ended. Run `rzv login` again.")

// RefreshOptions renews a stored sign-in.
type RefreshOptions struct {
	Issuer      string
	ClientID    string
	Audience    string
	BrokerClaim string
	// Subject must match the renewed access token's subject, so a refresh can
	// never switch the profile to another person.
	Subject               string
	RefreshToken          string
	AllowInsecureLoopback bool
	HTTPClient            *http.Client
	Now                   func() time.Time
}

// Refresh exchanges a refresh token for a new access token and, with the
// provider's rotation, a new refresh token. The new access token is verified
// exactly as at sign-in: signature, issuer, audience, client, type and broker.
func Refresh(ctx context.Context, opts RefreshOptions) (*Result, error) {
	issuerURL, err := normalizedURL(opts.Issuer, "Issuer", opts.AllowInsecureLoopback)
	if err != nil {
		return nil, err
	}
	issuer := strings.TrimSuffix(issuerURL.String(), "/")
	if opts.RefreshToken == "" {
		return nil, ErrSessionEnded
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: networkTimeout}
	}
	client = noRedirects(client)
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	meta, err := discover(ctx, client, issuer, opts.AllowInsecureLoopback)
	if err != nil {
		return nil, err
	}
	tokens, err := exchange(ctx, client, meta.TokenEndpoint, url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {opts.ClientID},
		"refresh_token": {opts.RefreshToken},
	}, "Identity provider refused to renew the sign-in. Run `rzv login` again.")
	if err != nil {
		return nil, err
	}
	set, err := fetchKeySet(ctx, client, meta.JWKSURI)
	if err != nil {
		return nil, unavailable("Identity provider signing keys are unavailable.")
	}
	check := tokenCheck{issuer: issuer, audience: opts.Audience, clientID: opts.ClientID, brokerClaim: opts.BrokerClaim, requireTyp: true, now: now(), allowedSkew: 30 * time.Second, kindForError: "access"}
	claims, err := check.verify(set, tokens.AccessToken)
	if err != nil {
		return nil, err
	}
	if opts.Subject != "" && claims.String("sub") != opts.Subject {
		return nil, authError("Identity provider renewed the sign-in for a different account. Run `rzv login` again.")
	}
	refresh := tokens.RefreshToken
	expires := tokens.refreshExpiry(now())
	if refresh == "" {
		// Without rotation the provider keeps the same refresh token valid.
		refresh = opts.RefreshToken
	}
	return &Result{Issuer: issuer, AccessToken: tokens.AccessToken, RefreshToken: refresh, RefreshExpiresAt: expires, Claims: claims}, nil
}

// Revoke asks the provider to revoke a refresh token (RFC 7009), which ends
// the CLI's session there. A provider without a revocation endpoint, or a
// token it already dropped, is not an error.
func Revoke(ctx context.Context, issuerValue, clientID, refreshToken string, allowInsecureLoopback bool, httpClient *http.Client) error {
	if refreshToken == "" {
		return nil
	}
	issuerURL, err := normalizedURL(issuerValue, "Issuer", allowInsecureLoopback)
	if err != nil {
		return err
	}
	issuer := strings.TrimSuffix(issuerURL.String(), "/")
	client := httpClient
	if client == nil {
		client = &http.Client{Timeout: networkTimeout}
	}
	client = noRedirects(client)
	meta, err := discover(ctx, client, issuer, allowInsecureLoopback)
	if err != nil {
		return err
	}
	if meta.RevocationEndpoint == "" {
		return nil
	}
	endpoint, err := normalizedURL(meta.RevocationEndpoint, "Identity provider endpoint", allowInsecureLoopback)
	if err != nil || endpoint.Scheme != issuerURL.Scheme || endpoint.Host != issuerURL.Host {
		return unavailable("Identity provider returned an endpoint outside its issuer origin.")
	}
	form := url.Values{"client_id": {clientID}, "token": {refreshToken}, "token_type_hint": {"refresh_token"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return unavailable("Identity provider revocation is unavailable.")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return unavailable("Identity provider revocation is unavailable.")
	}
	resp.Body.Close()
	// RFC 7009: the provider answers 200 for a revoked or unknown token. Any
	// other answer leaves the session's end unconfirmed.
	if resp.StatusCode != http.StatusOK {
		return unavailable("Identity provider did not confirm the revocation.")
	}
	return nil
}
