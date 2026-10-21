package cli

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Runzivo/rzv-cli/internal/api"
)

const activityUsage = `Usage:
  rzv activity list [--category login|general] [--actor-type customer|staff|api_key|system] [--result success|failure] [--query <text>] [--since <RFC3339>] [--until <RFC3339>] [--limit 1..100] [--cursor <cursor>] [--workspace <id>] [--json]
  rzv activity export [--category login|general] [--actor-type customer|staff|api_key|system] [--result success|failure] [--query <text>] [--since <RFC3339>] [--until <RFC3339>] [--workspace <id>] [--json]
  rzv activity status [--period-days 7|30|60] [--workspace <id>] [--json]`

func init() {
	register("activity", "List workspace activity or audit status", runActivity)
	commandUsage["activity"] = activityUsage
}
func runActivity(a *app, cmd *command) error {
	if len(cmd.args) == 0 {
		return usageError("%s", activityUsage)
	}
	switch cmd.args[0] {
	case "list":
		return activityList(a, cmd)
	case "export":
		return activityExport(a, cmd)
	case "status":
		return activityStatus(a, cmd)
	}
	return usageError("%s", activityUsage)
}

func activityFilter(cmd *command, paging bool) (api.ActivityFilter, error) {
	f := api.ActivityFilter{}
	var err error
	if f.Category, err = cmd.option("category"); err != nil {
		return f, err
	}
	if f.Category != "" && !slices.Contains([]string{"login", "general"}, f.Category) {
		return f, usageError("--category must be login or general.")
	}
	if f.ActorType, err = cmd.option("actor-type"); err != nil {
		return f, err
	}
	if f.ActorType != "" && !slices.Contains([]string{"customer", "staff", "api_key", "system"}, f.ActorType) {
		return f, usageError("--actor-type must be customer, staff, api_key, or system.")
	}
	if f.Result, err = cmd.option("result"); err != nil {
		return f, err
	}
	if f.Result != "" && !slices.Contains([]string{"success", "failure"}, f.Result) {
		return f, usageError("--result must be success or failure.")
	}
	if f.Query, err = cmd.option("query"); err != nil {
		return f, err
	}
	if len(f.Query) > 100 {
		return f, usageError("--query must be at most 100 bytes.")
	}
	for _, v := range []struct {
		name string
		to   *string
	}{{"since", &f.Since}, {"until", &f.Until}} {
		if *v.to, err = cmd.option(v.name); err != nil {
			return f, err
		}
		if *v.to != "" {
			if len(*v.to) > 64 {
				return f, usageError("--%s must be at most 64 bytes.", v.name)
			}
			if _, e := time.Parse(time.RFC3339Nano, *v.to); e != nil {
				return f, usageError("--%s must be RFC3339.", v.name)
			}
		}
	}
	if f.Since != "" && f.Until != "" {
		s, _ := time.Parse(time.RFC3339Nano, f.Since)
		u, _ := time.Parse(time.RFC3339Nano, f.Until)
		if u.Before(s) {
			return f, usageError("--until must not be before --since.")
		}
	}
	if paging {
		raw, err := cmd.option("limit")
		if err != nil {
			return f, err
		}
		if raw != "" {
			f.Limit, err = strconv.Atoi(raw)
			if err != nil || f.Limit < 1 || f.Limit > 100 {
				return f, usageError("--limit must be an integer from 1 to 100.")
			}
		}
		f.Cursor, err = cmd.option("cursor")
		if err != nil {
			return f, err
		}
		if len(f.Cursor) > 512 {
			return f, usageError("--cursor must be at most 512 bytes.")
		}
	}
	return f, nil
}
func activityList(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account", "category", "actor-type", "result", "query", "since", "until", "limit", "cursor"); err != nil {
		return err
	}
	if err := cmd.expectArgs(1, "rzv activity list"); err != nil {
		return err
	}
	f, err := activityFilter(cmd, true)
	if err != nil {
		return err
	}
	id, c, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	page, err := c.Activity(a.ctx, id, f)
	if err != nil {
		return apiError(err, "The activity resource was not found in this workspace.")
	}
	a.output(cmd, map[string]any{"workspace_id": id, "activity": page}, renderActivity(page))
	return nil
}
func activityExport(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account", "category", "actor-type", "result", "query", "since", "until"); err != nil {
		return err
	}
	if err := cmd.expectArgs(1, "rzv activity export"); err != nil {
		return err
	}
	f, err := activityFilter(cmd, false)
	if err != nil {
		return err
	}
	id, c, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	csv, err := c.ExportActivity(a.ctx, id, f)
	if err != nil {
		return apiError(err, "The activity export resource was not found in this workspace.")
	}
	if cmd.json() {
		a.output(cmd, map[string]any{"workspace_id": id, "csv": csv}, "")
	} else {
		n, writeErr := fmt.Fprint(a.Stdout, csv)
		if writeErr != nil || n != len(csv) {
			return newError("output_failed", exitFailure, "Could not write the CSV export.")
		}
	}
	return nil
}
func activityStatus(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account", "period-days"); err != nil {
		return err
	}
	if err := cmd.expectArgs(1, "rzv activity status"); err != nil {
		return err
	}
	raw, err := cmd.option("period-days")
	if err != nil {
		return err
	}
	days := 0
	if raw != "" {
		days, err = strconv.Atoi(raw)
		if err != nil || !slices.Contains([]int{7, 30, 60}, days) {
			return usageError("--period-days must be 7, 30, or 60.")
		}
	}
	id, c, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	status, err := c.AuditStatus(a.ctx, id, days)
	if err != nil {
		return apiError(err, "The audit status resource was not found in this workspace.")
	}
	a.output(cmd, map[string]any{"workspace_id": id, "audit_status": status}, renderAuditStatus(status))
	return nil
}
func renderActivity(p *api.ActivityPage) string {
	if len(p.Events) == 0 {
		return fmt.Sprintf("No activity events.\nScope: %s\nRetention: %d days\nNext cursor: %s", display(p.Scope), p.RetentionDays, displayOrDash(p.NextCursor))
	}
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "TIME\tACTION\tRESULT\tERROR\tACTOR TYPE\tACTOR\tDESCRIPTION")
	for _, e := range p.Events {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", display(e.OccurredAt), display(e.Action), display(e.Result), displayOrDash(e.ErrorCode), display(e.Actor.Type), display(e.Actor.Label), display(e.Description))
	}
	_ = w.Flush()
	return strings.TrimSuffix(b.String(), "\n") + fmt.Sprintf("\nScope: %s\nRetention: %d days\nNext cursor: %s", display(p.Scope), p.RetentionDays, displayOrDash(p.NextCursor))
}
func renderAuditStatus(s *api.AuditStatus) string {
	lines := []string{fmt.Sprintf("Audit status (%d days):", s.PeriodDays), "Generated: " + display(s.GeneratedAt), "History recorded: " + strconv.FormatBool(s.HistoryRecorded), "Components:"}
	for _, c := range s.Components {
		uptime := "unknown"
		if c.UptimePercent != nil {
			uptime = fmt.Sprintf("%g%%", *c.UptimePercent)
		}
		lines = append(lines, fmt.Sprintf("- %s: %s (last signal %s, uptime %s): %s", display(c.Name), display(c.State), displayOrDash(c.LastSignalAt), uptime, display(c.Detail)))
	}
	lines = append(lines, fmt.Sprintf("Execution assurances: jobs %d, fresh microVMs %d, destroyed %d, clean teardowns %d, forced stops %d, awaiting teardown %d", s.Assurances.JobsRun, s.Assurances.FreshMicroVMs, s.Assurances.Destroyed, s.Assurances.CleanTeardowns, s.Assurances.ForcedStops, s.Assurances.AwaitingTeardown), "Lease evidence:")
	if len(s.Evidence) == 0 {
		lines = append(lines, "- none")
	} else {
		for _, e := range s.Evidence {
			lines = append(lines, fmt.Sprintf("- job %s, lease %s, booted %s, destroyed %s, teardown %s, repository %s", display(e.JobID), display(e.LeaseID), display(e.BootedAt), displayOrDash(e.DestroyedAt), display(e.Teardown), displayOrDash(e.Repository)))
		}
	}
	return strings.Join(lines, "\n")
}
