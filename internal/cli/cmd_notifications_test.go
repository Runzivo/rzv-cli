package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const cliNotificationPageJSON = `{"notifications":[{"id":"12","category":"billing","kind":"billing.payment_failed","title":"Payment\u001b[31m failed","body":"Update\u001b payment","params":{"amount":12},"link":"/billing\u001b","created_at":"2026-10-21T10:00:00Z","read":false}],"unread_count":2,"next_cursor":"next"}`
const cliNotificationSettingsJSON = `{"settings":[{"category":"billing","in_app":true,"email":true,"email_locked":true},{"category":"jobs","in_app":false,"email":false,"email_locked":false},{"category":"account","in_app":true,"email":true,"email_locked":false},{"category":"support","in_app":true,"email":true,"email_locked":false},{"category":"system","in_app":true,"email":true,"email_locked":false}]}`

func TestNotificationCommands(t *testing.T) {
	h := selectedHarness(t)
	listCalls := 0
	h.routes["GET /v1/customer/tenants/t-1/notifications"] = func(w http.ResponseWriter, r *http.Request) {
		listCalls++
		if listCalls == 1 && r.URL.RawQuery != "category=billing&cursor=next&limit=25&unread=true" {
			got := r.URL.RawQuery
			t.Errorf("query = %q", got)
		}
		_, _ = w.Write([]byte(cliNotificationPageJSON))
	}
	code, out, _ := h.run("notifications", "list", "--category", "billing", "--unread", "true", "--limit", "25", "--cursor", "next")
	if code != 0 || !strings.Contains(out, "unread: 2") || !strings.Contains(out, "BODY") || !strings.Contains(out, `Update\u001b payment`) || !strings.Contains(out, `/billing\u001b`) || !strings.Contains(out, "Next cursor: next") || strings.Contains(out, "\x1b") {
		t.Fatalf("list: %d %q", code, out)
	}
	if code, out, _ = h.run("notifications", "list", "--json"); code != 0 || !strings.Contains(out, `"unread_count":2`) || !strings.Contains(out, `"next_cursor":"next"`) || !strings.Contains(out, `"params":{"amount":12}`) {
		t.Fatalf("list json: %d %q", code, out)
	}
	h.routes["POST /v1/customer/tenants/t-1/notifications/read"] = func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			IDs []string `json:"ids"`
			All bool     `json:"all"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if len(body.IDs) != 2 || body.All {
			t.Errorf("read body = %#v", body)
		}
		_, _ = w.Write([]byte(`{"unread_count":0}`))
	}
	if code, out, _ = h.run("notifications", "read", "12", "13", "--json"); code != 0 || !strings.Contains(out, `"unread_count":0`) {
		t.Fatalf("read: %d %q", code, out)
	}
	h.routes["POST /v1/customer/tenants/t-1/notifications/read"] = func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["all"] != true || len(body["ids"].([]any)) != 0 {
			t.Errorf("all body = %#v", body)
		}
		_, _ = w.Write([]byte(`{"unread_count":0}`))
	}
	if code, out, _ = h.run("notifications", "read", "--all"); code != 0 || !strings.Contains(out, "Unread notifications") {
		t.Fatalf("read all: %d %q", code, out)
	}
	h.json("GET /v1/customer/tenants/t-1/notification-settings", 200, cliNotificationSettingsJSON)
	if code, out, _ = h.run("notifications", "settings", "show"); code != 0 || !strings.Contains(out, "EMAIL LOCKED") || !strings.Contains(out, "billing") {
		t.Fatalf("settings show: %d %q", code, out)
	}
	h.routes["PUT /v1/customer/tenants/t-1/notification-settings"] = func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Settings []map[string]any `json:"settings"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if len(body.Settings) != 1 || body.Settings[0]["category"] != "billing" || body.Settings[0]["in_app"] != false || body.Settings[0]["email"] != false {
			t.Errorf("update body = %#v", body)
		}
		_, _ = w.Write([]byte(cliNotificationSettingsJSON))
	}
	if code, out, _ = h.run("notifications", "settings", "update", "--category", "billing", "--in-app", "false", "--email", "false", "--json"); code != 0 || !strings.Contains(out, `"email_locked":true`) {
		t.Fatalf("settings update: %d %q", code, out)
	}
}

func TestNotificationEmptyAndErrors(t *testing.T) {
	h := selectedHarness(t)
	h.json("GET /v1/customer/tenants/t-1/notifications", 200, `{"notifications":[],"unread_count":0,"next_cursor":null}`)
	if code, out, _ := h.run("notifications", "list"); code != 0 || !strings.Contains(out, "No notifications") {
		t.Fatalf("empty: %d %q", code, out)
	}
	h.json("GET /v1/customer/tenants/t-1/notifications", 403, `{"error":{"code":"tenant_membership_required"}}`)
	if code, _, _ := h.run("notifications", "list"); code != exitAccessDenied {
		t.Fatalf("403: %d", code)
	}
	h.json("GET /v1/customer/tenants/t-1/notifications", 404, `{"error":{"code":"not_found"}}`)
	if code, _, _ := h.run("notifications", "list"); code != exitNotFound {
		t.Fatalf("404: %d", code)
	}
	h.json("GET /v1/customer/tenants/t-1/notifications", 200, `{}`)
	if code, _, _ := h.run("notifications", "list"); code != exitFailure {
		t.Fatalf("malformed: %d", code)
	}
}

func TestNotificationValidationAndHumanOnly(t *testing.T) {
	h := newHarness(t)
	for _, args := range [][]string{
		{"notifications", "list", "--category", "bad"},
		{"notifications", "list", "--unread", "yes"},
		{"notifications", "list", "--limit", "0"},
		{"notifications", "list", "--cursor", strings.Repeat("x", 513)},
		{"notifications", "read"},
		{"notifications", "read", "x"},
		{"notifications", "read", "12", "--all"},
		{"notifications", "settings", "update", "--category", "billing", "--in-app", "true"},
		{"notifications", "settings", "update", "--category", "billing", "--in-app", "one", "--email", "true"},
	} {
		calls := len(h.seenAuth)
		if code, _, _ := h.run(args...); code != exitUsage || len(h.seenAuth) != calls {
			t.Errorf("%q exit = %d, calls = %d want %d", args, code, len(h.seenAuth), calls)
		}
	}
	h.env["RZV_API_KEY"] = "rzv_test_key"
	calls := len(h.seenAuth)
	if code, _, _ := h.run("notifications", "list"); code != exitAccessDenied || len(h.seenAuth) != calls {
		t.Fatalf("API key list: %d calls %d want %d", code, len(h.seenAuth), calls)
	}
	if code, _, _ := h.run("notifications", "settings", "show"); code != exitAccessDenied || len(h.seenAuth) != calls {
		t.Fatalf("API key settings: %d calls %d want %d", code, len(h.seenAuth), calls)
	}
}
