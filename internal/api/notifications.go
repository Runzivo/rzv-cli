package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strconv"
)

// NotificationCategories are the customer notification categories accepted by
// the customer API.
var NotificationCategories = []string{"billing", "jobs", "account", "support", "system"}

// Notification is one in-app notice for the signed-in customer.
type Notification struct {
	ID        string         `json:"id"`
	Category  string         `json:"category"`
	Kind      string         `json:"kind"`
	Title     string         `json:"title"`
	Body      string         `json:"body"`
	Params    map[string]any `json:"params"`
	Link      *string        `json:"link"`
	CreatedAt string         `json:"created_at"`
	Read      bool           `json:"read"`
}

func (n *Notification) valid() bool {
	return nonEmpty(n.ID, n.Category, n.Kind, n.Title, n.CreatedAt) && slices.Contains(NotificationCategories, n.Category)
}

func (n *Notification) UnmarshalJSON(data []byte) error {
	type notification Notification
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if err := json.Unmarshal(data, (*notification)(n)); err != nil {
		return err
	}
	for _, key := range []string{"id", "category", "kind", "title", "body", "created_at", "read"} {
		if value, ok := raw[key]; !ok || string(value) == "null" {
			return &Error{Status: http.StatusOK, Code: "malformed_response"}
		}
	}
	if _, ok := raw["link"]; !ok {
		return &Error{Status: http.StatusOK, Code: "malformed_response"}
	}
	if value, ok := raw["params"]; ok && string(value) == "null" {
		return &Error{Status: http.StatusOK, Code: "malformed_response"}
	}
	return nil
}

// NotificationPage is one page of notifications and the current unread count.
type NotificationPage struct {
	Notifications []Notification `json:"notifications"`
	UnreadCount   int64          `json:"unread_count"`
	NextCursor    *string        `json:"next_cursor"`

	nextCursorPresent  bool
	unreadCountPresent bool
}

func (p *NotificationPage) UnmarshalJSON(data []byte) error {
	type page NotificationPage
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if err := json.Unmarshal(data, (*page)(p)); err != nil {
		return err
	}
	if _, ok := raw["next_cursor"]; ok {
		p.nextCursorPresent = true
	}
	if value, ok := raw["unread_count"]; ok && string(value) != "null" {
		p.unreadCountPresent = true
	}
	return nil
}

// NotificationListOptions narrows a notification page request.
type NotificationListOptions struct {
	Category string
	Unread   *bool
	Limit    int
	Cursor   string
}

// Notifications lists the signed-in customer's notifications for a workspace.
func (c *Client) Notifications(ctx context.Context, tenantID string, options NotificationListOptions) (*NotificationPage, error) {
	query := url.Values{}
	if options.Category != "" {
		query.Set("category", options.Category)
	}
	if options.Unread != nil {
		if *options.Unread {
			query.Set("unread", "true")
		} else {
			query.Set("unread", "false")
		}
	}
	if options.Limit != 0 {
		query.Set("limit", strconv.Itoa(options.Limit))
	}
	if options.Cursor != "" {
		query.Set("cursor", options.Cursor)
	}
	var page NotificationPage
	if err := c.do(ctx, http.MethodGet, tenantPath(tenantID, "/notifications"), query, nil, &page); err != nil {
		return nil, err
	}
	if err := required(page.Notifications != nil && page.UnreadCount >= 0 && page.nextCursorPresent && page.unreadCountPresent); err != nil {
		return nil, err
	}
	for i := range page.Notifications {
		if err := required(page.Notifications[i].valid()); err != nil {
			return nil, err
		}
	}
	return &page, nil
}

// MarkNotificationsRead marks specific IDs, or every notification when all is true.
func (c *Client) MarkNotificationsRead(ctx context.Context, tenantID string, ids []string, all bool) (int64, error) {
	var body struct {
		UnreadCount *int64 `json:"unread_count"`
	}
	if err := c.do(ctx, http.MethodPost, tenantPath(tenantID, "/notifications/read"), nil, map[string]any{"ids": ids, "all": all}, &body); err != nil {
		return 0, err
	}
	if err := required(body.UnreadCount != nil && *body.UnreadCount >= 0); err != nil {
		return 0, err
	}
	return *body.UnreadCount, nil
}

// NotificationSetting is one category's in-app and email settings.
type NotificationSetting struct {
	Category    string `json:"category"`
	InApp       bool   `json:"in_app"`
	Email       bool   `json:"email"`
	EmailLocked bool   `json:"email_locked"`
}

func (s *NotificationSetting) UnmarshalJSON(data []byte) error {
	type setting NotificationSetting
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if err := json.Unmarshal(data, (*setting)(s)); err != nil {
		return err
	}
	for _, key := range []string{"category", "in_app", "email", "email_locked"} {
		if value, ok := raw[key]; !ok || string(value) == "null" {
			return &Error{Status: http.StatusOK, Code: "malformed_response"}
		}
	}
	return nil
}

// NotificationSettingsResponse is the API response for notification settings.
type NotificationSettingsResponse struct {
	Settings []NotificationSetting `json:"settings"`
}

func (r *NotificationSettingsResponse) valid() bool {
	if len(r.Settings) != len(NotificationCategories) {
		return false
	}
	seen := make(map[string]bool, len(r.Settings))
	for _, setting := range r.Settings {
		if !slices.Contains(NotificationCategories, setting.Category) || seen[setting.Category] {
			return false
		}
		seen[setting.Category] = true
	}
	return true
}

// NotificationSettings returns settings for every notification category.
func (c *Client) NotificationSettings(ctx context.Context, tenantID string) (*NotificationSettingsResponse, error) {
	var response NotificationSettingsResponse
	if err := c.do(ctx, http.MethodGet, tenantPath(tenantID, "/notification-settings"), nil, nil, &response); err != nil {
		return nil, err
	}
	if err := required(response.valid()); err != nil {
		return nil, err
	}
	return &response, nil
}

// UpdateNotificationSettings updates complete category settings.
func (c *Client) UpdateNotificationSettings(ctx context.Context, tenantID string, settings []NotificationSetting) (*NotificationSettingsResponse, error) {
	var response NotificationSettingsResponse
	changes := make([]struct {
		Category string `json:"category"`
		InApp    bool   `json:"in_app"`
		Email    bool   `json:"email"`
	}, len(settings))
	for i, setting := range settings {
		changes[i] = struct {
			Category string `json:"category"`
			InApp    bool   `json:"in_app"`
			Email    bool   `json:"email"`
		}{Category: setting.Category, InApp: setting.InApp, Email: setting.Email}
	}
	if err := c.do(ctx, http.MethodPut, tenantPath(tenantID, "/notification-settings"), nil, map[string]any{"settings": changes}, &response); err != nil {
		return nil, err
	}
	if err := required(response.valid()); err != nil {
		return nil, err
	}
	return &response, nil
}
