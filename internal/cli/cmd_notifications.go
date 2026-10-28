package cli

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/Runzivo/rzv-cli/internal/api"
)

const notificationsUsage = `Usage:
  rzv notifications list [--category <category>] [--unread true|false] [--limit <1-100>] [--cursor <cursor>] [--workspace <id>] [--json]
  rzv notifications read <id...> [--workspace <id>] [--json]
  rzv notifications read --all [--workspace <id>] [--json]
  rzv notifications settings show [--workspace <id>] [--json]
  rzv notifications settings update --category <category> --in-app true|false --email true|false [--workspace <id>] [--json]

Categories: billing, jobs, account, support, system.
Notifications and notification settings require your own sign-in; API keys are not accepted.`

const maxNotificationReadIDs = 100

func init() {
	register("notifications", "List, mark read, and configure workspace notifications", runNotifications)
	commandUsage["notifications"] = notificationsUsage
}

func runNotifications(a *app, cmd *command) error {
	if len(cmd.args) == 0 {
		return usageError("%s", notificationsUsage)
	}
	switch cmd.args[0] {
	case "list":
		return notificationsList(a, cmd)
	case "read":
		return notificationsRead(a, cmd)
	case "settings":
		return notificationSettings(a, cmd)
	default:
		return usageError("%s", notificationsUsage)
	}
}

func notificationWorkspaceClient(a *app, cmd *command) (string, *api.Client, error) {
	if err := a.requireHuman(); err != nil {
		return "", nil, err
	}
	return a.workspaceClient(cmd)
}

func notificationsList(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account", "category", "unread", "limit", "cursor"); err != nil {
		return err
	}
	if err := cmd.expectArgs(1, "rzv notifications list [--category <category>] [--unread true|false] [--limit <1-100>] [--cursor <cursor>] [--workspace <id>] [--json]"); err != nil {
		return err
	}
	options, err := notificationListOptions(cmd)
	if err != nil {
		return err
	}
	id, client, err := notificationWorkspaceClient(a, cmd)
	if err != nil {
		return err
	}
	page, err := client.Notifications(a.ctx, id, options)
	if err != nil {
		return apiError(err, "Notifications were not found in this workspace.")
	}
	a.output(cmd, map[string]any{"workspace_id": id, "notifications": page.Notifications, "unread_count": page.UnreadCount, "next_cursor": page.NextCursor}, renderNotifications(id, page))
	return nil
}

func notificationListOptions(cmd *command) (api.NotificationListOptions, error) {
	var options api.NotificationListOptions
	category, err := cmd.option("category")
	if err != nil {
		return options, err
	}
	if category != "" && !slices.Contains(api.NotificationCategories, category) {
		return options, usageError("--category must be billing, jobs, account, support, or system.")
	}
	options.Category = category
	if _, ok := cmd.options["unread"]; ok {
		unread, err := cmd.boolOption("unread")
		if err != nil {
			return options, err
		}
		options.Unread = &unread
	}
	if _, ok := cmd.options["limit"]; ok {
		limitText, err := cmd.required("limit")
		if err != nil {
			return options, err
		}
		limit, err := strconv.Atoi(limitText)
		if err != nil || limit < 1 || limit > 100 {
			return options, usageError("--limit must be an integer from 1 to 100.")
		}
		options.Limit = limit
	}
	cursor, err := cmd.option("cursor")
	if err != nil {
		return options, err
	}
	if len(cursor) > 512 {
		return options, usageError("--cursor must be at most 512 bytes.")
	}
	options.Cursor = cursor
	return options, nil
}

func notificationsRead(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account", "all"); err != nil {
		return err
	}
	if len(cmd.args) < 1 || (cmd.flags["all"] == (len(cmd.args) > 1)) {
		return usageError("Use `rzv notifications read <id...> [--workspace <id>] [--json]` or `rzv notifications read --all [--workspace <id>] [--json]`.")
	}
	ids := cmd.args[1:]
	if err := validateNotificationIDs(ids, cmd.flags["all"]); err != nil {
		return err
	}
	id, client, err := notificationWorkspaceClient(a, cmd)
	if err != nil {
		return err
	}
	unread, err := client.MarkNotificationsRead(a.ctx, id, ids, cmd.flags["all"])
	if err != nil {
		return apiError(err, "Notifications were not found in this workspace.")
	}
	a.output(cmd, map[string]any{"workspace_id": id, "unread_count": unread}, fmt.Sprintf("Unread notifications for workspace %s: %d", display(id), unread))
	return nil
}

func validateNotificationIDs(ids []string, all bool) error {
	if all {
		return nil
	}
	if len(ids) == 0 || len(ids) > maxNotificationReadIDs {
		return usageError("Pass from 1 to %d notification IDs, or use --all.", maxNotificationReadIDs)
	}
	for _, id := range ids {
		value, err := strconv.ParseInt(strings.TrimSpace(id), 10, 64)
		if err != nil || value <= 0 {
			return usageError("Notification IDs must be positive integers.")
		}
	}
	return nil
}

func notificationSettings(a *app, cmd *command) error {
	if len(cmd.args) < 2 {
		return usageError("%s", notificationsUsage)
	}
	switch cmd.args[1] {
	case "show":
		return notificationSettingsShow(a, cmd)
	case "update":
		return notificationSettingsUpdate(a, cmd)
	default:
		return usageError("%s", notificationsUsage)
	}
}

func notificationSettingsShow(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	if err := cmd.expectArgs(2, "rzv notifications settings show [--workspace <id>] [--json]"); err != nil {
		return err
	}
	id, client, err := notificationWorkspaceClient(a, cmd)
	if err != nil {
		return err
	}
	settings, err := client.NotificationSettings(a.ctx, id)
	if err != nil {
		return apiError(err, "Notification settings were not found in this workspace.")
	}
	a.output(cmd, map[string]any{"workspace_id": id, "settings": settings.Settings}, renderNotificationSettings(id, settings.Settings))
	return nil
}

func notificationSettingsUpdate(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account", "category", "in-app", "email"); err != nil {
		return err
	}
	if err := cmd.expectArgs(2, "rzv notifications settings update --category <category> --in-app true|false --email true|false [--workspace <id>] [--json]"); err != nil {
		return err
	}
	category, err := cmd.required("category")
	if err != nil {
		return err
	}
	if !slices.Contains(api.NotificationCategories, category) {
		return usageError("--category must be billing, jobs, account, support, or system.")
	}
	inApp, err := cmd.boolOption("in-app")
	if err != nil {
		return err
	}
	email, err := cmd.boolOption("email")
	if err != nil {
		return err
	}
	id, client, err := notificationWorkspaceClient(a, cmd)
	if err != nil {
		return err
	}
	settings, err := client.UpdateNotificationSettings(a.ctx, id, []api.NotificationSetting{{Category: category, InApp: inApp, Email: email}})
	if err != nil {
		return apiError(err, "Notification settings were not found in this workspace.")
	}
	a.output(cmd, map[string]any{"workspace_id": id, "settings": settings.Settings}, renderNotificationSettings(id, settings.Settings))
	return nil
}

func renderNotifications(id string, page *api.NotificationPage) string {
	if len(page.Notifications) == 0 {
		return fmt.Sprintf("No notifications for workspace %s. Unread: %d.", display(id), page.UnreadCount)
	}
	var table strings.Builder
	writer := tabwriter.NewWriter(&table, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "ID\tREAD\tCATEGORY\tKIND\tTITLE\tBODY\tLINK\tCREATED")
	for _, notification := range page.Notifications {
		read := "no"
		if notification.Read {
			read = "yes"
		}
		_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", display(notification.ID), read, display(notification.Category), display(notification.Kind), display(notification.Title), display(notification.Body), displayOrDash(notification.Link), display(notification.CreatedAt))
	}
	_ = writer.Flush()
	lines := []string{fmt.Sprintf("Notifications for workspace %s (unread: %d):", display(id), page.UnreadCount), strings.TrimSuffix(table.String(), "\n")}
	if page.NextCursor != nil {
		lines = append(lines, "Next cursor: "+display(*page.NextCursor))
	}
	return strings.Join(lines, "\n")
}

func renderNotificationSettings(id string, settings []api.NotificationSetting) string {
	var table strings.Builder
	writer := tabwriter.NewWriter(&table, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "CATEGORY\tIN-APP\tEMAIL\tEMAIL LOCKED")
	for _, setting := range settings {
		_, _ = fmt.Fprintf(writer, "%s\t%t\t%t\t%t\n", display(setting.Category), setting.InApp, setting.Email, setting.EmailLocked)
	}
	_ = writer.Flush()
	return "Notification settings for workspace " + display(id) + ":\n" + strings.TrimSuffix(table.String(), "\n")
}
