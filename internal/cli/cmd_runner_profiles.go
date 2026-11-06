package cli

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/Runzivo/rzv-cli/internal/api"
)

const customRunnerProfilesUsage = `Usage:
  rzv runners profiles list [--workspace <id>] [--json]
  rzv runners profiles create --name <name> --label <rzv-label> --vcpu <n> --mem-gib <n>
      [--disk-gib <n>] [--workspace <id>] [--json]
  rzv runners profiles delete <profile-id> [--workspace <id>] [--json]

A custom runner profile is a vCPU, memory and temporary disk shape chosen
independently, saved under a label. Put its workflow label in runs-on.
--vcpu is 2, 4, 8, 16 or 32; --mem-gib 4 to 128; --disk-gib 16 to 256
(default 16, which is included). The label is rzv- then lowercase letters,
digits or single hyphens. delete archives the profile: jobs already admitted
keep their shape.

New profiles are limited by plan: up to 4 vCPU on Free and Developer, 16 on
Team, 32 on Business, custom on Enterprise. A larger one is refused with the
plan it needs (exit code 4); see "rzv runners availability".`

const runnerAvailabilityUsage = `Usage:
  rzv runners availability [--vcpu <n> --mem-gib <n> [--disk-gib <n>]] [--workspace <id>] [--json]

Without a shape, shows every custom runner profile. With --vcpu and
--mem-gib, shows that shape. Each answer is "Available now", "Busy - jobs
will queue" or "Not supported", overall and per region, with the
equivalent-minute rate. Shared CPU (overcommitted) means the shape has more
vCPU than the host's physical CPU.

The plan is shown apart from availability: "included", or the plan a shape
needs marked locked, with a table of every vCPU option and an upgrade hint.
Saved profiles keep working after a plan change.`

var profileIDPattern = regexp.MustCompile(`^rpf_[0-9a-f]{24}$`)

const runnerProfileNotFound = "That custom runner profile was not found in this workspace. List them with `rzv runners profiles list`."

func init() {
	commandUsage["runners profiles"] = customRunnerProfilesUsage
	commandUsage["runners availability"] = runnerAvailabilityUsage
}

func runCustomRunnerProfiles(a *app, cmd *command) error {
	if len(cmd.args) < 2 {
		return usageError("%s", customRunnerProfilesUsage)
	}
	switch cmd.args[1] {
	case "list":
		return runnerProfilesList(a, cmd)
	case "create":
		return runnerProfilesCreate(a, cmd)
	case "delete":
		return runnerProfilesDelete(a, cmd)
	default:
		return usageError("%s", customRunnerProfilesUsage)
	}
}

// runnerProfileError gives the profile-specific Core refusals clear words;
// anything else is apiError's.
func runnerProfileError(err error) error {
	var apiErr *api.Error
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.Status == 404 && apiErr.Code == "runner_profiles_disabled":
			return newError(apiErr.Code, exitUnavailable, "Custom runner profiles are not turned on for this Runzivo environment.")
		case apiErr.Status == 503 && apiErr.Code == "runner_profiles_unavailable":
			return newError(apiErr.Code, exitUnavailable, "Custom runner profiles are not available on this Runzivo environment.")
		case apiErr.Status == 404 && apiErr.Code == "runner_profile_not_found":
			return notFoundError("%s", runnerProfileNotFound)
		case apiErr.Status == 400 && apiErr.Code == "invalid_runner_profile":
			return newError(apiErr.Code, exitUsage, "Runzivo refused this profile or shape. Check the name, label, --vcpu, --mem-gib and --disk-gib (see `rzv help runners profiles`).")
		case apiErr.Status == 409 && apiErr.Code == "runner_profile_label_taken":
			return newError(apiErr.Code, exitFailure, "Another live profile in this workspace already uses that label.")
		case apiErr.Status == 409 && apiErr.Code == "runner_profile_limit":
			return newError(apiErr.Code, exitFailure, "This workspace already has the most live profiles it can keep (50). Delete one first.")
		case apiErr.Status == 429 && apiErr.Code == "runner_availability_rate_limited":
			return newError(apiErr.Code, exitFailure, "Too many availability checks for this workspace. Wait a minute and try again.")
		case apiErr.Status == 503 && apiErr.Code == "runner_availability_unavailable":
			return newError(apiErr.Code, exitUnavailable, "Runner availability is unavailable on this Runzivo environment.")
		}
	}
	return apiError(err, runnerProfileNotFound)
}

// runnerProfileCreateError maps a failed create. plan_required names the plan
// to move to, as for sandboxes; everything else is runnerProfileError's.
func (a *app) runnerProfileCreateError(err error, vcpu int64) error {
	var apiErr *api.Error
	if errors.As(err, &apiErr) && apiErr.Status == 403 && apiErr.Code == "plan_required" {
		return a.sandboxCreateError(err, fmt.Sprintf("A %d vCPU runner profile", vcpu))
	}
	return runnerProfileError(err)
}

func runnerProfilesList(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	if err := cmd.expectArgs(2, "rzv runners profiles list [--workspace <id>] [--json]"); err != nil {
		return err
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	list, err := client.CustomRunnerProfiles(a.ctx, id)
	if err != nil {
		return runnerProfileError(err)
	}
	a.output(cmd, map[string]any{"workspace_id": id, "profiles": list.Profiles, "included_disk_gib": list.IncludedDiskGiB}, renderRunnerProfiles(id, list.Profiles))
	return nil
}

func renderRunnerProfiles(workspaceID string, profiles []api.CustomRunnerProfile) string {
	if len(profiles) == 0 {
		return "No custom runner profiles in workspace " + display(workspaceID) + ". Create one with `rzv runners profiles create`."
	}
	lines := []string{"Custom runner profiles in workspace " + display(workspaceID) + ":"}
	for _, p := range profiles {
		lines = append(lines, fmt.Sprintf("- %s [%s] %s, runs-on: %s", display(p.Name), display(p.ID), shapeText(p.VCPU, p.MemMiB/1024, p.DiskGiB), display(p.WorkflowLabel)))
	}
	return strings.Join(lines, "\n")
}

func shapeText(vcpu, memGiB, diskGiB int64) string {
	return fmt.Sprintf("%d vCPU, %d GiB memory, %d GiB disk", vcpu, memGiB, diskGiB)
}

// shapeOptions reads --vcpu, --mem-gib and --disk-gib. present is false when
// none is given; --disk-gib defaults to the included 16 GiB.
func shapeOptions(cmd *command) (shape api.RunnerShape, present bool, err error) {
	number := func(name string) (int64, bool, error) {
		raw, err := cmd.option(name)
		if err != nil || raw == "" {
			return 0, false, err
		}
		value, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || value < 1 {
			return 0, false, usageError("--%s must be a whole number.", name)
		}
		return value, true, nil
	}
	vcpu, hasVCPU, err := number("vcpu")
	if err != nil {
		return shape, false, err
	}
	mem, hasMem, err := number("mem-gib")
	if err != nil {
		return shape, false, err
	}
	disk, hasDisk, err := number("disk-gib")
	if err != nil {
		return shape, false, err
	}
	if !hasVCPU && !hasMem && !hasDisk {
		return shape, false, nil
	}
	if !hasVCPU || !hasMem {
		return shape, false, usageError("A shape needs both --vcpu and --mem-gib.")
	}
	if !hasDisk {
		disk = 16
	}
	return api.RunnerShape{VCPU: vcpu, MemGiB: mem, DiskGiB: disk}, true, nil
}

func runnerProfilesCreate(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account", "name", "label", "vcpu", "mem-gib", "disk-gib"); err != nil {
		return err
	}
	if err := cmd.expectArgs(2, "rzv runners profiles create --name <name> --label <rzv-label> --vcpu <n> --mem-gib <n> [--disk-gib <n>]"); err != nil {
		return err
	}
	name, err := cmd.required("name")
	if err != nil {
		return err
	}
	label, err := cmd.required("label")
	if err != nil {
		return err
	}
	shape, present, err := shapeOptions(cmd)
	if err != nil {
		return err
	}
	if !present {
		return usageError("rzv runners profiles create needs --vcpu and --mem-gib.")
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	profile, err := client.CreateCustomRunnerProfile(a.ctx, id, api.CreateCustomRunnerProfileInput{Name: name, Label: label, VCPU: shape.VCPU, MemGiB: shape.MemGiB, DiskGiB: shape.DiskGiB})
	if err != nil {
		return a.runnerProfileCreateError(err, shape.VCPU)
	}
	a.output(cmd, map[string]any{"workspace_id": id, "profile": profile},
		fmt.Sprintf("Created custom runner profile %s [%s]: %s.\nUse runs-on: %s", display(profile.Name), display(profile.ID), shapeText(profile.VCPU, profile.MemMiB/1024, profile.DiskGiB), display(profile.WorkflowLabel)))
	return nil
}

func runnerProfilesDelete(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	if err := cmd.expectArgs(3, "rzv runners profiles delete <profile-id>"); err != nil {
		return err
	}
	if !profileIDPattern.MatchString(cmd.args[2]) {
		return usageError("<profile-id> is a profile id such as rpf_0123456789abcdef01234567. See `rzv runners profiles list`.")
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	profileID := cmd.args[2]
	if err := client.ArchiveCustomRunnerProfile(a.ctx, id, profileID); err != nil {
		return runnerProfileError(err)
	}
	a.output(cmd, map[string]any{"workspace_id": id, "archived_profile_id": profileID}, "Archived custom runner profile "+display(profileID)+". Jobs already admitted keep their shape.")
	return nil
}

func runRunnerAvailability(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account", "vcpu", "mem-gib", "disk-gib"); err != nil {
		return err
	}
	if err := cmd.expectArgs(1, "rzv runners availability [--vcpu <n> --mem-gib <n> [--disk-gib <n>]]"); err != nil {
		return err
	}
	shape, present, err := shapeOptions(cmd)
	if err != nil {
		return err
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	var query *api.RunnerShape
	if present {
		query = &shape
	}
	answer, err := client.CustomRunnerProfileAvailability(a.ctx, id, query)
	if err != nil {
		return runnerProfileError(err)
	}
	result := map[string]any{"workspace_id": id, "shapes": answer.Shapes}
	if len(answer.VCPUOptions) > 0 {
		result["vcpu_options"] = answer.VCPUOptions
	}
	a.output(cmd, result, a.renderAvailability(id, answer))
	return nil
}

var availabilityStatusText = map[string]string{
	"available":     "Available now",
	"busy":          "Busy - jobs will queue",
	"not_supported": "Not supported",
}

var availabilityRegionText = map[string]string{
	"available":     "available now",
	"busy":          "busy",
	"not_supported": "not supported in this region",
}

var availabilityReasonText = map[string]string{
	"workspace_held":    "your workspace is on hold, so new jobs do not start",
	"concurrency_limit": "your workspace is running its concurrent job limit",
}

func statusText(table map[string]string, status string) string {
	if text, ok := table[status]; ok {
		return text
	}
	return display(status)
}

func (a *app) renderAvailability(workspaceID string, availability *api.CustomRunnerAvailability) string {
	answers := availability.Shapes
	lines := []string{"Runner availability for workspace " + display(workspaceID) + ":"}
	if len(answers) == 0 {
		lines = []string{"No custom runner profiles in workspace " + display(workspaceID) + ". Check a shape with --vcpu and --mem-gib."}
	}
	for _, answer := range answers {
		name := shapeText(answer.VCPU, answer.MemGiB, answer.DiskGiB)
		if answer.WorkflowLabel != "" {
			name = display(answer.WorkflowLabel) + " (" + name + ")"
		}
		status := statusText(availabilityStatusText, answer.Status)
		if answer.Reason == "unpriced" {
			status = "Not available yet: pricing not enabled"
		} else if reason, ok := availabilityReasonText[answer.Reason]; ok {
			status += " (" + reason + ")"
		}
		if answer.Overcommitted {
			status += ", shared CPU (overcommitted)"
		}
		rate := "rate -"
		if answer.Rate != "" {
			rate = display(answer.Rate) + " equivalent minutes per minute"
		}
		line := fmt.Sprintf("- %s: %s; %s", name, status, rate)
		if answer.PlanEligible != nil {
			line += "; plan: " + planColumn(answer.Locked(), answer.RequiredPlan)
		}
		lines = append(lines, line)
		for _, region := range answer.Regions {
			text := statusText(availabilityRegionText, region.Status)
			if region.Overcommitted {
				text += ", shared CPU"
			}
			lines = append(lines, fmt.Sprintf("    %s: %s", display(region.Region), text))
		}
	}
	out := strings.Join(lines, "\n")
	if table := vcpuPlanTable(availability.VCPUOptions); table != "" {
		out += "\n\n" + table
	}
	return out + a.lockedHints(availability)
}

// vcpuPlanTable lists every vCPU option with the plan it needs, locked ones
// included. It is empty from a server that sends no options.
func vcpuPlanTable(options []api.CustomRunnerVCPUOption) string {
	if len(options) == 0 {
		return ""
	}
	var out strings.Builder
	table := tabwriter.NewWriter(&out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "VCPU\tPLAN")
	for _, o := range options {
		fmt.Fprintf(table, "%d\t%s\n", o.VCPU, planColumn(o.Locked(), o.RequiredPlan))
	}
	_ = table.Flush()
	return strings.TrimRight(out.String(), "\n")
}

// lockedHints are the lines under the output for what the plan locks: the
// vCPU options when the server sends them, else the shapes. Enterprise-only
// sizes say to contact Runzivo; the rest say where to upgrade.
func (a *app) lockedHints(availability *api.CustomRunnerAvailability) string {
	type locked struct {
		name         string
		requiredPlan *string
	}
	var found []locked
	if len(availability.VCPUOptions) > 0 {
		for _, o := range availability.VCPUOptions {
			if o.Locked() {
				found = append(found, locked{fmt.Sprintf("%d vCPU", o.VCPU), o.RequiredPlan})
			}
		}
	} else {
		for _, s := range availability.Shapes {
			if s.Locked() {
				name := fmt.Sprintf("%d vCPU", s.VCPU)
				if s.WorkflowLabel != "" {
					name = display(s.WorkflowLabel)
				}
				found = append(found, locked{name, s.RequiredPlan})
			}
		}
	}
	var upgrade, enterprise []string
	for _, l := range found {
		switch {
		case l.requiredPlan != nil && *l.requiredPlan == "enterprise":
			enterprise = append(enterprise, l.name+" needs the Enterprise plan")
		case l.requiredPlan != nil && *l.requiredPlan != "":
			upgrade = append(upgrade, l.name+" needs the "+planLabel(*l.requiredPlan)+" plan")
		default:
			upgrade = append(upgrade, l.name+" is not in your plan")
		}
	}
	var out strings.Builder
	if len(upgrade) > 0 {
		fmt.Fprintf(&out, "\nLocked: %s. %s", strings.Join(upgrade, "; "), a.upgradeHint())
	}
	if len(enterprise) > 0 {
		fmt.Fprintf(&out, "\nLocked: %s. Contact Runzivo.", strings.Join(enterprise, "; "))
	}
	return out.String()
}
