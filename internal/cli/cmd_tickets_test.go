package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

const cliTicketJSON = `{"reference":"sup_1","tenant_id":"t-1","kind":"technical_help","subject":"Login\u001b[31m","body":"Details","status":"open","created_at":"2026-10-21T10:00:00Z","version":1}`
const cliTicketWithMessageJSON = `{"reference":"sup_1","tenant_id":"t-1","kind":"technical_help","subject":"Login","body":"Details","status":"open","created_at":"2026-10-21T10:00:00Z","version":1,"messages":[{"kind":"staff_public","body":"Reply","created_at":"2026-10-21T10:01:00Z"}]}`

func TestTicketsWorkspaceCommands(t *testing.T) {
	h := selectedHarness(t)
	h.json("GET /v1/customer/tenants/t-1/tickets", 200, `{"tickets":[`+cliTicketJSON+`]}`)
	if code, out, _ := h.run("tickets", "list"); code != 0 || !strings.Contains(out, "Tickets for workspace t-1") || !strings.Contains(out, "REFERENCE") || strings.Contains(out, "\x1b") {
		t.Fatalf("list: %d %q", code, out)
	}
	if code, out, _ := h.run("tickets", "list", "--workspace", "t-1", "--json"); code != 0 || !strings.Contains(out, `"workspace_id":"t-1"`) {
		t.Fatalf("workspace override: %d %q", code, out)
	}
	if code, out, _ := h.run("tickets", "list", "--account", "t-1", "--json"); code != 0 || !strings.Contains(out, `"workspace_id":"t-1"`) {
		t.Fatalf("account override: %d %q", code, out)
	}
	var openBody map[string]string
	h.routes["POST /v1/customer/tenants/t-1/tickets"] = func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&openBody)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ticket":` + cliTicketJSON + `}`))
	}
	if code, out, _ := h.run("tickets", "open", "--kind", "technical_help", "--subject", "Login", "--body", "Details"); code != 0 || !strings.Contains(out, "Ticket sup_1") || len(openBody) != 3 {
		t.Fatalf("open: %d %q %v", code, out, openBody)
	}
	h.routes["GET /v1/customer/tenants/t-1/ticket"] = func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("reference") != "sup_1" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"ticket":` + cliTicketWithMessageJSON + `}`))
	}
	if code, out, _ := h.run("tickets", "show", "sup_1"); code != 0 || !strings.Contains(out, "Messages:") || !strings.Contains(out, "staff_public") {
		t.Fatalf("show: %d %q", code, out)
	}
	var replyBody map[string]string
	h.routes["POST /v1/customer/tenants/t-1/ticket/messages"] = func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("reference") != "sup_1" {
			t.Errorf("reference = %q", r.URL.Query().Get("reference"))
		}
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &replyBody)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"message":{"kind":"customer","body":"More","created_at":"2026-10-21T10:02:00Z"}}`))
	}
	code, out, _ := h.run("tickets", "reply", "sup_1", "--body", "More", "--json")
	if code != 0 || replyBody["body"] != "More" || !strings.Contains(out, `"workspace_id":"t-1"`) {
		t.Fatalf("reply: %d %q %v", code, out, replyBody)
	}
	h.json("GET /v1/customer/tenants/t-1/ticket", 404, `{"error":{"code":"not_found"}}`)
	if code, _, errOut := h.run("tickets", "show", "missing"); code != exitNotFound || !strings.Contains(errOut, "not found") {
		t.Fatalf("missing: %d %q", code, errOut)
	}
}

func TestTicketsPersonalCommandsAndScopeValidation(t *testing.T) {
	h := selectedHarness(t)
	h.json("GET /v1/customer/tickets", 200, `{"tickets":[]}`)
	code, out, _ := h.run("tickets", "list", "--personal", "--json")
	if code != 0 || !strings.Contains(out, `"scope":"personal"`) {
		t.Fatalf("personal list: %d %q", code, out)
	}
	h.routes["POST /v1/customer/tickets"] = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ticket":` + cliTicketJSON + `}`))
	}
	if code, _, _ := h.run("tickets", "open", "--personal", "--kind", "signup_help", "--subject", "Signup", "--body", "Help"); code != 0 {
		t.Fatalf("personal open: %d", code)
	}
	h.json("GET /v1/customer/ticket", 200, `{"ticket":`+cliTicketWithMessageJSON+`}`)
	if code, out, _ := h.run("tickets", "show", "sup_1", "--personal"); code != 0 || !strings.Contains(out, "personal tickets") {
		t.Fatalf("personal show: %d %q", code, out)
	}
	h.routes["POST /v1/customer/ticket/messages"] = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"message":{"kind":"customer","body":"More","created_at":"2026-10-21T10:02:00Z"}}`))
	}
	if code, out, _ := h.run("tickets", "reply", "sup_1", "--personal"); code != exitUsage || out != "" {
		t.Fatalf("personal reply missing body: %d %q", code, out)
	}
	if code, out, _ := h.run("tickets", "reply", "sup_1", "--personal", "--body", "More"); code != 0 || !strings.Contains(out, "personal tickets") {
		t.Fatalf("personal reply: %d %q", code, out)
	}
	h.json("GET /v1/customer/ticket", 403, `{"error":{"code":"tenant_membership_required"}}`)
	if code, _, _ := h.run("tickets", "show", "sup_1", "--personal"); code != exitAccessDenied {
		t.Fatalf("personal 403: %d", code)
	}
	if code, _, _ := h.run("tickets", "list", "--personal", "--workspace", "t-1"); code != exitUsage {
		t.Fatalf("mixed scope: %d", code)
	}
	h.env["RZV_API_KEY"] = "rzv_test_key"
	if code, _, _ := h.run("tickets", "list", "--personal"); code != exitAccessDenied {
		t.Fatalf("personal key: %d", code)
	}
}

func TestTicketsAPIErrorsAndWorkspaceAPIKey(t *testing.T) {
	h := selectedHarness(t)
	h.json("GET /v1/customer/tenants/t-1/tickets", 401, `{"error":{"code":"unauthorized"}}`)
	if code, _, _ := h.run("tickets", "list"); code != exitAuth {
		t.Fatalf("401: %d", code)
	}
	h.json("GET /v1/customer/tenants/t-1/tickets", 200, `{}`)
	if code, _, _ := h.run("tickets", "list"); code != exitFailure {
		t.Fatalf("malformed: %d", code)
	}

	keyHarness := newHarness(t)
	keyHarness.env["RZV_API_KEY"] = "rzv_test_key"
	keyHarness.env["RZV_API_URL"] = keyHarness.api.URL
	keyHarness.json("GET /v1/customer/tenants/t-5/tickets", 200, `{"tickets":[]}`)
	code, out, _ := keyHarness.run("tickets", "list", "--workspace", "t-5", "--json")
	if code != 0 || !strings.Contains(out, `"workspace_id":"t-5"`) || keyHarness.seenAuth[len(keyHarness.seenAuth)-1] != "Bearer rzv_test_key" {
		t.Fatalf("workspace API key: %d %q %q", code, out, keyHarness.seenAuth)
	}
}

func TestTicketsInputValidation(t *testing.T) {
	h := newHarness(t)
	for _, args := range [][]string{
		{"tickets", "open", "--kind", "bad", "--subject", "x", "--body", "x"},
		{"tickets", "open", "--kind", "technical_help", "--subject", strings.Repeat("x", 256), "--body", "x"},
		{"tickets", "open", "--kind", "technical_help", "--subject", strings.Repeat("é", 128), "--body", "x"},
		{"tickets", "reply", "sup_1", "--body", strings.Repeat("x", maxTicketRequestBytes)},
		{"tickets", "show", " "},
	} {
		if code, _, _ := h.run(args...); code != exitUsage {
			t.Errorf("%q exit = %d", args, code)
		}
	}
	if cmd, err := parse([]string{"tickets", "list", "--personal"}); err != nil || !cmd.flags["personal"] {
		t.Fatalf("personal parse = %+v, %v", cmd, err)
	}
}
