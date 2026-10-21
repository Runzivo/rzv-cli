package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
)

// TicketKinds are the support ticket categories accepted by the customer API.
var TicketKinds = []string{"technical_help", "credit_request", "professional_services", "signup_help"}

// Ticket is a support request visible to the authenticated customer.
type Ticket struct {
	Reference string          `json:"reference"`
	TenantID  *string         `json:"tenant_id,omitempty"`
	Kind      string          `json:"kind"`
	Subject   string          `json:"subject"`
	Body      string          `json:"body"`
	Status    string          `json:"status"`
	CreatedAt string          `json:"created_at"`
	Version   int64           `json:"version"`
	Messages  []TicketMessage `json:"messages,omitempty"`

	versionPresent bool
}

func (t *Ticket) valid() bool {
	return t.versionPresent && nonEmpty(t.Reference, t.Kind, t.Subject, t.Body, t.Status, t.CreatedAt) &&
		slices.Contains(TicketKinds, t.Kind) && (t.Status == "open" || t.Status == "resolved")
}

func (t *Ticket) UnmarshalJSON(data []byte) error {
	var raw struct {
		Reference string          `json:"reference"`
		TenantID  *string         `json:"tenant_id"`
		Kind      string          `json:"kind"`
		Subject   string          `json:"subject"`
		Body      string          `json:"body"`
		Status    string          `json:"status"`
		CreatedAt string          `json:"created_at"`
		Version   *int64          `json:"version"`
		Messages  []TicketMessage `json:"messages"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	t.Reference = raw.Reference
	t.TenantID = raw.TenantID
	t.Kind = raw.Kind
	t.Subject = raw.Subject
	t.Body = raw.Body
	t.Status = raw.Status
	t.CreatedAt = raw.CreatedAt
	t.Messages = raw.Messages
	t.versionPresent = raw.Version != nil
	if raw.Version != nil {
		t.Version = *raw.Version
	}
	return nil
}

// TicketMessage is a public conversation message. Customer tickets never
// expose staff identity or private staff messages.
type TicketMessage struct {
	Kind      string `json:"kind"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

func (m *TicketMessage) valid() bool {
	return nonEmpty(m.Kind, m.Body, m.CreatedAt) && (m.Kind == "customer" || m.Kind == "staff_public")
}

// CreateTicketInput is the request body for opening a ticket. Scope and
// requester identity are derived from the authenticated route, never sent by
// the client.
type CreateTicketInput struct {
	Kind    string `json:"kind"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func ticketPath(tenantID, suffix string) string {
	if tenantID == "" {
		return "/v1/customer" + suffix
	}
	return tenantPath(tenantID, suffix)
}

// Tickets lists personal tickets when tenantID is empty, otherwise tickets
// visible in the workspace.
func (c *Client) Tickets(ctx context.Context, tenantID string) ([]Ticket, error) {
	var body struct {
		Tickets *[]Ticket `json:"tickets"`
	}
	if err := c.do(ctx, http.MethodGet, ticketPath(tenantID, "/tickets"), nil, nil, &body); err != nil {
		return nil, err
	}
	if err := required(body.Tickets != nil); err != nil {
		return nil, err
	}
	for i := range *body.Tickets {
		if err := required((*body.Tickets)[i].valid()); err != nil {
			return nil, err
		}
	}
	return *body.Tickets, nil
}

// CreateTicket opens a personal or workspace ticket according to tenantID.
func (c *Client) CreateTicket(ctx context.Context, tenantID string, input CreateTicketInput) (*Ticket, error) {
	var body struct {
		Ticket *Ticket `json:"ticket"`
	}
	if err := c.do(ctx, http.MethodPost, ticketPath(tenantID, "/tickets"), nil, input, &body); err != nil {
		return nil, err
	}
	if err := required(body.Ticket != nil && body.Ticket.valid()); err != nil {
		return nil, err
	}
	for i := range body.Ticket.Messages {
		if err := required(body.Ticket.Messages[i].valid()); err != nil {
			return nil, err
		}
	}
	return body.Ticket, nil
}

// Ticket reads one personal or workspace ticket. reference is always a query
// value, because it is opaque and is not a path segment.
func (c *Client) Ticket(ctx context.Context, tenantID, reference string) (*Ticket, error) {
	var body struct {
		Ticket *Ticket `json:"ticket"`
	}
	if err := c.do(ctx, http.MethodGet, ticketPath(tenantID, "/ticket"), url.Values{"reference": {reference}}, nil, &body); err != nil {
		return nil, err
	}
	if err := required(body.Ticket != nil && body.Ticket.valid()); err != nil {
		return nil, err
	}
	for i := range body.Ticket.Messages {
		if err := required(body.Ticket.Messages[i].valid()); err != nil {
			return nil, err
		}
	}
	return body.Ticket, nil
}

// ReplyTicket appends a customer message to one personal or workspace ticket.
func (c *Client) ReplyTicket(ctx context.Context, tenantID, reference, reply string) (*TicketMessage, error) {
	var body struct {
		Message *TicketMessage `json:"message"`
	}
	if err := c.do(ctx, http.MethodPost, ticketPath(tenantID, "/ticket/messages"), url.Values{"reference": {reference}}, map[string]string{"body": reply}, &body); err != nil {
		return nil, err
	}
	if err := required(body.Message != nil && body.Message.valid()); err != nil {
		return nil, err
	}
	return body.Message, nil
}
