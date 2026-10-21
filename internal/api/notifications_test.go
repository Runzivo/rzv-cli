package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

const notificationPageJSON = `{"notifications":[{"id":"12","category":"billing","kind":"billing.payment_failed","title":"Payment failed","body":"Update payment","params":{"amount":12},"link":"/billing","created_at":"2026-10-21T10:00:00Z","read":false}],"unread_count":2,"next_cursor":"next"}`
const notificationSettingsJSON = `{"settings":[{"category":"billing","in_app":true,"email":true,"email_locked":true},{"category":"jobs","in_app":false,"email":false,"email_locked":false},{"category":"account","in_app":true,"email":true,"email_locked":false},{"category":"support","in_app":true,"email":true,"email_locked":false},{"category":"system","in_app":true,"email":true,"email_locked":false}]}`

func TestNotificationsUseDocumentedRequestsAndResponses(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /v1/customer/tenants/t/1/notifications":
			if got := r.URL.RawQuery; got != "category=billing&cursor=next&limit=25&unread=false" {
				t.Errorf("query = %q", got)
			}
			_, _ = w.Write([]byte(notificationPageJSON))
		case "POST /v1/customer/tenants/t/1/notifications/read":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["all"] != false || len(body["ids"].([]any)) != 2 {
				t.Errorf("read body = %#v", body)
			}
			_, _ = w.Write([]byte(`{"unread_count":0}`))
		case "GET /v1/customer/tenants/t/1/notification-settings":
			_, _ = w.Write([]byte(notificationSettingsJSON))
		case "PUT /v1/customer/tenants/t/1/notification-settings":
			var body struct {
				Settings []map[string]any `json:"settings"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if len(body.Settings) != 1 || body.Settings[0]["category"] != "support" || body.Settings[0]["in_app"] != true || body.Settings[0]["email"] != false {
				t.Errorf("settings body = %#v", body)
			}
			if _, ok := body.Settings[0]["email_locked"]; ok {
				t.Errorf("request contains response-only email_locked: %#v", body)
			}
			_, _ = w.Write([]byte(notificationSettingsJSON))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	falseValue := false
	page, err := client.Notifications(context.Background(), "t/1", NotificationListOptions{Category: "billing", Unread: &falseValue, Limit: 25, Cursor: "next"})
	if err != nil || page.UnreadCount != 2 || page.NextCursor == nil || *page.NextCursor != "next" || page.Notifications[0].Params["amount"] != float64(12) {
		t.Fatalf("page = %#v, %v", page, err)
	}
	if unread, err := client.MarkNotificationsRead(context.Background(), "t/1", []string{"12", "13"}, false); err != nil || unread != 0 {
		t.Fatalf("mark read = %d, %v", unread, err)
	}
	if settings, err := client.NotificationSettings(context.Background(), "t/1"); err != nil || len(settings.Settings) != 5 || !settings.Settings[0].EmailLocked {
		t.Fatalf("settings = %#v, %v", settings, err)
	}
	if _, err := client.UpdateNotificationSettings(context.Background(), "t/1", []NotificationSetting{{Category: "support", InApp: true, Email: false}}); err != nil {
		t.Fatalf("update: %v", err)
	}
}

func TestNotificationsRejectRequiredNullFields(t *testing.T) {
	cases := []struct {
		name string
		body string
		call func(*Client) error
	}{
		{
			name: "notification body",
			body: `{"notifications":[{"id":"12","category":"billing","kind":"billing.payment_failed","title":"Payment failed","body":null,"link":null,"created_at":"2026-10-21T10:00:00Z","read":false}],"unread_count":0,"next_cursor":null}`,
			call: func(c *Client) error {
				_, err := c.Notifications(context.Background(), "t", NotificationListOptions{})
				return err
			},
		},
		{
			name: "notification read",
			body: `{"notifications":[{"id":"12","category":"billing","kind":"billing.payment_failed","title":"Payment failed","body":"Body","link":null,"created_at":"2026-10-21T10:00:00Z","read":null}],"unread_count":0,"next_cursor":null}`,
			call: func(c *Client) error {
				_, err := c.Notifications(context.Background(), "t", NotificationListOptions{})
				return err
			},
		},
		{
			name: "setting in app",
			body: `{"settings":[{"category":"billing","in_app":null,"email":true,"email_locked":true},{"category":"jobs","in_app":false,"email":false,"email_locked":false},{"category":"account","in_app":true,"email":true,"email_locked":false},{"category":"support","in_app":true,"email":true,"email_locked":false},{"category":"system","in_app":true,"email":true,"email_locked":false}]}`,
			call: func(c *Client) error { _, err := c.NotificationSettings(context.Background(), "t"); return err },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := newTestClient(t, respond(http.StatusOK, tc.body))
			if err := tc.call(client); err == nil {
				t.Fatal("required null accepted")
			}
		})
	}
}

func TestNotificationSettingsRequireAllUniqueCategories(t *testing.T) {
	for name, body := range map[string]string{
		"missing":   `{"settings":[{"category":"billing","in_app":true,"email":true,"email_locked":true}]}`,
		"duplicate": `{"settings":[{"category":"billing","in_app":true,"email":true,"email_locked":true},{"category":"billing","in_app":true,"email":true,"email_locked":false},{"category":"account","in_app":true,"email":true,"email_locked":false},{"category":"support","in_app":true,"email":true,"email_locked":false},{"category":"system","in_app":true,"email":true,"email_locked":false}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			client := newTestClient(t, respond(http.StatusOK, body))
			if _, err := client.NotificationSettings(context.Background(), "t"); err == nil {
				t.Fatal("incomplete settings accepted")
			}
		})
	}
}

func TestNotificationsRejectMalformedResponses(t *testing.T) {
	cases := []struct {
		name string
		call func(*Client) error
	}{
		{"page", func(c *Client) error {
			_, err := c.Notifications(context.Background(), "t", NotificationListOptions{})
			return err
		}},
		{"read", func(c *Client) error {
			_, err := c.MarkNotificationsRead(context.Background(), "t", nil, true)
			return err
		}},
		{"settings", func(c *Client) error { _, err := c.NotificationSettings(context.Background(), "t"); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := newTestClient(t, respond(http.StatusOK, `{}`))
			if err := tc.call(client); err == nil {
				t.Fatal("malformed response accepted")
			}
		})
	}
}

func TestNotificationsRequireUnreadCountButAcceptZero(t *testing.T) {
	for name, body := range map[string]string{
		"missing": `{"notifications":[],"next_cursor":null}`,
		"null":    `{"notifications":[],"unread_count":null,"next_cursor":null}`,
	} {
		t.Run(name, func(t *testing.T) {
			client := newTestClient(t, respond(http.StatusOK, body))
			if _, err := client.Notifications(context.Background(), "t", NotificationListOptions{}); err == nil {
				t.Fatal("missing unread_count accepted")
			}
		})
	}
	client := newTestClient(t, respond(http.StatusOK, `{"notifications":[],"unread_count":0,"next_cursor":null}`))
	page, err := client.Notifications(context.Background(), "t", NotificationListOptions{})
	if err != nil || page.UnreadCount != 0 {
		t.Fatalf("zero unread count = %#v, %v", page, err)
	}
}
