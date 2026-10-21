package cli

import (
	"fmt"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/Runzivo/rzv-cli/internal/api"
)

const insightsUsage = `Usage:
  rzv insights summary [--days <1-90>] [--workspace <id>] [--json]
  rzv insights heatmap [--days <1-90>] [--workspace <id>] [--json]
  rzv insights slow-jobs [--days <1-90>] [--limit <1-25>] [--workspace <id>] [--json]`

func init() {
	register("insights", "Show workspace job insights", runInsights)
	commandUsage["insights"] = insightsUsage
}
func insightInt(cmd *command, name string, max int) (int, error) {
	raw, err := cmd.option(name)
	if err != nil || raw == "" {
		return 0, err
	}
	n, e := strconv.Atoi(raw)
	if e != nil || n < 1 || n > max {
		return 0, usageError("--%s must be between 1 and %d.", name, max)
	}
	return n, nil
}
func runInsights(a *app, cmd *command) error {
	if len(cmd.args) == 0 {
		return usageError("%s", insightsUsage)
	}
	if err := cmd.allow("workspace", "account", "days", "limit"); err != nil {
		return err
	}
	if len(cmd.args) != 1 {
		return usageError("%s", insightsUsage)
	}
	days, err := insightInt(cmd, "days", 90)
	if err != nil {
		return err
	}
	if cmd.args[0] != "summary" && cmd.args[0] != "heatmap" && cmd.args[0] != "slow-jobs" {
		return usageError("%s", insightsUsage)
	}
	limit := 0
	if cmd.args[0] == "slow-jobs" {
		limit, err = insightInt(cmd, "limit", 25)
		if err != nil {
			return err
		}
	} else if _, present := cmd.options["limit"]; present {
		return usageError("--limit is only valid for slow-jobs.")
	}
	id, c, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	switch cmd.args[0] {
	case "summary":
		v, e := c.InsightsSummary(a.ctx, id, days)
		if e != nil {
			return apiError(e, "")
		}
		a.output(cmd, map[string]any{"workspace_id": id, "summary": v}, renderInsightsSummary(id, v))
		return nil
	case "heatmap":
		v, e := c.InsightsHeatmap(a.ctx, id, days)
		if e != nil {
			return apiError(e, "")
		}
		var b strings.Builder
		w := tabwriter.NewWriter(&b, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "WEEKDAY\tHOUR\tJOBS\tSTARTED JOBS\tMEDIAN WAIT\tP90 WAIT")
		for _, x := range v.Cells {
			fmt.Fprintf(w, "%d\t%02d:00\t%d\t%d\t%s\t%s\n", x.Weekday, x.Hour, x.Jobs, x.StartedJobs, secondsOrDash(x.MedianWait), secondsOrDash(x.P90Wait))
		}
		w.Flush()
		a.output(cmd, map[string]any{"workspace_id": id, "heatmap": v}, fmt.Sprintf("Insights heatmap for workspace %s (%d days, %s):\n%s", display(id), v.WindowDays, display(v.Timezone), strings.TrimSuffix(b.String(), "\n")))
		return nil
	case "slow-jobs":
		v, e := c.InsightsSlowJobs(a.ctx, id, days, limit)
		if e != nil {
			return apiError(e, "")
		}
		var b strings.Builder
		w := tabwriter.NewWriter(&b, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "RANK\tREPOSITORY\tWORKFLOW\tRUNS\tMEDIAN DURATION\tP90 DURATION\tMAX DURATION\tLAST RUNNER")
		for index, x := range v.Workflows {
			fmt.Fprintf(w, "%d\t%s\t%s\t%d\t%s\t%s\t%s\t%s\n", index+1, displayOrDash(x.Repository), displayOrDash(x.Workflow), x.Runs, seconds(x.MedianDurationSeconds), seconds(x.P90DurationSeconds), seconds(x.MaxDurationSeconds), displayOrDash(x.LastRunnerLabel))
		}
		w.Flush()
		a.output(cmd, map[string]any{"workspace_id": id, "slow_jobs": v}, fmt.Sprintf("Slow jobs for workspace %s (%d days, %s):\n%s", display(id), v.WindowDays, display(v.Timezone), strings.TrimSuffix(b.String(), "\n")))
		return nil
	}
	return usageError("%s", insightsUsage)
}

func renderInsightsSummary(workspaceID string, summary *api.InsightsSummary) string {
	lines := []string{
		fmt.Sprintf("Insights summary for workspace %s (%d days, %s):", display(workspaceID), summary.WindowDays, display(summary.Timezone)),
		fmt.Sprintf("Totals: %d jobs, %g minutes (%d jobs with recorded minutes)", summary.Totals.Jobs, summary.Totals.Minutes, summary.Totals.JobsWithMinutes),
		fmt.Sprintf("Duration (%d samples): median %s, p90 %s", summary.Duration.Samples, secondsOrDash(summary.Duration.MedianSeconds), secondsOrDash(summary.Duration.P90Seconds)),
		fmt.Sprintf("Queue wait (%d samples): median %s, p90 %s", summary.QueueWait.Samples, secondsOrDash(summary.QueueWait.MedianSeconds), secondsOrDash(summary.QueueWait.P90Seconds)),
		"",
		"Jobs by day:",
	}
	var jobs strings.Builder
	jobsWriter := tabwriter.NewWriter(&jobs, 0, 4, 2, ' ', 0)
	fmt.Fprintln(jobsWriter, "DATE\tTOTAL\tQUEUED\tIN PROGRESS\tCOMPLETED\tFAILED\tCANCELLED\tSUCCESS\tFAILURE\tCONCLUSION CANCELLED\tOTHER\tPENDING")
	for _, day := range summary.JobsByDay {
		fmt.Fprintf(jobsWriter, "%s\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\n", display(day.Date), day.Total, day.Status.Queued, day.Status.InProgress, day.Status.Completed, day.Status.Failed, day.Status.Cancelled, day.Conclusions.Success, day.Conclusions.Failure, day.Conclusions.Cancelled, day.Conclusions.Other, day.Conclusions.Pending)
	}
	_ = jobsWriter.Flush()
	lines = append(lines, strings.TrimSuffix(jobs.String(), "\n"), "", "Recorded minutes by day:")
	var minutes strings.Builder
	minutesWriter := tabwriter.NewWriter(&minutes, 0, 4, 2, ' ', 0)
	fmt.Fprintln(minutesWriter, "DATE\tMINUTES\tRUNNERS")
	for _, day := range summary.MinutesByDay {
		runners := make([]string, 0, len(day.Runners))
		for _, runner := range day.Runners {
			runners = append(runners, fmt.Sprintf("%s (%d vCPU, %d MiB): %g minutes, %d jobs", display(runner.RunnerLabel), runner.VCPUCount, runner.MemSizeMiB, runner.Minutes, runner.Jobs))
		}
		if len(runners) == 0 {
			runners = append(runners, "-")
		}
		fmt.Fprintf(minutesWriter, "%s\t%g\t%s\n", display(day.Date), day.Minutes, strings.Join(runners, "; "))
	}
	_ = minutesWriter.Flush()
	return strings.Join(append(lines, strings.TrimSuffix(minutes.String(), "\n")), "\n")
}

func seconds(value float64) string { return strconv.FormatFloat(value, 'f', -1, 64) + "s" }

func secondsOrDash(value *float64) string {
	if value == nil {
		return "-"
	}
	return seconds(*value)
}
