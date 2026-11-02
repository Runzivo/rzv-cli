package cli

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/Runzivo/rzv-cli/internal/api"
)

const disksUsage = `Usage:
  rzv disks list [--workspace <id>] [--json]
  rzv disks get <disk-id> [--workspace <id>] [--json]
  rzv disks create --name <name> --size-gib <1-64> [--idempotency-key <key>] [--workspace <id>] [--json]
  rzv disks attach <disk-id> --sandbox <sandbox-id> [--workspace <id>] [--json]
  rzv disks detach <disk-id> [--workspace <id>] [--json]
  rzv disks delete <disk-id> [--workspace <id>] [--json]

A persistent disk keeps files after a sandbox ends. Create a sandbox with it,
"rzv sandboxes create --name dev --disk <disk-id>", and it is mounted at /mnt/disk
while the sandbox runs. The disk lives on one Runzivo server: a sandbox that uses
it runs on that server or waits for it. It is not copied to another server and is
not a backup. One sandbox can use a disk at a time; stop the sandbox before you
detach the disk or attach it to another stopped sandbox. Deleting a disk removes
its files for good.

Persistent disks are off unless Runzivo has turned them on for your environment
and given your workspace a disk quota. No disk price is published and disk
charges are off. Disks use the sandboxes:read and sandboxes:write permissions.`

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
	a.output(cmd, map[string]any{"workspace_id": workspace, "disks": listing.Disks, "quota": listing.Quota}, renderDiskList(workspace, listing))
	return nil
}

func renderDiskQuota(quota api.DiskQuota) string {
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
	out.WriteString("\n" + renderDiskQuota(listing.Quota))
	return out.String()
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
		return apiError(err, "")
	}
	if verb == "Deleting" && disk.Status == "deleted" {
		verb = "Deleted"
	}
	a.output(cmd, map[string]any{"workspace_id": workspace, "disk": disk}, renderDisk(disk, verb))
	return nil
}

func disksCreate(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account", "name", "size-gib", "idempotency-key"); err != nil {
		return err
	}
	usage := "rzv disks create --name <name> --size-gib <1-64>"
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
	if err != nil || size < 1 || size > 64 {
		return usageError("--size-gib must be a whole number from 1 to 64.")
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
		return apiError(err, "")
	}
	a.output(cmd, map[string]any{"workspace_id": workspace, "disk": disk}, renderDisk(disk, "Attached")+"\nIt is mounted at "+display(disk.MountPath)+" from the sandbox's next start.")
	return nil
}
