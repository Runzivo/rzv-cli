package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Runzivo/rzv-cli/internal/api"
)

const jobsUsage = `Usage:
  rzv jobs list [--workspace <id>] [--json]            Jobs, newest first
  rzv jobs get <job-id> [--workspace <id>] [--json]    One job's status
  rzv jobs logs <job-id> [--workspace <id>] [--json]   Job log text when available

"rzv jobs status <job-id>" is the same as "get". Status and conclusion are
separate: only conclusion "success" means the workflow job succeeded.`

func init() {
	register("jobs", "List jobs or show job status and logs", runJobs)
	commandUsage["jobs"] = jobsUsage
	register("usage", "Show the workspace's recorded usage", runUsage)
	commandUsage["usage"] = "Usage: rzv usage [--workspace <id>] [--json]"
}

func runJobs(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	if len(cmd.args) == 0 {
		return usageError("%s", jobsUsage)
	}
	switch cmd.args[0] {
	case "list":
		if err := cmd.expectArgs(1, "rzv jobs list [--workspace <id>] [--json]"); err != nil {
			return err
		}
		id, client, err := a.workspaceClient(cmd)
		if err != nil {
			return err
		}
		jobs, err := client.Jobs(a.ctx, id)
		if err != nil {
			return apiError(err, "")
		}
		a.output(cmd, map[string]any{"workspace_id": id, "jobs": jobs}, renderJobs(id, jobs))
		return nil
	case "get", "status":
		if err := cmd.expectArgs(2, "rzv jobs get <job-id> [--workspace <id>] [--json]"); err != nil {
			return err
		}
		id, client, err := a.workspaceClient(cmd)
		if err != nil {
			return err
		}
		job, err := client.Job(a.ctx, id, cmd.args[1])
		if err != nil {
			return apiError(err, "The job was not found in this workspace.")
		}
		a.output(cmd, map[string]any{"workspace_id": id, "job": job}, renderJob(id, job))
		return nil
	case "logs":
		if err := cmd.expectArgs(2, "rzv jobs logs <job-id> [--workspace <id>] [--json]"); err != nil {
			return err
		}
		id, client, err := a.workspaceClient(cmd)
		if err != nil {
			return err
		}
		logs, err := client.JobLogs(a.ctx, id, cmd.args[1])
		if err != nil {
			return apiError(err, "The job logs were not found in this workspace.")
		}
		text := "Logs for job " + display(logs.JobID) + " (" + display(logs.State) + "):"
		if logs.State == "available" {
			text += "\n" + displayLog(*logs.Content)
			if logs.Truncated {
				text += "\n[log truncated]"
			}
		} else {
			text += "\nLogs are not available."
			if logs.Reason != nil {
				text += " Reason: " + display(*logs.Reason)
			}
		}
		if logs.GitHubURL != nil {
			text += "\nFull log URL: " + display(*logs.GitHubURL)
		}
		a.output(cmd, map[string]any{"workspace_id": id, "logs": logs}, text)
		return nil
	default:
		return usageError("%s", jobsUsage)
	}
}

func renderJobs(workspaceID string, jobs []api.Job) string {
	if len(jobs) == 0 {
		return "No jobs for workspace " + display(workspaceID) + "."
	}
	lines := []string{"Jobs for workspace " + display(workspaceID) + ":"}
	for _, j := range jobs {
		conclusion := ""
		if j.Conclusion != nil {
			conclusion = " (" + display(*j.Conclusion) + ")"
		}
		lines = append(lines, fmt.Sprintf("- %s %s%s %s (%s)", display(j.JobID), display(j.Status), conclusion, display(j.Repository), display(j.Workflow)))
	}
	return strings.Join(lines, "\n")
}

func renderJob(workspaceID string, j *api.Job) string {
	lines := []string{
		fmt.Sprintf("Job %s for workspace %s:", display(j.JobID), display(workspaceID)),
		fmt.Sprintf("GitHub job: %d (attempt %d)", j.WorkflowJobID, j.RunAttempt),
		fmt.Sprintf("Repository: %s [%s]", display(j.Repository), display(j.RepositoryID)),
		"Workflow: " + display(j.Workflow),
		fmt.Sprintf("Runner: %s (%d vCPU, %d MiB)", display(j.RunnerLabel), j.VCPUCount, j.MemSizeMiB),
		"Status: " + display(j.Status),
		// Only the conclusion says whether the workflow job succeeded.
		"Conclusion: " + displayOrDash(j.Conclusion),
	}
	if j.WaitReason != "" {
		lines = append(lines, "Waiting for: "+display(j.WaitReason))
	}
	lines = append(lines,
		"Queued: "+display(j.QueuedAt),
		"Started: "+displayOrDash(j.StartedAt),
		"Completed: "+displayOrDash(j.CompletedAt),
	)
	if j.BillableMinutes != nil {
		lines = append(lines, "Billable minutes: "+strconv.FormatFloat(*j.BillableMinutes, 'f', -1, 64))
	}
	return strings.Join(lines, "\n")
}

func runUsage(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	if err := cmd.expectArgs(0, "rzv usage [--workspace <id>] [--json]"); err != nil {
		return err
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	outcome, err := client.Usage(a.ctx, id)
	if err != nil {
		return apiError(err, "")
	}
	value := map[string]any{"workspace_id": id, "kind": outcome.Kind}
	if outcome.Usage != nil {
		value["usage"] = outcome.Usage
	}
	a.output(cmd, value, renderUsage(id, outcome))
	return nil
}

func renderUsage(workspaceID string, outcome *api.UsageOutcome) string {
	switch outcome.Kind {
	case "not_linked":
		return "No GitHub App installation is linked to workspace " + display(workspaceID) + "; there is no usage yet."
	case "not_recorded":
		return "No finished jobs yet for workspace " + display(workspaceID) + "."
	}
	u := outcome.Usage
	lines := []string{
		fmt.Sprintf("Usage for workspace %s (%s to %s):", display(workspaceID), display(u.PeriodStart), display(u.PeriodEnd)),
		"Billable minutes: " + strconv.FormatFloat(u.BillableMinutes, 'f', -1, 64),
		fmt.Sprintf("Jobs: %d", u.JobCount),
	}
	if len(u.Notes) > 0 {
		notes := make([]string, len(u.Notes))
		for i, n := range u.Notes {
			notes[i] = display(n)
		}
		lines = append(lines, "Notes: "+strings.Join(notes, "; "))
	}
	return strings.Join(lines, "\n")
}
