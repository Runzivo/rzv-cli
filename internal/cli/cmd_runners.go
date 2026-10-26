package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Runzivo/rzv-cli/internal/api"
)

const concurrencyUsage = "Usage: rzv concurrency [--workspace <id>] [--json]"

const runnersUsage = `Usage: rzv runners <repository-id> [--workspace <id>] [--json]

<repository-id> is GitHub's numeric repository id (see rzv repositories list).
Lists the runner sizes the repository can use, with the workspace's running
and waiting jobs on each.`

func init() {
	register("concurrency", "Show the workspace's concurrent-job limit and use", runConcurrency)
	commandUsage["concurrency"] = concurrencyUsage
	register("runners", "Show runner sizes and active jobs for a repository", runRunners)
	commandUsage["runners"] = runnersUsage
}

func runConcurrency(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	if err := cmd.expectArgs(0, "rzv concurrency [--workspace <id>] [--json]"); err != nil {
		return err
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	outcome, err := client.Concurrency(a.ctx, id)
	if err != nil {
		return apiError(err, "")
	}
	value := map[string]any{"workspace_id": id, "kind": outcome.Kind}
	if outcome.Concurrency != nil {
		value["concurrency"] = outcome.Concurrency
	}
	a.output(cmd, value, renderConcurrency(id, outcome))
	return nil
}

func renderConcurrency(workspaceID string, outcome *api.ConcurrencyOutcome) string {
	if outcome.Kind == "not_linked" {
		return "No GitHub App installation is linked to workspace " + display(workspaceID) + "; there is no concurrency limit to report."
	}
	c := outcome.Concurrency
	lines := []string{
		fmt.Sprintf("Concurrency for workspace %s:", display(workspaceID)),
		fmt.Sprintf("Running: %d of %d (%d remaining)", c.Used, c.Limit, c.Remaining),
		"Admission: " + display(c.Admission),
	}
	if c.Reason != nil {
		lines = append(lines, "Reason: "+display(*c.Reason))
	}
	return strings.Join(lines, "\n")
}

func runRunners(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	if err := cmd.expectArgs(1, "rzv runners <repository-id> [--workspace <id>] [--json]"); err != nil {
		return err
	}
	repoID, err := strconv.ParseInt(cmd.args[0], 10, 64)
	if err != nil || repoID < 1 {
		return usageError("<repository-id> must be GitHub's numeric repository id, not its name. See `rzv repositories list`.")
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	outcome, err := client.RunnerInventory(a.ctx, id, repoID)
	if err != nil {
		return apiError(err, "")
	}
	value := map[string]any{"workspace_id": id, "repository_id": repoID, "kind": outcome.Kind}
	if outcome.Inventory != nil {
		value["runner_inventory"] = outcome.Inventory
	}
	a.output(cmd, value, renderRunners(id, repoID, outcome))
	return nil
}

func renderRunners(workspaceID string, repoID int64, outcome *api.InventoryOutcome) string {
	switch outcome.Kind {
	case "not_linked":
		return "No GitHub App installation is linked to workspace " + display(workspaceID) + "; there are no runner sizes to list."
	case "not_authorized":
		return fmt.Sprintf("Repository %d is not authorized for workspace %s.", repoID, display(workspaceID))
	}
	inventory := outcome.Inventory
	if len(inventory.Profiles) == 0 {
		return fmt.Sprintf("No published runner sizes for repository %d.", repoID)
	}
	lines := []string{fmt.Sprintf("Runner sizes for repository %d (workspace %s):", repoID, display(workspaceID))}
	for _, p := range inventory.Profiles {
		status := "available"
		if !p.Available {
			reason := "unavailable"
			if p.Reason != nil {
				reason = *p.Reason
			}
			status = "blocked (" + display(reason) + ")"
		}
		lines = append(lines, fmt.Sprintf("- %s: %d vCPU, %d MiB, %d running, %d waiting, runs-on: %s",
			status, p.VCPUCount, p.MemSizeMiB, p.RunningJobs, p.WaitingJobs, display(p.Label)))
	}
	if u := inventory.Unlisted; u.RunningJobs > 0 || u.WaitingJobs > 0 {
		lines = append(lines, fmt.Sprintf("Other sizes no longer listed: %d running, %d waiting", u.RunningJobs, u.WaitingJobs))
	}
	return strings.Join(lines, "\n")
}
