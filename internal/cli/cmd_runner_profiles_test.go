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
		_, _ = w.Write([]byte(`{"shapes":[{"profile_id":"rpf_0123456789abcdef01234567","workflow_label":"env-rzv-wide","equivalent_minutes_per_minute":"12.5","vcpu":32,"mem_gib":16,"disk_gib":64,"status":"available","overcommitted":true,"regions":[{"region":"yow1","status":"available","overcommitted":true},{"region":"yul1","status":"not_supported","overcommitted":false}]}]}`))
	}
	code, out, _ := h.run("runners", "availability")
	if code != 0 || !strings.Contains(out, "- env-rzv-wide (32 vCPU, 16 GiB memory, 64 GiB disk): Available now, shared CPU (overcommitted); 12.5 equivalent minutes per minute") ||
		!strings.Contains(out, "    yow1: available now, shared CPU") || !strings.Contains(out, "    yul1: not supported in this region") {
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
		{"runners", "availability", "--region", "yul1"},
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
