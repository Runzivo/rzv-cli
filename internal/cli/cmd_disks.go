package cli

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Runzivo/rzv-cli/internal/api"
)

const disksUsage = `Usage:
  rzv disks list [--workspace <id>] [--json]
  rzv disks get <disk-id> [--workspace <id>] [--json]
  rzv disks create --name <name> --size-gib <1-16384> [--idempotency-key <key>] [--workspace <id>] [--json]
  rzv disks attach <disk-id> --sandbox <sandbox-id> [--workspace <id>] [--json]
  rzv disks detach <disk-id> [--workspace <id>] [--json]
  rzv disks delete <disk-id> [--workspace <id>] [--json]

A persistent disk keeps files after a sandbox ends. Create a sandbox with it,
"rzv sandboxes create --name dev --disk <disk-id>", and it is mounted at /mnt/disk
while the sandbox runs. The disk lives on one Runzivo server: a sandbox that uses
it runs on that server or waits for it. It is not copied to another server and is
not a backup. One sandbox can use a disk at a time; stop the sandbox before you
detach the disk or attach it to another stopped sandbox. Deleting a disk removes
its files for good. A deleted disk shows as "deleting" until its server confirms
the files are removed, then as "deleted". Sizes are in GiB (1 GiB is 1024^3 bytes).

Persistent disks must be enabled for your environment. In grant mode your
workspace needs a disk quota; in capacity mode any workspace can create disks
while the region has room. "rzv disks list --json" reports mode, quota and the
current max_size_gib. Core enforces these limits; 16384 GiB is the technical
ceiling, not a guarantee of available space.

The published price is CA$0.10 per provisioned GiB-month, prorated while a disk
exists, attached or detached, until deletion is confirmed. The customer disk
estimate API (getCustomerDiskEstimate) reports current rates, monthly estimates
and charges_enabled. When charges are on, disks consume equivalent minutes
from the workspace balance. Disk charges are currently off in production.
Disks use the sandboxes:read and sandboxes:write permissions.`

var diskIDPattern = regexp.MustCompile(`^vol_[0-9a-f]{24}$`)

func init() {
	register("disks", "Create persistent disks for sandboxes", runDisks)
	commandUsage["disks"] = disksUsage
}

func diskID(value string) (string, error) {
	if !diskIDPattern.MatchString(value) {
		return "", usageError("A disk id looks like vol_ followed by 24 letters and digits. See `rzv disks list`.")
	}
	return value, nil
}

func runDisks(a *app, cmd *command) error {
	if len(cmd.args) == 0 {
		return usageError("%s", disksUsage)
	}
	switch cmd.args[0] {
	case "list":
		return disksList(a, cmd)
	case "get":
		return diskAction(a, cmd, "get", func(c *api.Client, ws, id string) (*api.Disk, error) { return c.DiskGet(a.ctx, ws, id) }, "")
	case "create":
		return disksCreate(a, cmd)
	case "attach":
		return disksAttach(a, cmd)
	case "detach":
		return diskAction(a, cmd, "detach", func(c *api.Client, ws, id string) (*api.Disk, error) { return c.DiskDetach(a.ctx, ws, id) }, "Detached")
	case "delete":
		return diskAction(a, cmd, "delete", func(c *api.Client, ws, id string) (*api.Disk, error) { return c.DiskDelete(a.ctx, ws, id) }, "Deleting")
	default:
		return usageError("%s", disksUsage)
	}
}

func disksList(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	if err := cmd.expectArgs(1, "rzv disks list [--workspace <id>] [--json]"); err != nil {
		return err
	}
	workspace, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	listing, err := client.DiskList(a.ctx, workspace)
	if err != nil {
		return apiError(err, "")
	}
	a.output(cmd, map[string]any{
		"workspace_id": workspace,
		"disks":        listing.Disks,
		"quota":        listing.Quota,
		"min_size_gib": listing.MinSizeGiB,
		"max_size_gib": listing.MaxSizeGiB,
	}, renderDiskList(workspace, listing))
	return nil
}

func renderDiskQuota(quota api.DiskQuota) string {
	if quota.Mode == "capacity" {
		text := fmt.Sprintf("Capacity: %d of %d GiB used, %d of %d disks.", quota.UsedGiB, quota.QuotaGiB, quota.Disks, quota.MaxDisks)
		if quota.QuotaGiB == 0 {
			text = "No persistent disk capacity is available in this region. Try again when space is available."
		}
		if quota.RegionCapacityGiB != nil && quota.RegionFreeGiB != nil {
			text += fmt.Sprintf("\nRegion: %d of %d GiB free.", *quota.RegionFreeGiB, *quota.RegionCapacityGiB)
		}
		return text
	}
	if quota.QuotaGiB == 0 {
		return "This workspace has no persistent disk quota. Ask Runzivo support for one."
	}
	return fmt.Sprintf("Quota: %d of %d GiB used, %d of %d disks.", quota.UsedGiB, quota.QuotaGiB, quota.Disks, quota.MaxDisks)
}

func renderDiskList(workspace string, listing *api.DiskListing) string {
	live := []api.Disk{}
	for _, d := range listing.Disks {
		if d.Status != "deleted" {
			live = append(live, d)
		}
	}
	if len(live) == 0 {
		return "No persistent disks in workspace " + display(workspace) + ". Create one with `rzv disks create --name <name> --size-gib <size>`.\n" + renderDiskQuota(listing.Quota)
	}
	var out strings.Builder
	table := tabwriter.NewWriter(&out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "ID\tNAME\tSIZE\tSTATUS\tATTACHED TO\tUSAGE")
	for _, d := range live {
		fmt.Fprintf(table, "%s\t%s\t%d GiB\t%s\t%s\t%s\n", display(d.ID), display(d.Name), d.SizeGiB, display(d.Status), attachedTo(d), diskUsage(d.Usage))
	}
	_ = table.Flush()
	for _, d := range live {
		if note := scheduledDeletion(d); note != "" {
			out.WriteString(display(d.ID) + ": " + note + "\n")
		}
	}
	out.WriteString("\n" + renderDiskQuota(listing.Quota))
	return out.String()
}

// scheduledDeletion describes a pending deletion by Runzivo, or "" if none.
func scheduledDeletion(d api.Disk) string {
	s := d.ScheduledDeletion
	if s == nil {
		return ""
	}
	when := display(s.DeleteAfter)
	if t, err := s.DeleteAfterTime(); err == nil {
		when = t.UTC().Format("2006-01-02 15:04") + " UTC"
	}
	reason := display(s.Reason)
	switch s.Reason {
	case "unpaid":
		reason = "unpaid"
	case "workspace_closed":
		reason = "workspace closed"
	}
	return "Scheduled for deletion on " + when + " (" + reason + ")"
}

// diskError is apiError for a call about one known disk. disk_wrong_state
// reads the disk to say whether it is past its scheduled deletion time; if
// that read fails, apiError's combined message stands.
func diskError(a *app, client *api.Client, workspace, id string, err error) error {
	var apiErr *api.Error
	if !errors.As(err, &apiErr) || apiErr.Status != 409 || apiErr.Code != "disk_wrong_state" {
		return apiError(err, "")
	}
	disk, getErr := client.DiskGet(a.ctx, workspace, id)
	if getErr != nil {
		return apiError(err, "")
	}
	if s := disk.ScheduledDeletion; s != nil {
		if t, parseErr := s.DeleteAfterTime(); parseErr == nil && !t.After(time.Now()) {
			return newError(apiErr.Code, exitFailure, "That disk is past its scheduled deletion time and is being deleted.")
		}
	}
	return newError(apiErr.Code, exitFailure, "That disk is being deleted.")
}

func attachedTo(d api.Disk) string {
	if d.AttachedSandboxID == nil {
		return "-"
	}
	return display(*d.AttachedSandboxID)
}

func diskUsage(usage api.DiskUsage) string {
	return strconv.FormatFloat(usage.GBHours, 'f', 1, 64) + " GB-h, " + credits(usage.CreditsUsed)
}

func renderDisk(d *api.Disk, verb string) string {
	head := fmt.Sprintf("%s [%s]: %s", display(d.Name), display(d.ID), display(d.Status))
	if verb != "" {
		head = verb + " disk " + head
	}
	lines := []string{head, fmt.Sprintf("Size: %d GiB", d.SizeGiB)}
	if d.AttachedSandboxID != nil {
		lines = append(lines, "Attached to: "+display(*d.AttachedSandboxID)+", mounted at "+display(d.MountPath)+" while it runs")
	} else {
		lines = append(lines, "Not attached")
	}
	lines = append(lines, "Usage: "+diskUsage(d.Usage))
	if note := scheduledDeletion(*d); note != "" {
		lines = append(lines, note)
	}
	return strings.Join(lines, "\n")
}

func diskAction(a *app, cmd *command, name string, call func(*api.Client, string, string) (*api.Disk, error), verb string) error {
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	if err := cmd.expectArgs(2, "rzv disks "+name+" <disk-id> [--workspace <id>] [--json]"); err != nil {
		return err
	}
	id, err := diskID(cmd.args[1])
	if err != nil {
		return err
	}
	workspace, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	disk, err := call(client, workspace, id)
	if err != nil {
		return diskError(a, client, workspace, id, err)
	}
	text := renderDisk(disk, verb)
	if verb == "Deleting" {
		if disk.Status == "deleted" {
			text = renderDisk(disk, "Deleted")
		} else {
			text += "\nIt shows as deleting until its server confirms the files are removed. Check with `rzv disks get " + display(disk.ID) + "`."
		}
	}
	a.output(cmd, map[string]any{"workspace_id": workspace, "disk": disk}, text)
	return nil
}

func disksCreate(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account", "name", "size-gib", "idempotency-key"); err != nil {
		return err
	}
	usage := "rzv disks create --name <name> --size-gib <1-16384>"
	if err := cmd.expectArgs(1, usage); err != nil {
		return err
	}
	name, err := cmd.required("name")
	if err != nil {
		return err
	}
	if strings.TrimSpace(name) == "" || len([]rune(name)) > 80 {
		return usageError("--name must be 1 to 80 characters.")
	}
	text, err := cmd.required("size-gib")
	if err != nil {
		return err
	}
	size, err := strconv.Atoi(text)
	if err != nil || size < 1 || size > 16384 {
		return usageError("--size-gib must be a whole number from 1 to 16384.")
	}
	input := api.CreateDiskInput{Name: name, SizeGiB: size}
	if key, err := cmd.option("idempotency-key"); err != nil {
		return err
	} else if key != "" {
		if len(key) < 8 || len(key) > 200 {
			return usageError("--idempotency-key must be 8 to 200 characters.")
		}
		input.ClientOperationID = key
	}
	workspace, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	disk, err := client.DiskCreate(a.ctx, workspace, input)
	if err != nil {
		return apiError(err, "")
	}
	a.output(cmd, map[string]any{"workspace_id": workspace, "disk": disk}, renderDisk(disk, "Created")+"\nUse it with `rzv sandboxes create --name <name> --disk "+display(disk.ID)+"`.")
	return nil
}

func disksAttach(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account", "sandbox"); err != nil {
		return err
	}
	if err := cmd.expectArgs(2, "rzv disks attach <disk-id> --sandbox <sandbox-id>"); err != nil {
		return err
	}
	id, err := diskID(cmd.args[1])
	if err != nil {
		return err
	}
	text, err := cmd.required("sandbox")
	if err != nil {
		return err
	}
	sandbox, err := sandboxID(text)
	if err != nil {
		return err
	}
	workspace, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	disk, err := client.DiskAttach(a.ctx, workspace, id, sandbox)
	if err != nil {
		return diskError(a, client, workspace, id, err)
	}
	a.output(cmd, map[string]any{"workspace_id": workspace, "disk": disk}, renderDisk(disk, "Attached")+"\nIt is mounted at "+display(disk.MountPath)+" from the sandbox's next start.")
	return nil
}
