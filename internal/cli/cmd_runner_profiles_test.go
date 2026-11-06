package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

const runnerProfileJSON = `{"id":"rpf_0123456789abcdef01234567","name":"Wide","label":"rzv-wide","workflow_label":"env-rzv-wide","vcpu":32,"mem_mib":16384,"disk_gib":64,"created_at":"2026-11-05T10:00:00Z"}`

func TestRunnerProfiles(t *testing.T) {
	h := selectedHarness(t)
	h.json("GET /v1/customer/tenants/t-1/custom-runner-profiles", 200, `{"profiles":[`+runnerProfileJSON+`],"included_disk_gib":16}`)
	code, out, _ := h.run("runners", "profiles", "list")
	if code != 0 || !strings.Contains(out, "- Wide [rpf_0123456789abcdef01234567] 32 vCPU, 16 GiB memory, 64 GiB disk, runs-on: env-rzv-wide") {
		t.Fatalf("list: %d %q", code, out)
	}
	code, out, _ = h.run("runners", "profiles", "list", "--json")
	var listed struct {
		Profiles []struct {
			WorkflowLabel string `json:"workflow_label"`
		} `json:"profiles"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &listed) != nil || len(listed.Profiles) != 1 || listed.Profiles[0].WorkflowLabel != "env-rzv-wide" {
		t.Fatalf("list --json: %d %q", code, out)
	}
	h.json("GET /v1/customer/tenants/t-1/custom-runner-profiles", 404, `{"error":{"code":"runner_profiles_disabled"}}`)
	if code, _, errOut := h.run("runners", "profiles", "list"); code == 0 || !strings.Contains(errOut, "not turned on") {
		t.Fatalf("disabled: %d %q", code, errOut)
	}

	var bodies []map[string]any
	h.routes["POST /v1/customer/tenants/t-1/custom-runner-profiles"] = func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(data, &body)
		bodies = append(bodies, body)
		w.WriteHeader(201)
		_, _ = w.Write([]byte(runnerProfileJSON))
	}
	code, out, _ = h.run("runners", "profiles", "create", "--name", "Wide", "--label", "rzv-wide", "--vcpu", "32", "--mem-gib", "16", "--disk-gib", "64")
	if code != 0 || !strings.Contains(out, "Use runs-on: env-rzv-wide") {
		t.Fatalf("create: %d %q", code, out)
	}
	if b := bodies[0]; b["name"] != "Wide" || b["label"] != "rzv-wide" || b["vcpu"] != float64(32) || b["mem_gib"] != float64(16) || b["disk_gib"] != float64(64) || len(b) != 5 {
		t.Fatalf("create body = %v", b)
	}
	// --disk-gib defaults to the included 16 GiB.
	if code, _, _ := h.run("runners", "profiles", "create", "--name", "Small", "--label", "rzv-small", "--vcpu", "2", "--mem-gib", "4"); code != 0 || bodies[1]["disk_gib"] != float64(16) {
		t.Fatalf("default disk: %d %v", code, bodies[1])
	}
	for _, args := range [][]string{
		{"runners", "profiles", "create", "--label", "rzv-x1", "--vcpu", "2", "--mem-gib", "4"},
		{"runners", "profiles", "create", "--name", "X", "--vcpu", "2", "--mem-gib", "4"},
		{"runners", "profiles", "create", "--name", "X", "--label", "rzv-x1", "--vcpu", "2"},
		{"runners", "profiles", "create", "--name", "X", "--label", "rzv-x1", "--vcpu", "two", "--mem-gib", "4"},
		{"runners", "profiles", "delete"},
		{"runners", "profiles", "delete", "wide"},
		{"runners", "profiles", "bogus"},
		{"runners", "profiles"},
	} {
		if code, _, _ := h.run(args...); code != exitUsage {
			t.Errorf("%v accepted: %d", args, code)
		}
	}
	if len(bodies) != 2 {
		t.Fatalf("a usage error reached the API: %v", bodies)
	}
	h.json("POST /v1/customer/tenants/t-1/custom-runner-profiles", 409, `{"error":{"code":"runner_profile_label_taken"}}`)
	if code, _, errOut := h.run("runners", "profiles", "create", "--name", "Wide", "--label", "rzv-wide", "--vcpu", "32", "--mem-gib", "16"); code == 0 || !strings.Contains(errOut, "already uses that label") {
		t.Fatalf("label taken: %d %q", code, errOut)
	}
	h.json("POST /v1/customer/tenants/t-1/custom-runner-profiles", 400, `{"error":{"code":"invalid_runner_profile","detail":"vcpu must be 2, 4, 8, 16 or 32"}}`)
	if code, _, errOut := h.run("runners", "profiles", "create", "--name", "Odd", "--label", "rzv-odd", "--vcpu", "6", "--mem-gib", "16"); code != exitUsage || !strings.Contains(errOut, "refused this profile") {
		t.Fatalf("invalid: %d %q", code, errOut)
	}

	h.routes["DELETE /v1/customer/tenants/t-1/custom-runner-profiles/rpf_0123456789abcdef01234567"] = func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}
	if code, out, _ := h.run("runners", "profiles", "delete", "rpf_0123456789abcdef01234567"); code != 0 || !strings.Contains(out, "Archived custom runner profile rpf_0123456789abcdef01234567") {
		t.Fatalf("delete: %d %q", code, out)
	}
	h.json("DELETE /v1/customer/tenants/t-1/custom-runner-profiles/rpf_ffffffffffffffffffffffff", 404, `{"error":{"code":"runner_profile_not_found"}}`)
	if code, _, errOut := h.run("runners", "profiles", "delete", "rpf_ffffffffffffffffffffffff"); code != exitNotFound || !strings.Contains(errOut, "was not found") {
		t.Fatalf("missing: %d %q", code, errOut)
	}
}

func TestRunnerAvailability(t *testing.T) {
	h := selectedHarness(t)
	var queries []string
	h.routes["GET /v1/customer/tenants/t-1/custom-runner-profiles/availability"] = func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"shapes":[{"profile_id":"rpf_0123456789abcdef01234567","workflow_label":"env-rzv-wide","equivalent_minutes_per_minute":"12.5","vcpu":32,"mem_gib":16,"disk_gib":64,"status":"available","overcommitted":true,"regions":[{"region":"region-a","status":"available","overcommitted":true},{"region":"region-b","status":"not_supported","overcommitted":false}]}]}`))
	}
	code, out, _ := h.run("runners", "availability")
	if code != 0 || !strings.Contains(out, "- env-rzv-wide (32 vCPU, 16 GiB memory, 64 GiB disk): Available now, shared CPU (overcommitted); 12.5 equivalent minutes per minute") ||
		!strings.Contains(out, "    region-a: available now, shared CPU") || !strings.Contains(out, "    region-b: not supported in this region") {
		t.Fatalf("availability: %d %q", code, out)
	}
	if code, _, _ := h.run("runners", "availability", "--vcpu", "8", "--mem-gib", "32"); code != 0 {
		t.Fatalf("shape: %d", code)
	}
	if len(queries) != 2 || queries[0] != "" || queries[1] != "disk_gib=16&mem_gib=32&vcpu=8" {
		t.Fatalf("queries = %q", queries)
	}
	for _, args := range [][]string{
		{"runners", "availability", "--vcpu", "8"},
		{"runners", "availability", "--disk-gib", "32"},
		{"runners", "availability", "extra"},
		{"runners", "availability", "--region", "region-b"},
	} {
		if code, _, _ := h.run(args...); code != exitUsage {
			t.Errorf("%v accepted: %d", args, code)
		}
	}

	h.json("GET /v1/customer/tenants/t-1/custom-runner-profiles/availability", 200, `{"shapes":[{"vcpu":2,"mem_gib":4,"disk_gib":16,"status":"not_supported","reason":"unpriced","overcommitted":false,"regions":[]},{"vcpu":4,"mem_gib":8,"disk_gib":16,"equivalent_minutes_per_minute":"1.75","status":"busy","reason":"concurrency_limit","overcommitted":false,"regions":[]}]}`)
	code, out, _ = h.run("runners", "availability")
	if code != 0 || !strings.Contains(out, "Not available yet: pricing not enabled; rate -") || !strings.Contains(out, "Busy - jobs will queue (your workspace is running its concurrent job limit); 1.75") {
		t.Fatalf("reasons: %d %q", code, out)
	}
	h.json("GET /v1/customer/tenants/t-1/custom-runner-profiles/availability", 429, `{"error":{"code":"runner_availability_rate_limited"}}`)
	if code, _, errOut := h.run("runners", "availability"); code == 0 || !strings.Contains(errOut, "Too many availability checks") {
		t.Fatalf("rate limited: %d %q", code, errOut)
	}
	h.json("GET /v1/customer/tenants/t-1/custom-runner-profiles/availability", 200, `{"shapes":[{"vcpu":2,"mem_gib":4,"disk_gib":16}]}`)
	if code, _, _ := h.run("runners", "availability"); code == 0 {
		t.Fatal("an answer without a status was accepted")
	}
}

func TestRunnerProfilesHelp(t *testing.T) {
	h := newHarness(t)
	if code, out, _ := h.run("help", "runners", "profiles"); code != 0 || !strings.Contains(out, "rzv runners profiles create --name") {
		t.Fatalf("help runners profiles: %d %q", code, out)
	}
	if code, out, _ := h.run("runners", "availability", "--help"); code != 0 || !strings.Contains(out, "rzv runners availability [--vcpu") {
		t.Fatalf("runners availability --help: %d %q", code, out)
	}
	if code, out, _ := h.run("help", "runners"); code != 0 || !strings.Contains(out, "rzv runners <repository-id>") {
		t.Fatalf("help runners: %d %q", code, out)
	}
}

func TestRunnerProfilesRefusalsAndJSON(t *testing.T) {
	h := selectedHarness(t)
	base := "/v1/customer/tenants/t-1/custom-runner-profiles"
	item := base + "/rpf_0123456789abcdef01234567"
	createArgs := []string{"runners", "profiles", "create", "--name", "Wide", "--label", "rzv-wide", "--vcpu", "32", "--mem-gib", "16"}

	h.json("POST "+base, 201, runnerProfileJSON)
	code, out, _ := h.run(append(createArgs, "--json")...)
	var created struct {
		WorkspaceID string `json:"workspace_id"`
		Profile     struct {
			ID            string `json:"id"`
			WorkflowLabel string `json:"workflow_label"`
		} `json:"profile"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &created) != nil || created.WorkspaceID != "t-1" || created.Profile.ID != "rpf_0123456789abcdef01234567" || created.Profile.WorkflowLabel != "env-rzv-wide" {
		t.Fatalf("create --json: %d %q", code, out)
	}
	h.routes["DELETE "+item] = func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }
	code, out, _ = h.run("runners", "profiles", "delete", "rpf_0123456789abcdef01234567", "--json")
	var deleted map[string]string
	if code != 0 || json.Unmarshal([]byte(out), &deleted) != nil || deleted["archived_profile_id"] != "rpf_0123456789abcdef01234567" || deleted["workspace_id"] != "t-1" {
		t.Fatalf("delete --json: %d %q", code, out)
	}

	for _, c := range []struct {
		name, route string
		status      int
		code        string
		args        []string
		exit        int
		message     string
	}{
		{"list unavailable", "GET " + base, 503, "runner_profiles_unavailable", []string{"runners", "profiles", "list"}, exitUnavailable, "not available on this Runzivo environment"},
		{"create unavailable", "POST " + base, 503, "runner_profiles_unavailable", createArgs, exitUnavailable, "not available on this Runzivo environment"},
		{"delete unavailable", "DELETE " + item, 503, "runner_profiles_unavailable", []string{"runners", "profiles", "delete", "rpf_0123456789abcdef01234567"}, exitUnavailable, "not available on this Runzivo environment"},
		{"availability profiles unavailable", "GET " + base + "/availability", 503, "runner_profiles_unavailable", []string{"runners", "availability"}, exitUnavailable, "not available on this Runzivo environment"},
		{"availability unavailable", "GET " + base + "/availability", 503, "runner_availability_unavailable", []string{"runners", "availability"}, exitUnavailable, "Runner availability is unavailable"},
		{"delete disabled", "DELETE " + item, 404, "runner_profiles_disabled", []string{"runners", "profiles", "delete", "rpf_0123456789abcdef01234567"}, exitUnavailable, "not turned on"},
		{"limit", "POST " + base, 409, "runner_profile_limit", createArgs, exitFailure, "most live profiles"},
		{"permission", "POST " + base, 403, "permission_required", createArgs, exitAccessDenied, ""},
		{"key scope", "GET " + base, 403, "api_key_scope_required", []string{"runners", "profiles", "list"}, exitAccessDenied, "scope does not allow"},
	} {
		h.json(c.route, c.status, `{"error":{"code":"`+c.code+`"}}`)
		code, _, errOut := h.run(c.args...)
		if code != c.exit || !strings.Contains(errOut, c.message) {
			t.Errorf("%s: %d %q, want exit %d with %q", c.name, code, errOut, c.exit, c.message)
		}
	}
}

const runnerAvailabilityPath = "GET /v1/customer/tenants/t-1/custom-runner-profiles/availability"

// planGatedAvailability is a Free workspace: 2 and 4 vCPU are included, 8 and
// 16 need Team and Business, 32 needs Enterprise. The 8 vCPU shape is also
// busy, to show plan and capacity are separate answers.
const planGatedAvailability = `{"shapes":[
{"workflow_label":"env-rzv-small","vcpu":4,"mem_gib":8,"disk_gib":16,"status":"available","overcommitted":false,"regions":[],"plan_eligible":true,"required_plan":"free"},
{"vcpu":8,"mem_gib":16,"disk_gib":16,"status":"busy","overcommitted":false,"regions":[],"plan_eligible":false,"required_plan":"team"}],
"vcpu_options":[
{"vcpu":2,"plan_eligible":true,"required_plan":"free"},
{"vcpu":4,"plan_eligible":true,"required_plan":"free"},
{"vcpu":8,"plan_eligible":false,"required_plan":"team"},
{"vcpu":16,"plan_eligible":false,"required_plan":"business"},
{"vcpu":32,"plan_eligible":false,"required_plan":"enterprise"}]}`

func TestRunnerAvailabilityShowsLockedAndIncludedPlans(t *testing.T) {
	h := selectedHarness(t)
	h.json(runnerAvailabilityPath, 200, planGatedAvailability)
	code, out, errOut := h.run("runners", "availability")
	if code != 0 || errOut != "" {
		t.Fatalf("availability: %d %q %q", code, out, errOut)
	}
	for _, text := range []string{
		"- env-rzv-small (4 vCPU, 8 GiB memory, 16 GiB disk): Available now; rate -; plan: included",
		"- 8 vCPU, 16 GiB memory, 16 GiB disk: Busy - jobs will queue; rate -; plan: Team (locked)",
		// The test harness API is not a Runzivo environment, so the hint names
		// the portal page instead of a link.
		"Locked: 8 vCPU needs the Team plan; 16 vCPU needs the Business plan. Upgrade in the Runzivo portal under Settings, Billing.",
		"Locked: 32 vCPU needs the Enterprise plan. Contact Runzivo.",
	} {
		if !strings.Contains(out, text) {
			t.Errorf("output lacks %q:\n%s", text, out)
		}
	}
	rows := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		if fields := strings.Fields(line); len(fields) >= 2 && !strings.HasPrefix(line, "-") && !strings.HasPrefix(line, " ") {
			rows[fields[0]] = strings.Join(fields[1:], " ")
		}
	}
	for vcpu, plan := range map[string]string{"VCPU": "PLAN", "2": "included", "4": "included", "8": "Team (locked)", "16": "Business (locked)", "32": "Enterprise (locked)"} {
		if rows[vcpu] != plan {
			t.Errorf("vCPU table row %s = %q, want %q", vcpu, rows[vcpu], plan)
		}
	}
}

func TestRunnerAvailabilityUnlockedHasNoHint(t *testing.T) {
	h := selectedHarness(t)
	h.json(runnerAvailabilityPath, 200, `{"shapes":[{"vcpu":4,"mem_gib":8,"disk_gib":16,"status":"available","overcommitted":false,"regions":[],"plan_eligible":true,"required_plan":null}],"vcpu_options":[{"vcpu":2,"plan_eligible":true,"required_plan":null},{"vcpu":4,"plan_eligible":true,"required_plan":null}]}`)
	code, out, _ := h.run("runners", "availability")
	if code != 0 || strings.Contains(out, "Locked") || strings.Contains(out, "Upgrade") || !strings.Contains(out, "plan: included") {
		t.Fatalf("unlocked: %d %q", code, out)
	}
}

func TestRunnerAvailabilityOldServerHasNoPlanColumn(t *testing.T) {
	h := selectedHarness(t)
	h.json(runnerAvailabilityPath, 200, `{"shapes":[{"vcpu":4,"mem_gib":8,"disk_gib":16,"status":"available","overcommitted":false,"regions":[]}]}`)
	code, out, _ := h.run("runners", "availability")
	if code != 0 || strings.Contains(out, "plan") || strings.Contains(out, "Locked") || strings.Contains(out, "VCPU") {
		t.Fatalf("old server: %d %q", code, out)
	}
	code, out, _ = h.run("runners", "availability", "--json")
	var got map[string]json.RawMessage
	if code != 0 || json.Unmarshal([]byte(out), &got) != nil {
		t.Fatalf("old server --json: %d %q", code, out)
	}
	if _, has := got["vcpu_options"]; has {
		t.Errorf("an old server's output invented vcpu_options: %q", out)
	}
	if strings.Contains(out, "plan_eligible") || strings.Contains(out, "required_plan") {
		t.Errorf("an old server's output invented plan fields: %q", out)
	}
}

func TestRunnerAvailabilityJSONPassesPlanFieldsThrough(t *testing.T) {
	h := selectedHarness(t)
	h.json(runnerAvailabilityPath, 200, planGatedAvailability)
	code, out, _ := h.run("runners", "availability", "--json")
	var got struct {
		Shapes []struct {
			PlanEligible bool    `json:"plan_eligible"`
			RequiredPlan *string `json:"required_plan"`
		} `json:"shapes"`
		VCPUOptions []struct {
			VCPU         int     `json:"vcpu"`
			PlanEligible bool    `json:"plan_eligible"`
			RequiredPlan *string `json:"required_plan"`
		} `json:"vcpu_options"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &got) != nil || len(got.Shapes) != 2 || len(got.VCPUOptions) != 5 {
		t.Fatalf("--json: %d %q", code, out)
	}
	if !got.Shapes[0].PlanEligible || got.Shapes[1].PlanEligible || got.Shapes[1].RequiredPlan == nil || *got.Shapes[1].RequiredPlan != "team" {
		t.Errorf("shapes = %+v", got.Shapes)
	}
	last := got.VCPUOptions[4]
	if last.VCPU != 32 || last.PlanEligible || last.RequiredPlan == nil || *last.RequiredPlan != "enterprise" {
		t.Errorf("last vcpu option = %+v", last)
	}
}

func TestRunnerAvailabilityHintsFromShapesWithoutOptions(t *testing.T) {
	h := selectedHarness(t)
	h.json(runnerAvailabilityPath, 200, `{"shapes":[{"workflow_label":"env-rzv-wide","vcpu":16,"mem_gib":32,"disk_gib":16,"status":"available","overcommitted":false,"regions":[],"plan_eligible":false,"required_plan":"business"}]}`)
	code, out, _ := h.run("runners", "availability")
	if code != 0 || !strings.Contains(out, "plan: Business (locked)") || !strings.Contains(out, "Locked: env-rzv-wide needs the Business plan.") || strings.Contains(out, "VCPU") {
		t.Fatalf("shapes only: %d %q", code, out)
	}
}

func TestRunnerProfileCreatePlanRequiredNamesThePlan(t *testing.T) {
	h := selectedHarness(t)
	createArgs := []string{"runners", "profiles", "create", "--name", "Wide", "--label", "rzv-wide", "--vcpu", "16", "--mem-gib", "32"}
	for _, c := range []struct {
		body string
		want []string
	}{
		{`{"error":{"code":"plan_required","required_plan":"team","message":"x","message_fr_CA":"y"}}`, []string{"A 16 vCPU runner profile needs the Team plan.", "Upgrade"}},
		{`{"error":{"code":"plan_required","required_plan":"enterprise"}}`, []string{"A 16 vCPU runner profile needs the Enterprise plan. Contact Runzivo."}},
		{`{"error":{"code":"plan_required","required_plan":null}}`, []string{"A 16 vCPU runner profile is not included in this workspace's plan"}},
	} {
		h.json("POST /v1/customer/tenants/t-1/custom-runner-profiles", 403, c.body)
		code, out, errOut := h.run(createArgs...)
		if code != exitAccessDenied || out != "" {
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
