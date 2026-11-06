package cli

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Runzivo/rzv-cli/internal/api"
	"github.com/Runzivo/rzv-cli/internal/config"
)

const sizesPath = "GET /v1/customer/tenants/t-1/sandboxes"

// plansListing is a Free workspace: 2vcpu-8gb is included, 4vcpu-16gb is
// included but busy, 8vcpu-32gb needs Team, 16vcpu-64gb is a custom limit.
const plansListing = `{"sandboxes":[],"max_running":2,"sizes":["2vcpu-8gb","4vcpu-16gb","8vcpu-32gb"],"lifetimes":[8],"size_availability":[
{"size":"2vcpu-8gb","status":"available","vcpu":2,"mem_mib":8192,"equivalent_minutes_per_minute":"2","plan_eligible":true,"required_plan":"free"},
{"size":"4vcpu-16gb","status":"busy","reason":"pool_full","vcpu":4,"mem_mib":16384,"equivalent_minutes_per_minute":"4","plan_eligible":true,"required_plan":"free"},
{"size":"8vcpu-32gb","status":"available","vcpu":8,"mem_mib":32768,"equivalent_minutes_per_minute":"8","plan_eligible":false,"required_plan":"team"},
{"size":"16vcpu-64gb","status":"unsupported","reason":"no_host_fits","vcpu":16,"mem_mib":65536,"equivalent_minutes_per_minute":"16","plan_eligible":false,"required_plan":"enterprise"}]}`

func TestSandboxSizesTableShowsLockedBusyAndIncluded(t *testing.T) {
	h := selectedHarness(t)
	h.json(sizesPath, 200, plansListing)
	code, out, errOut := h.run("sandboxes", "sizes")
	if code != 0 || errOut != "" {
		t.Fatalf("sizes: %d %q %q", code, out, errOut)
	}
	lines := strings.Split(out, "\n")
	if len(lines) < 7 {
		t.Fatalf("sizes output too short: %q", out)
	}
	for i, fields := range [][]string{
		{"SIZE", "VCPU", "RAM", "RATE", "(eq-min/min)", "PLAN", "AVAILABILITY"},
		{"2vcpu-8gb", "2", "8", "GiB", "2", "included", "available"},
		{"4vcpu-16gb", "4", "16", "GiB", "4", "included", "busy", "(all", "pools", "full", "now)"},
		{"8vcpu-32gb", "8", "32", "GiB", "8", "Team", "(locked)", "available"},
		{"16vcpu-64gb", "16", "64", "GiB", "16", "Enterprise", "(locked)", "unsupported", "(no", "host", "fits)"},
	} {
		if got := strings.Fields(lines[i]); strings.Join(got, " ") != strings.Join(fields, " ") {
			t.Errorf("row %d = %q, want fields %q", i, lines[i], fields)
		}
	}
	// The test harness API is a loopback address, not a Runzivo environment,
	// so the hint names the portal page instead of a link.
	for _, text := range []string{
		"Locked: 8vcpu-32gb needs the Team plan. Upgrade in the Runzivo portal under Settings, Billing.",
		"Locked by a custom limit on this workspace: 16vcpu-64gb. Contact Runzivo to change it.",
	} {
		if !strings.Contains(out, text) {
			t.Errorf("sizes output lacks %q:\n%s", text, out)
		}
	}
}

func TestSandboxSizesUnlockedHasNoHint(t *testing.T) {
	h := selectedHarness(t)
	h.json(sizesPath, 200, `{"sandboxes":[],"max_running":2,"sizes":["2vcpu-8gb"],"lifetimes":[8],"size_availability":[
{"size":"2vcpu-8gb","status":"available","vcpu":2,"mem_mib":8192,"equivalent_minutes_per_minute":"2","plan_eligible":true,"required_plan":null}]}`)
	code, out, _ := h.run("sandboxes", "sizes")
	if code != 0 || strings.Contains(out, "Locked") || strings.Contains(out, "Upgrade") || !strings.Contains(out, "included") {
		t.Fatalf("unlocked: %d %q", code, out)
	}
}

func TestSandboxSizesOldServerReadsAsEligibleAndUnknown(t *testing.T) {
	h := selectedHarness(t)
	h.json(sizesPath, 200, `{"sandboxes":[],"max_running":2,"sizes":["2vcpu-8gb","4vcpu-16gb"]}`)
	code, out, _ := h.run("sandboxes", "sizes")
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if code != 0 || len(lines) != 3 || strings.Contains(out, "Locked") {
		t.Fatalf("old server: %d %q", code, out)
	}
	if got := strings.Fields(lines[1]); strings.Join(got, " ") != "2vcpu-8gb - - - included available" {
		t.Errorf("old server row = %q", lines[1])
	}
	code, out, _ = h.run("sandboxes", "sizes", "--json")
	var got struct {
		Rows []map[string]any `json:"size_availability"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &got) != nil || len(got.Rows) != 2 || got.Rows[0]["size"] != "2vcpu-8gb" {
		t.Fatalf("old server json: %d %q", code, out)
	}
	if _, has := got.Rows[0]["plan_eligible"]; has {
		t.Errorf("an old server's row invented plan_eligible: %v", got.Rows[0])
	}
	// list keeps its old JSON shape: no size_availability from an old server.
	_, out, _ = h.run("sandboxes", "list", "--json")
	if strings.Contains(out, "size_availability") {
		t.Errorf("list --json invented size_availability: %q", out)
	}
}

func TestSandboxSizesJSONPassesPlanFieldsThrough(t *testing.T) {
	h := selectedHarness(t)
	h.json(sizesPath, 200, plansListing)
	for _, args := range [][]string{{"sandboxes", "sizes", "--json"}, {"sandboxes", "list", "--json"}} {
		code, out, _ := h.run(args...)
		var got struct {
			Rows []struct {
				Size         string  `json:"size"`
				Status       string  `json:"status"`
				Reason       string  `json:"reason"`
				VCPU         int     `json:"vcpu"`
				MemMiB       int     `json:"mem_mib"`
				Rate         string  `json:"equivalent_minutes_per_minute"`
				PlanEligible bool    `json:"plan_eligible"`
				RequiredPlan *string `json:"required_plan"`
			} `json:"size_availability"`
		}
		if code != 0 || json.Unmarshal([]byte(out), &got) != nil || len(got.Rows) != 4 {
			t.Fatalf("%v: %d %q", args, code, out)
		}
		busy, locked := got.Rows[1], got.Rows[2]
		if busy.Status != "busy" || busy.Reason != "pool_full" || !busy.PlanEligible || busy.VCPU != 4 || busy.MemMiB != 16384 || busy.Rate != "4" {
			t.Errorf("%v: busy row = %+v", args, busy)
		}
		if locked.PlanEligible || locked.RequiredPlan == nil || *locked.RequiredPlan != "team" {
			t.Errorf("%v: locked row = %+v", args, locked)
		}
	}
	// While the setting is off, required_plan stays null.
	h.json(sizesPath, 200, `{"sandboxes":[],"max_running":2,"sizes":["2vcpu-8gb"],"size_availability":[{"size":"2vcpu-8gb","status":"available","vcpu":2,"mem_mib":8192,"equivalent_minutes_per_minute":"2","plan_eligible":true,"required_plan":null}]}`)
	_, out, _ := h.run("sandboxes", "sizes", "--json")
	if !strings.Contains(out, `"required_plan": null`) && !strings.Contains(out, `"required_plan":null`) {
		t.Errorf("null required_plan lost: %q", out)
	}
}

func TestSandboxCreatePlanRequiredNamesThePlan(t *testing.T) {
	h := selectedHarness(t)
	cases := []struct {
		body string
		exit int
		want []string
	}{
		{`{"error":{"code":"plan_required","required_plan":"team","message":"x","message_fr_CA":"y"}}`, exitAccessDenied, []string{"16vcpu-64gb needs the Team plan.", "Upgrade"}},
		{`{"error":{"code":"plan_required","required_plan":"enterprise"}}`, exitAccessDenied, []string{"16vcpu-64gb is above the limit set for this workspace", "Contact Runzivo"}},
		{`{"error":{"code":"plan_required","required_plan":null}}`, exitAccessDenied, []string{"16vcpu-64gb is not included in this workspace's plan"}},
	}
	for _, c := range cases {
		h.json("POST /v1/customer/tenants/t-1/sandboxes", 403, c.body)
		code, out, errOut := h.run("sandboxes", "create", "--name", "big", "--size", "16vcpu-64gb")
		if code != c.exit || out != "" {
			t.Errorf("%s: exit %d out %q err %q", c.body, code, out, errOut)
		}
		for _, text := range c.want {
			if !strings.Contains(errOut, text) {
				t.Errorf("%s: stderr %q lacks %q", c.body, errOut, text)
			}
		}
		if strings.Contains(errOut, "not authorized") {
			t.Errorf("%s: raw 403 message leaked: %q", c.body, errOut)
		}
	}
}

func TestPlanRequiredUpgradeLinkFollowsTheEnvironment(t *testing.T) {
	for env, portal := range map[string]string{"prd": "https://app.runzivo.dev", "shared": "https://shared-app.runzivo.dev"} {
		preset, err := config.LookupEnvironment(env)
		if err != nil || preset.PortalURL != portal {
			t.Fatalf("preset %s: %+v %v", env, preset, err)
		}
		a := &app{ctx: context.Background(), Deps: Deps{
			Getenv: func(k string) string { return map[string]string{"RZV_API_KEY": "key", "RZV_ENV": env}[k] },
			Store:  &config.Store{Dir: t.TempDir()},
		}}
		err = a.sandboxCreateError(&api.Error{Status: 403, Code: "plan_required", RequiredPlan: "team"}, "16vcpu-64gb")
		want := "16vcpu-64gb needs the Team plan. Upgrade: " + portal + "/settings?tab=billing"
		if err == nil || err.Error() != want {
			t.Errorf("%s: %v, want %q", env, err, want)
		}
	}
	// A custom API address is not a known environment: no guessed link.
	a := &app{ctx: context.Background(), Deps: Deps{
		Getenv: func(k string) string {
			return map[string]string{"RZV_API_KEY": "key", "RZV_API_URL": "https://api.example.test"}[k]
		},
		Store: &config.Store{Dir: t.TempDir()},
	}}
	err := a.sandboxCreateError(&api.Error{Status: 403, Code: "plan_required", RequiredPlan: "business"}, "8vcpu-32gb")
	if err == nil || strings.Contains(err.Error(), "http") || !strings.Contains(err.Error(), "needs the Business plan") {
		t.Errorf("custom API: %v", err)
	}
	// Other errors still go through the common mapping.
	if err := a.sandboxCreateError(&api.Error{Status: 403, Code: "permission_required"}, "2vcpu-8gb"); err == nil || !strings.Contains(err.Error(), "does not allow") {
		t.Errorf("permission_required: %v", err)
	}
}
