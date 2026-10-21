package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
)

const testTicketJSON = `{"reference":"sup_1","tenant_id":"t-1","kind":"technical_help","subject":"Help","body":"Details","status":"open","created_at":"2026-10-21T10:00:00Z","version":1}`
const testMessageJSON = `{"kind":"customer","body":"More details","created_at":"2026-10-21T10:01:00Z"}`

func TestTicketsPersonalAndWorkspaceWireContract(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-credential" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		switch r.Method + " " + r.URL.Path {
		case "GET /v1/customer/tickets":
			_, _ = w.Write([]byte(`{"tickets":[` + testTicketJSON + `]}`))
		case "POST /v1/customer/tickets":
			var input CreateTicketInput
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input != (CreateTicketInput{Kind: "technical_help", Subject: "Help", Body: "Details"}) {
				t.Errorf("personal input = %+v, %v", input, err)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"ticket":` + testTicketJSON + `}`))
		case "GET /v1/customer/ticket":
			if got := r.URL.Query().Get("reference"); got != "sup /?" {
				t.Errorf("reference = %q", got)
			}
			_, _ = w.Write([]byte(`{"ticket":{"reference":"sup_1","tenant_id":"t-1","kind":"technical_help","subject":"Help","body":"Details","status":"open","created_at":"2026-10-21T10:00:00Z","version":1,"messages":[` + testMessageJSON + `]}}`))
		case "POST /v1/customer/ticket/messages":
			if got := r.URL.Query().Get("reference"); got != "sup /?" {
				t.Errorf("reference = %q", got)
			}
			body, _ := io.ReadAll(r.Body)
			if string(body) != `{"body":"More details"}` {
				t.Errorf("reply body = %s", body)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"message":` + testMessageJSON + `}`))
		case "GET /v1/customer/tenants/t-1/tickets":
			_, _ = w.Write([]byte(`{"tickets":[]}`))
		case "POST /v1/customer/tenants/t-1/tickets":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"ticket":` + testTicketJSON + `}`))
		case "GET /v1/customer/tenants/t-1/ticket":
			if got := r.URL.Query().Get("reference"); got != "sup-1" {
				t.Errorf("workspace reference = %q", got)
			}
			_, _ = w.Write([]byte(`{"ticket":` + testTicketJSON + `}`))
		case "POST /v1/customer/tenants/t-1/ticket/messages":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"message":` + testMessageJSON + `}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()
	if tickets, err := client.Tickets(ctx, ""); err != nil || len(tickets) != 1 || tickets[0].Reference != "sup_1" {
		t.Fatalf("personal list = %+v, %v", tickets, err)
	}
	input := CreateTicketInput{Kind: "technical_help", Subject: "Help", Body: "Details"}
	if ticket, err := client.CreateTicket(ctx, "", input); err != nil || ticket.Reference != "sup_1" {
		t.Fatalf("personal create = %+v, %v", ticket, err)
	}
	if ticket, err := client.Ticket(ctx, "", "sup /?"); err != nil || len(ticket.Messages) != 1 {
		t.Fatalf("personal show = %+v, %v", ticket, err)
	}
	if message, err := client.ReplyTicket(ctx, "", "sup /?", "More details"); err != nil || message.Kind != "customer" {
		t.Fatalf("personal reply = %+v, %v", message, err)
	}
	if tickets, err := client.Tickets(ctx, "t-1"); err != nil || len(tickets) != 0 {
		t.Fatalf("workspace list = %+v, %v", tickets, err)
	}
	if _, err := client.CreateTicket(ctx, "t-1", input); err != nil {
		t.Fatalf("workspace create: %v", err)
	}
	if _, err := client.Ticket(ctx, "t-1", "sup-1"); err != nil {
		t.Fatalf("workspace show: %v", err)
	}
	if _, err := client.ReplyTicket(ctx, "t-1", "sup-1", "More details"); err != nil {
		t.Fatalf("workspace reply: %v", err)
	}
}

func TestTicketsRejectMalformedResponses(t *testing.T) {
	ctx := context.Background()
	for name, call := range map[string]func(*Client) error{
		"list":   func(c *Client) error { _, err := c.Tickets(ctx, ""); return err },
		"create": func(c *Client) error { _, err := c.CreateTicket(ctx, "", CreateTicketInput{}); return err },
		"show":   func(c *Client) error { _, err := c.Ticket(ctx, "", "sup-1"); return err },
		"reply":  func(c *Client) error { _, err := c.ReplyTicket(ctx, "", "sup-1", "body"); return err },
	} {
		t.Run(name, func(t *testing.T) {
			client := newTestClient(t, respond(http.StatusOK, `{}`))
			var apiErr *Error
			if err := call(client); !errors.As(err, &apiErr) || apiErr.Code != "malformed_response" {
				t.Fatalf("error = %v", err)
			}
		})
	}
}
