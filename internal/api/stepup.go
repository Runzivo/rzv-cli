package api

import (
	"context"
	"net/http"
	"net/url"
)

// Step-up categories (StepUpCategory): what the person is about to do.
const (
	StepUpBilling         = "billing"
	StepUpAPIKeys         = "api_keys"
	StepUpIntegrations    = "integrations"
	StepUpAccountSecurity = "account_security"
)

// StepUpAvailability is getCustomerStepUpEmailCode's answer. Reason is
// "no_verified_email" or empty; MaskedEmail is present when Available.
type StepUpAvailability struct {
	Available   bool   `json:"available"`
	CodeOnly    bool   `json:"code_only"`
	Reason      string `json:"reason,omitempty"`
	MaskedEmail string `json:"masked_email,omitempty"`
}

// StepUpCodeChallenge is requestCustomerStepUpEmailCode's answer.
type StepUpCodeChallenge struct {
	ChallengeID string `json:"challenge_id"`
	Category    string `json:"category"`
	MaskedEmail string `json:"masked_email"`
	ExpiresAt   string `json:"expires_at"`
	ResendAfter string `json:"resend_after"`
}

// StepUpVerified is verifyCustomerStepUpEmailCode's answer.
type StepUpVerified struct {
	Category        string `json:"category"`
	VerifiedAt      string `json:"verified_at"`
	StepUpExpiresAt string `json:"step_up_expires_at"`
}

const stepUpEmailCodePath = "/v1/customer/step-up/email-code"

// StepUpEmailCodeAvailability calls getCustomerStepUpEmailCode.
func (c *Client) StepUpEmailCodeAvailability(ctx context.Context, category string) (*StepUpAvailability, error) {
	var body struct {
		Available   *bool  `json:"available"`
		CodeOnly    *bool  `json:"code_only"`
		Reason      string `json:"reason"`
		MaskedEmail string `json:"masked_email"`
	}
	if err := c.do(ctx, http.MethodGet, stepUpEmailCodePath, url.Values{"category": {category}}, nil, &body); err != nil {
		return nil, err
	}
	if err := required(body.Available != nil && body.CodeOnly != nil); err != nil {
		return nil, err
	}
	return &StepUpAvailability{Available: *body.Available, CodeOnly: *body.CodeOnly, Reason: body.Reason, MaskedEmail: body.MaskedEmail}, nil
}

// RequestStepUpEmailCode calls requestCustomerStepUpEmailCode: the service
// emails a six-digit code to the caller's verified address.
func (c *Client) RequestStepUpEmailCode(ctx context.Context, category string) (*StepUpCodeChallenge, error) {
	var body StepUpCodeChallenge
	if err := c.do(ctx, http.MethodPost, stepUpEmailCodePath, nil, map[string]string{"category": category}, &body); err != nil {
		return nil, err
	}
	if err := required(nonEmpty(body.ChallengeID)); err != nil {
		return nil, err
	}
	return &body, nil
}

// VerifyStepUpEmailCode calls verifyCustomerStepUpEmailCode.
func (c *Client) VerifyStepUpEmailCode(ctx context.Context, challengeID, code string) (*StepUpVerified, error) {
	var body StepUpVerified
	if err := c.do(ctx, http.MethodPost, stepUpEmailCodePath+"/verify", nil, map[string]string{"challenge_id": challengeID, "code": code}, &body); err != nil {
		return nil, err
	}
	return &body, nil
}
