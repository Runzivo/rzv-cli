package cli

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/Runzivo/rzv-cli/internal/api"
)

const ticketsUsage = `Usage:
  rzv tickets list [--workspace <id> | --personal] [--json]
  rzv tickets open --kind <kind> --subject <subject> --body <body>
      [--workspace <id> | --personal] [--json]
  rzv tickets show <reference> [--workspace <id> | --personal] [--json]
  rzv tickets reply <reference> --body <body> [--workspace <id> | --personal] [--json]

Kinds: technical_help, credit_request, professional_services, signup_help.
Tickets use the selected workspace by default. Pass --personal for tickets
outside a workspace. The customer API does not provide close or reopen
operations for tickets.`

const maxTicketRequestBytes = 16 << 10

func init() {
	register("tickets", "List, open, show or reply to support tickets", runTickets)
	commandUsage["tickets"] = ticketsUsage
}

type ticketScope struct {
	workspaceID string
	personal    bool
}

func runTickets(a *app, cmd *command) error {
	if len(cmd.args) == 0 {
		return usageError("%s", ticketsUsage)
	}
	switch cmd.args[0] {
	case "list":
		return ticketsList(a, cmd)
	case "open":
		return ticketsOpen(a, cmd)
	case "show":
		return ticketsShow(a, cmd)
	case "reply":
		return ticketsReply(a, cmd)
	default:
		return usageError("%s", ticketsUsage)
	}
}

func ticketClient(a *app, cmd *command) (ticketScope, *api.Client, error) {
	if cmd.flags["personal"] {
		if workspace, err := workspaceOption(cmd); err != nil {
			return ticketScope{}, nil, err
		} else if workspace != "" {
			return ticketScope{}, nil, usageError("Use --personal or --workspace/--account, not both.")
		}
		client, _, err := a.humanClient()
		return ticketScope{personal: true}, client, err
	}
	workspaceID, client, err := a.workspaceClient(cmd)
	return ticketScope{workspaceID: workspaceID}, client, err
}

func ticketsList(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account", "personal"); err != nil {
		return err
	}
	if err := cmd.expectArgs(1, "rzv tickets list [--workspace <id> | --personal] [--json]"); err != nil {
		return err
	}
	scope, client, err := ticketClient(a, cmd)
	if err != nil {
		return err
	}
	tickets, err := client.Tickets(a.ctx, scope.workspaceID)
	if err != nil {
		return apiError(err, "")
	}
	a.output(cmd, ticketListOutput(scope, tickets), renderTickets(scope, tickets))
	return nil
}

func ticketsOpen(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account", "personal", "kind", "subject", "body"); err != nil {
		return err
	}
	if err := cmd.expectArgs(1, "rzv tickets open --kind <kind> --subject <subject> --body <body>"); err != nil {
		return err
	}
	kind, err := cmd.required("kind")
	if err != nil {
		return err
	}
	if !slices.Contains(api.TicketKinds, kind) {
		return usageError("--kind must be technical_help, credit_request, professional_services, or signup_help.")
	}
	subject, err := cmd.required("subject")
	if err != nil {
		return err
	}
	body, err := cmd.required("body")
	if err != nil {
		return err
	}
	input := api.CreateTicketInput{Kind: kind, Subject: subject, Body: body}
	if err := validateTicketInput(input); err != nil {
		return err
	}
	scope, client, err := ticketClient(a, cmd)
	if err != nil {
		return err
	}
	ticket, err := client.CreateTicket(a.ctx, scope.workspaceID, input)
	if err != nil {
		return apiError(err, "")
	}
	a.output(cmd, ticketOutput(scope, ticket), renderTicket(scope, ticket))
	return nil
}

func ticketsShow(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account", "personal"); err != nil {
		return err
	}
	if err := cmd.expectArgs(2, "rzv tickets show <reference> [--workspace <id> | --personal] [--json]"); err != nil {
		return err
	}
	reference, err := ticketReference(cmd.args[1])
	if err != nil {
		return err
	}
	scope, client, err := ticketClient(a, cmd)
	if err != nil {
		return err
	}
	ticket, err := client.Ticket(a.ctx, scope.workspaceID, reference)
	if err != nil {
		return apiError(err, "The ticket was not found in this scope.")
	}
	a.output(cmd, ticketOutput(scope, ticket), renderTicket(scope, ticket))
	return nil
}

func ticketsReply(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account", "personal", "body"); err != nil {
		return err
	}
	if err := cmd.expectArgs(2, "rzv tickets reply <reference> --body <body> [--workspace <id> | --personal] [--json]"); err != nil {
		return err
	}
	reference, err := ticketReference(cmd.args[1])
	if err != nil {
		return err
	}
	body, err := cmd.required("body")
	if err != nil {
		return err
	}
	if err := validateReply(body); err != nil {
		return err
	}
	scope, client, err := ticketClient(a, cmd)
	if err != nil {
		return err
	}
	message, err := client.ReplyTicket(a.ctx, scope.workspaceID, reference, body)
	if err != nil {
		return apiError(err, "The ticket was not found in this scope.")
	}
	a.output(cmd, ticketMessageOutput(scope, reference, message), renderTicketMessage(scope, reference, message))
	return nil
}

func validateTicketInput(input api.CreateTicketInput) error {
	if len(input.Subject) > 255 {
		return usageError("--subject must be at most 255 bytes.")
	}
	encoded, _ := json.Marshal(input)
	if len(encoded) > maxTicketRequestBytes {
		return usageError("--subject and --body together must fit in the 16 KiB ticket request limit.")
	}
	return nil
}

func validateReply(body string) error {
	encoded, _ := json.Marshal(map[string]string{"body": body})
	if len(encoded) > maxTicketRequestBytes {
		return usageError("--body must fit in the 16 KiB ticket reply request limit.")
	}
	return nil
}

func ticketReference(reference string) (string, error) {
	if strings.TrimSpace(reference) == "" {
		return "", usageError("<reference> must not be blank.")
	}
	return reference, nil
}

func ticketListOutput(scope ticketScope, tickets []api.Ticket) map[string]any {
	value := map[string]any{"tickets": tickets}
	if scope.personal {
		value["scope"] = "personal"
	} else {
		value["workspace_id"] = scope.workspaceID
	}
	return value
}

func ticketOutput(scope ticketScope, ticket *api.Ticket) map[string]any {
	value := map[string]any{"ticket": ticket}
	if scope.personal {
		value["scope"] = "personal"
	} else {
		value["workspace_id"] = scope.workspaceID
	}
	return value
}

func ticketMessageOutput(scope ticketScope, reference string, message *api.TicketMessage) map[string]any {
	value := map[string]any{"reference": reference, "message": message}
	if scope.personal {
		value["scope"] = "personal"
	} else {
		value["workspace_id"] = scope.workspaceID
	}
	return value
}

func ticketScopeLabel(scope ticketScope) string {
	if scope.personal {
		return "personal tickets"
	}
	return "workspace " + display(scope.workspaceID)
}

func renderTickets(scope ticketScope, tickets []api.Ticket) string {
	if len(tickets) == 0 {
		return "No tickets for " + ticketScopeLabel(scope) + "."
	}
	var table strings.Builder
	writer := tabwriter.NewWriter(&table, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "REFERENCE\tSTATUS\tKIND\tSUBJECT\tCREATED")
	for _, ticket := range tickets {
		_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n", display(ticket.Reference), display(ticket.Status), display(ticket.Kind), display(ticket.Subject), display(ticket.CreatedAt))
	}
	_ = writer.Flush()
	return "Tickets for " + ticketScopeLabel(scope) + ":\n" + strings.TrimSuffix(table.String(), "\n")
}

func renderTicket(scope ticketScope, ticket *api.Ticket) string {
	lines := []string{
		fmt.Sprintf("Ticket %s for %s:", display(ticket.Reference), ticketScopeLabel(scope)),
		"Kind: " + display(ticket.Kind),
		"Subject: " + display(ticket.Subject),
		"Status: " + display(ticket.Status),
		"Created: " + display(ticket.CreatedAt),
		fmt.Sprintf("Version: %d", ticket.Version),
		"Body: " + display(ticket.Body),
	}
	if len(ticket.Messages) > 0 {
		lines = append(lines, "Messages:")
		for _, message := range ticket.Messages {
			lines = append(lines, fmt.Sprintf("- %s at %s: %s", display(message.Kind), display(message.CreatedAt), display(message.Body)))
		}
	}
	return strings.Join(lines, "\n")
}

func renderTicketMessage(scope ticketScope, reference string, message *api.TicketMessage) string {
	return fmt.Sprintf("Added %s message to ticket %s for %s at %s:\n%s", display(message.Kind), display(reference), ticketScopeLabel(scope), display(message.CreatedAt), display(message.Body))
}
