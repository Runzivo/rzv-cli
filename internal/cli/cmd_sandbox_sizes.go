package cli

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/Runzivo/rzv-cli/internal/api"
	"github.com/Runzivo/rzv-cli/internal/config"
)

// upgradePath is the portal page where a workspace changes its plan.
const upgradePath = "/settings?tab=billing"

// planLabels are the plan names as people read them. An unknown plan from a
// newer server is shown as the server sends it.
var planLabels = map[string]string{
	"free": "Free", "developer": "Developer", "team": "Team", "business": "Business", "enterprise": "Enterprise",
}

func planLabel(plan string) string {
	if label, ok := planLabels[plan]; ok {
		return label
	}
	return display(plan)
}

// portalURL is the portal origin that matches the API this run talks to, or
// "" when none is known (a custom RZV_API_URL, for one). It never assumes the
// production portal.
func (a *app) portalURL() string {
	base := ""
	if a.apiKey() != "" {
		base, _, _ = a.apiKeyTarget()
	} else if a.Store != nil {
		if creds, _ := a.Store.Read(); creds != nil {
			base = creds.APIBaseURL
		}
	}
	if base == "" {
		if env, err := config.LookupEnvironment(a.Getenv("RZV_ENV")); err == nil {
			return env.PortalURL
		}
		return ""
	}
	base = strings.TrimRight(base, "/")
	for _, name := range config.EnvironmentNames() {
		if env, err := config.LookupEnvironment(name); err == nil && env.APIBaseURL == base {
			return env.PortalURL
		}
	}
	return ""
}

// upgradeHint is the sentence that tells where to change the plan.
func (a *app) upgradeHint() string {
	if portal := a.portalURL(); portal != "" {
		return "Upgrade: " + portal + upgradePath
	}
	return "Upgrade in the Runzivo portal under Settings, Billing."
}

// sandboxCreateError maps a failed create. plan_required names the plan to
// move to; everything else goes through apiError.
func (a *app) sandboxCreateError(err error, size string) error {
	var apiErr *api.Error
	if !errors.As(err, &apiErr) || apiErr.Status != 403 || apiErr.Code != "plan_required" {
		return apiError(err, "")
	}
	switch apiErr.RequiredPlan {
	case "enterprise":
		return newError(apiErr.Code, exitAccessDenied, "%s needs the Enterprise plan. Contact Runzivo.", display(size))
	case "":
		return newError(apiErr.Code, exitAccessDenied, "%s is not included in this workspace's plan. %s", display(size), a.upgradeHint())
	}
	return newError(apiErr.Code, exitAccessDenied, "%s needs the %s plan. %s", display(size), planLabel(apiErr.RequiredPlan), a.upgradeHint())
}

func sandboxesSizes(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	if err := cmd.expectArgs(1, "rzv sandboxes sizes [--workspace <id>] [--json]"); err != nil {
		return err
	}
	workspace, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	listing, err := client.SandboxList(a.ctx, workspace)
	if err != nil {
		return apiError(err, "")
	}
	rows := sizeRows(listing)
	a.output(cmd, map[string]any{"workspace_id": workspace, "size_availability": rows}, a.renderSandboxSizes(rows))
	return nil
}

// sizeRows are the size_availability entries, or, from a server that sends
// none, one row per size in the sizes list with the plan fields unknown.
func sizeRows(listing *api.SandboxListing) []api.SandboxSize {
	if len(listing.SizeAvailability) > 0 {
		return listing.SizeAvailability
	}
	rows := make([]api.SandboxSize, 0, len(listing.Sizes))
	for _, size := range sandboxSizes {
		if slices.Contains(listing.Sizes, size) {
			rows = append(rows, api.SandboxSize{Size: size, Status: "available"})
		}
	}
	return rows
}

func sizeRAM(s api.SandboxSize) string {
	if s.MemMiB == nil {
		return "-"
	}
	gib := float64(*s.MemMiB) / 1024
	return strconv.FormatFloat(gib, 'f', -1, 64) + " GiB"
}

func sizeVCPU(s api.SandboxSize) string {
	if s.VCPU == nil {
		return "-"
	}
	return strconv.Itoa(*s.VCPU)
}

func sizeRate(s api.SandboxSize) string {
	if s.EquivalentMinutesPerMinute == "" {
		return "-"
	}
	return display(s.EquivalentMinutesPerMinute)
}

// sizePlan is "included" for a size the plan allows. A locked size names the
// plan it needs and is marked locked.
func sizePlan(s api.SandboxSize) string {
	if !s.Locked() {
		return "included"
	}
	if s.RequiredPlan == nil || *s.RequiredPlan == "" {
		return "locked"
	}
	return planLabel(*s.RequiredPlan) + " (locked)"
}

// sizeStatus is the regional status, with the reason a size is not available.
func sizeStatus(s api.SandboxSize) string {
	status := display(s.Status)
	switch s.Reason {
	case "":
		return status
	case "pool_full":
		return status + " (all pools full now)"
	case "no_host_fits":
		return status + " (no host fits)"
	case "not_rated":
		return status + " (not rated)"
	}
	return status + " (" + display(s.Reason) + ")"
}

func (a *app) renderSandboxSizes(rows []api.SandboxSize) string {
	if len(rows) == 0 {
		return "No sandbox sizes are on offer here."
	}
	var out strings.Builder
	table := tabwriter.NewWriter(&out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "SIZE\tVCPU\tRAM\tRATE (eq-min/min)\tPLAN\tAVAILABILITY")
	for _, s := range rows {
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%s\n", display(s.Size), sizeVCPU(s), sizeRAM(s), sizeRate(s), sizePlan(s), sizeStatus(s))
	}
	_ = table.Flush()
	var locked, enterprise []string
	for _, s := range rows {
		switch {
		case !s.Locked():
		case s.RequiredPlan != nil && *s.RequiredPlan == "enterprise":
			enterprise = append(enterprise, display(s.Size)+" needs the Enterprise plan")
		default:
			text := display(s.Size)
			if s.RequiredPlan != nil && *s.RequiredPlan != "" {
				text += " needs the " + planLabel(*s.RequiredPlan) + " plan"
			} else {
				text += " is not in your plan"
			}
			locked = append(locked, text)
		}
	}
	if len(locked) > 0 {
		fmt.Fprintf(&out, "\nLocked: %s. %s", strings.Join(locked, "; "), a.upgradeHint())
	}
	if len(enterprise) > 0 {
		fmt.Fprintf(&out, "\nLocked: %s. Contact Runzivo.", strings.Join(enterprise, "; "))
	}
	return strings.TrimRight(out.String(), "\n")
}
