package cli

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/Runzivo/rzv-cli/internal/api"
)

const jobJSON = `{"job_id":"j-1","workflow_job_id":42,"run_attempt":1,"repository_id":"100","repository":"acme/app","workflow":"CI","runner_label":"rzv-ubuntu-24.04-x64-2vcpu-8gb","status":"completed","conclusion":"success","queued_at":"2026-10-18T10:00:00Z","started_at":"2026-10-18T10:00:05Z","completed_at":"2026-10-18T10:02:00Z","scheduled_at":"2026-10-18T10:00:01Z","vcpu_count":2,"mem_size_mib":8192,"billable_minutes":1.9,"host_id":null,"lease_issued_at":null,"lease_claimed_at":null,"lease_completed_at":null}`

func selectedHarness(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t)
	h.login()
	if code, _, errOut := h.run("workspaces", "select", "t-1"); code != 0 {
		t.Fatalf("select: %s", errOut)
	}
	return h
}

func TestJobsListAndGet(t *testing.T) {
	h := selectedHarness(t)
	h.json("GET /v1/customer/tenants/t-1/jobs", 200, `{"jobs":[`+jobJSON+`]}`)
	h.routes["GET /v1/customer/tenants/t-1/job"] = func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("jobId") != "j-1" {
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"error":{"code":"not_found"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"job":` + jobJSON + `}`))
	}
	code, out, _ := h.run("jobs", "list")
	if code != 0 || !strings.Contains(out, "- j-1 completed (success) acme/app (CI)") {
		t.Fatalf("jobs list: %d %q", code, out)
	}
	code, out, _ = h.run("jobs", "status", "j-1")
	if code != 0 || !strings.Contains(out, "Conclusion: success") || !strings.Contains(out, "GitHub job: 42 (attempt 1)") {
		t.Fatalf("jobs status: %d %q", code, out)
	}
	code, out, _ = h.run("jobs", "get", "j-1", "--json")
	var got struct {
		WorkspaceID string         `json:"workspace_id"`
		Job         map[string]any `json:"job"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &got) != nil || got.WorkspaceID != "t-1" || got.Job["conclusion"] != "success" {
		t.Fatalf("jobs get --json: %d %q", code, out)
	}
	if code, _, errOut := h.run("jobs", "get", "nope"); code != exitNotFound || !strings.Contains(errOut, "not found") {
		t.Fatalf("missing job: %d %q", code, errOut)
	}
	if code, _, errOut := h.run("jobs", "list", "--workspace", "t-9"); code != exitAccessDenied {
		t.Fatalf("foreign workspace: %d %q", code, errOut)
	}
}

func TestUsageOutcomes(t *testing.T) {
	h := selectedHarness(t)
	h.json("GET /v1/customer/tenants/t-1/usage", 404, `{"error":{"code":"installation_not_linked"}}`)
	if code, out, _ := h.run("usage"); code != 0 || !strings.Contains(out, "No GitHub App installation") {
		t.Fatalf("not linked: %d %q", code, out)
	}
	h.json("GET /v1/customer/tenants/t-1/usage", 200, `{"usage":{"period_start":"a","period_end":"b","billable_minutes":12.5,"job_count":3,"notes":[]}}`)
	if code, out, _ := h.run("usage", "--json"); code != 0 || !strings.Contains(out, `"kind":"recorded"`) || !strings.Contains(out, `"billable_minutes":12.5`) {
		t.Fatalf("recorded: %d %q", code, out)
	}
}

func TestConcurrency(t *testing.T) {
	h := selectedHarness(t)
	h.json("GET /v1/customer/tenants/t-1/concurrency", 404, `{"error":{"code":"installation_not_linked"}}`)
	if code, out, _ := h.run("concurrency"); code != 0 || !strings.Contains(out, "No GitHub App installation") {
		t.Fatalf("not linked: %d %q", code, out)
	}
	h.json("GET /v1/customer/tenants/t-1/concurrency", 200, `{"concurrency":{"limit":4,"used":4,"remaining":0,"admission":"full","reason":"organization_at_concurrency_limit"}}`)
	code, out, _ := h.run("concurrency")
	if code != 0 || !strings.Contains(out, "Running: 4 of 4 (0 remaining)") || !strings.Contains(out, "Admission: full") || !strings.Contains(out, "Reason: organization_at_concurrency_limit") {
		t.Fatalf("configured: %d %q", code, out)
	}
	if code, out, _ := h.run("concurrency", "--json"); code != 0 || !strings.Contains(out, `"kind":"configured"`) || !strings.Contains(out, `"used":4`) {
		t.Fatalf("json: %d %q", code, out)
	}
	h.json("GET /v1/customer/tenants/t-1/concurrency", 200, `{"concurrency":{"limit":4}}`)
	if code, _, _ := h.run("concurrency"); code == 0 {
		t.Fatal("a response without admission must be rejected")
	}
	if code, _, _ := h.run("concurrency", "extra"); code != exitUsage {
		t.Fatalf("extra argument: %d", code)
	}
}

func TestRunners(t *testing.T) {
	h := selectedHarness(t)
	h.routes["GET /v1/customer/tenants/t-1/runner-inventory"] = func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("repository_id") != "100" {
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"error":{"code":"repository_not_authorized"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"runner_inventory":{"profiles":[{"label":"rzv-ubuntu-24.04-x64-2vcpu-8gb","vcpu_count":2,"mem_size_mib":8192,"available":true,"reason":null,"running_jobs":2,"waiting_jobs":1},{"label":"rzv-ubuntu-24.04-x64-8vcpu-32gb","vcpu_count":8,"mem_size_mib":32768,"available":false,"reason":"no_host_large_enough","running_jobs":0,"waiting_jobs":0}],"unlisted":{"running_jobs":1,"waiting_jobs":0}}}`))
	}
	code, out, _ := h.run("runners", "100")
	if code != 0 || !strings.Contains(out, "available: 2 vCPU, 8192 MiB, 2 running, 1 waiting, runs-on: rzv-ubuntu-24.04-x64-2vcpu-8gb") ||
		!strings.Contains(out, "blocked (no_host_large_enough)") || !strings.Contains(out, "Other sizes no longer listed: 1 running, 0 waiting") {
		t.Fatalf("runners: %d %q", code, out)
	}
	if code, out, _ := h.run("runners", "100", "--json"); code != 0 || !strings.Contains(out, `"kind":"available"`) || !strings.Contains(out, `"running_jobs":2`) {
		t.Fatalf("json: %d %q", code, out)
	}
	if code, out, _ := h.run("runners", "101"); code != 0 || !strings.Contains(out, "not authorized") {
		t.Fatalf("unauthorized repo: %d %q", code, out)
	}
	h.routes["GET /v1/customer/tenants/t-1/runner-inventory"] = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"error":{"code":"installation_not_linked"}}`))
	}
	if code, out, _ := h.run("runners", "100"); code != 0 || !strings.Contains(out, "No GitHub App installation") {
		t.Fatalf("not linked: %d %q", code, out)
	}
	if code, _, _ := h.run("runners", "acme/app"); code != exitUsage {
		t.Fatalf("non-numeric repo: %d", code)
	}
	if code, _, _ := h.run("runners"); code != exitUsage {
		t.Fatalf("missing repo: %d", code)
	}
}

func TestBilling(t *testing.T) {
	h := selectedHarness(t)
	h.json("GET /v1/customer/tenants/t-1/billing", 200, `{"billing":{"allowance_configured":true,"checkout_available":true,"payment_mode":"test","live_payments_enabled":false,
		"balance":{"free_allowance_minutes":500,"free_consumed_minutes":10.5,"free_reserved_minutes":0,"free_available":489.5,"purchased_remaining_minutes":0,"purchased_reserved_minutes":0,"purchased_available":0,"reserved_minutes":0,"period_start":"a","period_end":"b","review_hold":false},
		"packs":[{"id":"p1","currency":"USD","amount_cents":1000,"minutes":1000}]}}`)
	if code, out, _ := h.run("billing"); code != 0 || !strings.Contains(out, "Free: 489.5 available of 500") || !strings.Contains(out, "- p1: 1000 minutes") {
		t.Fatalf("billing: %d %q", code, out)
	}
	h.json("GET /v1/customer/tenants/t-1/billing/purchases", 200, `{"purchases":[],"limit":20,"bounds":{"limit":20,"max_limit":100}}`)
	if code, out, _ := h.run("billing", "purchases"); code != 0 || !strings.Contains(out, "No purchases") {
		t.Fatalf("purchases: %d %q", code, out)
	}
	if code, _, _ := h.run("billing", "checkout"); code != exitUsage {
		t.Fatalf("checkout should be a usage error: %d", code)
	}
}

func TestGitHubStatusRepositoriesAndProfiles(t *testing.T) {
	h := selectedHarness(t)
	h.json("GET /v1/customer/tenants/t-1/github-installation", 404, `{"error":{"code":"installation_not_linked"}}`)
	if code, out, _ := h.run("github", "status"); code != 0 || !strings.Contains(out, "No GitHub App installation") {
		t.Fatalf("status: %d %q", code, out)
	}
	h.json("GET /v1/customer/tenants/t-1/github-installations", 200, `{"installations":[{"installation_id":"9","state":"active","account_name":"acme","manage_url":null,"repositories":[{"repository_id":"100","name":"acme/app","authorized":true},{"repository_id":"101","name":null,"authorized":false}]}]}`)
	code, out, _ := h.run("repositories", "list")
	if code != 0 || !strings.Contains(out, "- acme/app [100] (authorized)") || !strings.Contains(out, "- - [101] (not authorized)") {
		t.Fatalf("repositories: %d %q", code, out)
	}
	h.routes["GET /v1/customer/tenants/t-1/runner-profiles"] = func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("repository_id") != "100" {
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"error":{"code":"repository_not_authorized"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"runner_profiles":{"workflow_label_format":"rzv-<image>-<arch>-<size>","profiles":[{"label":"rzv-ubuntu-24.04-x64-2vcpu-8gb","vcpu_count":2,"mem_size_mib":8192,"available":true,"reason":null},{"label":"rzv-ubuntu-24.04-x64-8vcpu-32gb","vcpu_count":8,"mem_size_mib":32768,"available":false,"reason":"no_host_large_enough"}]}}`))
	}
	code, out, _ = h.run("runner-profiles", "100")
	if code != 0 || !strings.Contains(out, "available: 2 vCPU, 8192 MiB, runs-on: rzv-ubuntu-24.04-x64-2vcpu-8gb") || !strings.Contains(out, "blocked (no_host_large_enough)") {
		t.Fatalf("profiles: %d %q", code, out)
	}
	if code, out, _ := h.run("runner-profiles", "101"); code != 0 || !strings.Contains(out, "not authorized") {
		t.Fatalf("unauthorized repo: %d %q", code, out)
	}
	if code, _, _ := h.run("runner-profiles", "acme/app"); code != exitUsage {
		t.Fatalf("non-numeric repo: %d", code)
	}
}

func TestGitHubConnectAndAuthorize(t *testing.T) {
	h := selectedHarness(t)
	h.json("POST /v1/customer/tenants/t-1/github-installation/start", 200, `{"state":"s-1","expires_at":"2026-10-18T11:00:00Z"}`)
	if code, out, _ := h.run("github", "connect"); code != 0 || !strings.Contains(out, "State: s-1") {
		t.Fatalf("connect: %d %q", code, out)
	}
	var gh *httptest.Server
	gh = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login/device/code" {
			_, _ = w.Write([]byte(`{"device_code":"dc","user_code":"ABCD","verification_uri":"` + gh.URL + `/login/device","expires_in":900,"interval":1}`))
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"gh-user-token"}`))
	}))
	t.Cleanup(gh.Close)
	githubBaseURL = gh.URL
	t.Cleanup(func() { githubBaseURL = "https://github.com" })
	var sent map[string]any
	h.routes["POST /v1/customer/tenants/t-1/github-installation/complete"] = func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &sent)
		_, _ = w.Write([]byte(`{"github_installation":{"state":"active","account_name":"acme","manage_url":null,"repositories":[]}}`))
	}
	code, out, errOut := h.run("github", "authorize", "--state", "s-1", "--installation-id", "9", "--github-client-id", "Iv1.test")
	if code != 0 || !strings.Contains(out, "State: active") || !strings.Contains(errOut, "enter this code: ABCD") {
		t.Fatalf("authorize: %d %q %q", code, out, errOut)
	}
	if sent["user_access_token"] != "gh-user-token" || sent["installation_id"] != float64(9) || sent["state"] != "s-1" {
		t.Fatalf("complete body: %v", sent)
	}
	if strings.Contains(out+errOut, "gh-user-token") {
		t.Fatal("GitHub token printed")
	}
	h.json("POST /v1/customer/tenants/t-1/github-installation/complete", 409, `{"error":{"code":"link_state_invalid"}}`)
	if code, _, _ := h.run("github", "authorize", "--state", "s-1", "--installation-id", "9", "--github-client-id", "Iv1.test"); code != exitFailure {
		t.Fatalf("link_state_invalid: %d", code)
	}
}

func TestAPIKeys(t *testing.T) {
	h := selectedHarness(t)
	h.json("GET /v1/customer/tenants/t-1/api-keys", 200, `{"api_keys":[{"id":"k1","name":"ci","prefix":"rzv_ab","scopes":["jobs:read"],"permissions":["jobs:read","logs:read"],"created_at":"a","last_used_at":null,"revoked_at":null,"expires_at":"2027-10-18T00:00:00Z"}],"allowed_scopes":["jobs:read"],"capabilities":{"can_create":true,"can_rename":true,"can_revoke":true}}`)
	if code, out, _ := h.run("api-keys", "list"); code != 0 || !strings.Contains(out, "- ci [k1] rzv_ab… active, scopes jobs:read, permissions jobs:read,logs:read") {
		t.Fatalf("list: %d %q", code, out)
	}
	code, out, _ := h.run("api-keys", "list", "--json")
	if code != 0 || !strings.Contains(out, `"permissions":["jobs:read","logs:read"]`) {
		t.Fatalf("list json: %d %q", code, out)
	}
	var body map[string]any
	h.routes["POST /v1/customer/tenants/t-1/api-keys"] = func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &body)
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"api_key":{"id":"k2","name":"deploy","prefix":"rzv_cd","scopes":[],"permissions":["jobs:write","logs:read"],"created_at":"a","last_used_at":null,"revoked_at":null,"expires_at":null},"secret":"one-time-secret"}`))
	}
	code, out, _ = h.run("api-keys", "create", "--name", "deploy", "--permissions", "jobs:write, logs:read", "--idempotency-key", "op-1", "--json")
	if code != 0 || !strings.Contains(out, "one-time-secret") || body["idempotency_key"] != "op-1" || !strings.Contains(out, `"permissions":["jobs:write","logs:read"]`) {
		t.Fatalf("create: %d %q %v", code, out, body)
	}
	permissions, ok := body["permissions"].([]any)
	if !ok || len(permissions) != 2 || permissions[0] != "jobs:write" || permissions[1] != "logs:read" {
		t.Fatalf("permissions body: %v", body)
	}
	if _, ok := body["scopes"]; ok {
		t.Fatalf("permissions request included scopes: %v", body)
	}
	code, out, _ = h.run("api-keys", "create", "--name", "deploy", "--scopes", "jobs:read, usage:read", "--idempotency-key", "op-2")
	if code != 0 || !strings.Contains(out, "permissions jobs:write,logs:read") || body["scopes"] == nil {
		t.Fatalf("legacy create: %d %q %v", code, out, body)
	}
	for _, scopes := range []string{"all:all,jobs:read", "jobs:read,jobs:read", "admin:all", ""} {
		if code, _, _ := h.run("api-keys", "create", "--name", "x", "--scopes="+scopes, "--idempotency-key", "op"); code != exitUsage {
			t.Errorf("scopes %q accepted: %d", scopes, code)
		}
	}
	for _, permissions := range []string{"", ",", "jobs:read,jobs:read"} {
		calls := len(h.seenAuth)
		if code, _, _ := h.run("api-keys", "create", "--name", "x", "--permissions="+permissions, "--idempotency-key", "op"); code != exitUsage || len(h.seenAuth) != calls {
			t.Errorf("permissions %q accepted or made a request: %d", permissions, code)
		}
	}
	calls := len(h.seenAuth)
	if code, _, _ := h.run("api-keys", "create", "--name", "x", "--scopes", "jobs:read", "--permissions", "logs:read", "--idempotency-key", "op"); code != exitUsage || len(h.seenAuth) != calls {
		t.Fatalf("mixed options accepted or made a request: %d", code)
	}
	h.json("POST /v1/customer/tenants/t-1/api-keys", 403, `{"error":{"code":"grant_ceiling"}}`)
	if code, _, errOut := h.run("api-keys", "create", "--name", "denied", "--permissions", "billing:write", "--idempotency-key", "op-denied"); code != exitAccessDenied || !strings.Contains(errOut, "not authorized") {
		t.Fatalf("grant ceiling: %d %q", code, errOut)
	}
	h.routes["DELETE /v1/customer/tenants/t-1/api-keys/k1"] = func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }
	if code, out, _ := h.run("api-keys", "revoke", "k1", "--json"); code != 0 || !strings.Contains(out, `"revoked_api_key_id":"k1"`) {
		t.Fatalf("revoke: %d %q", code, out)
	}
	h.json("PATCH /v1/customer/tenants/t-1/api-keys/k1", 200, `{"api_key":{"id":"k1","name":"renamed","prefix":"rzv_ab","scopes":["jobs:read"],"permissions":["jobs:read"],"created_at":"a","last_used_at":null,"revoked_at":null,"expires_at":null}}`)
	if code, out, _ := h.run("api-keys", "rename", "k1", "--name", "renamed"); code != 0 || !strings.Contains(out, "Renamed API key k1 to renamed with permissions jobs:read.") {
		t.Fatalf("rename: %d %q", code, out)
	}
	code, out, _ = h.run("api-keys", "rename", "k1", "--name", "renamed", "--json")
	if code != 0 || !strings.Contains(out, `"permissions":["jobs:read"]`) {
		t.Fatalf("rename json: %d %q", code, out)
	}
	h.env["RZV_API_KEY"] = "rzv_test_key"
	if code, _, _ := h.run("api-keys", "list"); code != exitAccessDenied {
		t.Fatalf("api key managing keys: %d", code)
	}
}

func TestAPIKeyWithoutSignIn(t *testing.T) {
	h := newHarness(t)
	h.env["RZV_API_KEY"] = "rzv_test_key"
	h.env["RZV_API_URL"] = h.api.URL
	h.json("GET /v1/customer/tenants/t-5/jobs", 200, `{"jobs":[]}`)
	if code, _, errOut := h.run("jobs", "list"); code != exitUsage || !strings.Contains(errOut, "--workspace") {
		t.Fatalf("no workspace: %d %q", code, errOut)
	}
	code, out, _ := h.run("jobs", "list", "--workspace", "t-5")
	if code != 0 || !strings.Contains(out, "No jobs for workspace t-5") {
		t.Fatalf("api key jobs: %d %q", code, out)
	}
	if last := h.seenAuth[len(h.seenAuth)-1]; last != "Bearer rzv_test_key" {
		t.Fatalf("API key not sent: %q", last)
	}
	for _, auth := range h.seenAuth {
		if auth == "" {
			t.Fatal("request without credentials")
		}
	}
}

const workspaceSettingsJSON = `{"tenant_id":"t-1","name":"Acme","logo":null,"can_update":true}`
const workspaceMemberJSON = `{"identity_id":"u-1","role":"member","email":"person@example.test","can_change_role":true,"can_remove":true,"allowed_roles":["owner","admin","member"]}`
const workspaceInvitationJSON = `{"invitation_id":"i-1","email":"person@example.test","role":"member","status":"pending","expires_at":"2026-11-01T00:00:00Z","created_at":"2026-10-01T00:00:00Z","can_resend":true,"can_revoke":true,"teams":[{"team_id":"team-1","name":"Build"}]}`

func TestWorkspaceSettingsMembersAndInvitations(t *testing.T) {
	h := selectedHarness(t)
	h.json("GET /v1/customer/tenants/t-1/settings", 200, workspaceSettingsJSON)
	h.json("GET /v1/customer/tenants/t-1/members", 200, `{"current_identity_id":"me","can_manage_invitations":true,"can_invite":true,"members":[`+workspaceMemberJSON+`]}`)
	h.json("GET /v1/customer/tenants/t-1/invitations", 200, `{"invitations":[`+workspaceInvitationJSON+`]}`)
	if code, out, _ := h.run("workspaces", "settings", "show"); code != 0 || !strings.Contains(out, "Can update: true") {
		t.Fatalf("settings show: %d %q", code, out)
	}
	if code, out, _ := h.run("workspaces", "members", "list", "--json"); code != 0 || !strings.Contains(out, `"can_change_role":true`) || !strings.Contains(out, `"allowed_roles":["owner","admin","member"]`) {
		t.Fatalf("members list json: %d %q", code, out)
	}
	if code, out, _ := h.run("workspaces", "invitations", "list"); code != 0 || !strings.Contains(out, "INVITATION ID") || !strings.Contains(out, "Build [team-1]") {
		t.Fatalf("invitations list: %d %q", code, out)
	}

	var patch map[string]any
	h.routes["PATCH /v1/customer/tenants/t-1/settings"] = func(w http.ResponseWriter, r *http.Request) {
		patch = nil
		_ = json.NewDecoder(r.Body).Decode(&patch)
		_, _ = w.Write([]byte(workspaceSettingsJSON))
	}
	if code, out, _ := h.run("workspaces", "settings", "update", "--name", "Renamed", "--json"); code != 0 || patch["name"] != "Renamed" || strings.Contains(out, "remove_logo") {
		t.Fatalf("settings update: %d %q %#v", code, out, patch)
	}

	var role map[string]string
	h.routes["PATCH /v1/customer/tenants/t-1/members/u-1"] = func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&role)
		_, _ = w.Write([]byte(workspaceMemberJSON))
	}
	if code, out, _ := h.run("workspaces", "members", "role", "u-1", "--role", "member"); code != 0 || role["role"] != "member" || !strings.Contains(out, "Updated member") {
		t.Fatalf("member role: %d %q %#v", code, out, role)
	}
	h.routes["DELETE /v1/customer/tenants/t-1/members/u-1"] = func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }
	if code, out, _ := h.run("workspaces", "members", "remove", "u-1"); code != 0 || !strings.Contains(out, "Removed member") {
		t.Fatalf("member remove: %d %q", code, out)
	}

	var invitation map[string]any
	h.routes["POST /v1/customer/tenants/t-1/invitations"] = func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&invitation)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(workspaceInvitationJSON))
	}
	if code, out, _ := h.run("workspaces", "invitations", "invite", "--email", "person@example.test", "--role", "member", "--team-ids", "team-1,team-2"); code != 0 || !strings.Contains(out, "Invited") || len(invitation["team_ids"].([]any)) != 2 {
		t.Fatalf("invite: %d %q %#v", code, out, invitation)
	}
	h.json("POST /v1/customer/tenants/t-1/invitations/i-1/resend", 200, workspaceInvitationJSON)
	if code, out, _ := h.run("workspaces", "invitations", "resend", "i-1"); code != 0 || !strings.Contains(out, "Resent invitation") {
		t.Fatalf("resend: %d %q", code, out)
	}
	var teams map[string]any
	h.routes["PATCH /v1/customer/tenants/t-1/invitations/i-1"] = func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&teams)
		_, _ = w.Write([]byte(workspaceInvitationJSON))
	}
	if code, out, _ := h.run("workspaces", "invitations", "teams", "i-1", "--team-ids="); code != 0 || !strings.Contains(out, "Updated custom teams") || len(teams["team_ids"].([]any)) != 0 {
		t.Fatalf("teams: %d %q %#v", code, out, teams)
	}
	h.routes["DELETE /v1/customer/tenants/t-1/invitations/i-1"] = func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }
	if code, out, _ := h.run("workspaces", "invitations", "revoke", "i-1", "--json"); code != 0 || !strings.Contains(out, `"revoked_invitation_id":"i-1"`) {
		t.Fatalf("revoke: %d %q", code, out)
	}
}

func TestWorkspaceLogoAndInvitationAcceptance(t *testing.T) {
	h := selectedHarness(t)
	var imageData bytes.Buffer
	if err := png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	logo := t.TempDir() + "/logo.png"
	if err := os.WriteFile(logo, imageData.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	var patch map[string]any
	h.routes["PATCH /v1/customer/tenants/t-1/settings"] = func(w http.ResponseWriter, r *http.Request) {
		patch = nil
		_ = json.NewDecoder(r.Body).Decode(&patch)
		_, _ = w.Write([]byte(workspaceSettingsJSON))
	}
	if code, out, _ := h.run("workspaces", "settings", "update", "--logo", logo); code != 0 || !strings.Contains(out, "Updated workspace settings") || patch["logo"].(map[string]any)["content_type"] != "image/png" {
		t.Fatalf("logo update: %d %q %#v", code, out, patch)
	}
	if code, _, _ := h.run("workspaces", "settings", "update", "--logo", logo, "--remove-logo"); code != exitUsage {
		t.Fatalf("logo remove conflict: %d", code)
	}
	if code, out, _ := h.run("workspaces", "settings", "update", "--remove-logo", "--json"); code != 0 || patch["remove_logo"] != true || len(patch) != 1 || !strings.Contains(out, `"workspace_id":"t-1"`) {
		t.Fatalf("logo remove: %d %q %#v", code, out, patch)
	}
	if code, _, _ := h.run("workspaces", "settings", "update", "--logo", t.TempDir()+"/missing"); code != exitUsage {
		t.Fatalf("missing logo: %d", code)
	}

	// Accept uses a human session but does not use the selected workspace or memberships.
	fresh := newHarness(t)
	fresh.login()
	fresh.routes["GET /v1/customer/memberships"] = func(_ http.ResponseWriter, _ *http.Request) { t.Fatal("accept should not check workspace membership") }
	var received string
	fresh.routes["POST /v1/customer/invitations/accept"] = func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		received = body["token"]
		_, _ = w.Write([]byte(`{"tenant_id":"t-3","tenant_slug":"joined","tenant_name":"Joined","role":"member"}`))
	}
	secretFile := t.TempDir() + "/invite-token"
	if err := os.WriteFile(secretFile, []byte(" synthetic-secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := fresh.run("workspaces", "invitations", "accept", "--token-file", secretFile, "--json")
	if code != 0 || received != "synthetic-secret" || strings.Contains(out+errOut, received) || !strings.Contains(out, `"tenant_id":"t-3"`) {
		t.Fatalf("accept: %d %q %q token=%q", code, out, errOut, received)
	}
	fresh.env["RZV_API_KEY"] = "key"
	if code, _, _ := fresh.run("workspaces", "invitations", "accept", "--token", "synthetic-secret"); code != exitAccessDenied {
		t.Fatalf("API key accepted invitation: %d", code)
	}
}

func TestWorkspaceAccessValidationAndConflicts(t *testing.T) {
	h := selectedHarness(t)
	calls := len(h.seenAuth)
	if code, _, _ := h.run("workspaces", "invitations", "teams", "i-1"); code != exitUsage || len(h.seenAuth) != calls {
		t.Fatalf("missing teams option networked: %d", code)
	}
	if code, _, _ := h.run("workspaces", "invitations", "invite", "--email", "person@example.test", "--role", "member", "--team-ids", "one,one"); code != exitUsage || len(h.seenAuth) != calls {
		t.Fatalf("duplicate teams networked: %d", code)
	}
	if code, _, _ := h.run("workspaces", "members", "role", "u-1", "--role", "viewer"); code != exitUsage || len(h.seenAuth) != calls {
		t.Fatalf("invalid member role networked: %d", code)
	}
	if code, _, _ := h.run("workspaces", "invitations", "invite", "--email", "person@example.test", "--role", "viewer"); code != exitUsage || len(h.seenAuth) != calls {
		t.Fatalf("invalid invitation role networked: %d", code)
	}
	if code, _, _ := h.run("workspaces", "settings", "update", "--name", strings.Repeat("é", 65)); code != exitUsage || len(h.seenAuth) != calls {
		t.Fatalf("multibyte name networked: %d", code)
	}
	if err := validateWorkspaceSettingsPatch(api.WorkspaceSettingsPatch{Logo: &api.WorkspaceLogo{ContentType: "image/png", Base64: strings.Repeat("a", 256<<10)}}); err == nil {
		t.Fatal("oversized settings patch accepted")
	}
	if err := validateWorkspaceSettingsPatch(api.WorkspaceSettingsPatch{Logo: &api.WorkspaceLogo{ContentType: "image/png", Base64: strings.Repeat("a", (256<<10)-100)}}); err != nil {
		t.Fatalf("settings patch below body limit rejected: %v", err)
	}
	largeToken := t.TempDir() + "/large-token"
	if err := os.WriteFile(largeToken, []byte(strings.Repeat("x", (64<<10)+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := h.run("workspaces", "invitations", "accept", "--token-file", largeToken); code != exitUsage || len(h.seenAuth) != calls {
		t.Fatalf("oversized token file networked: %d", code)
	}
	h.json("PATCH /v1/customer/tenants/t-1/members/u-1", 409, `{"error":{"code":"last_owner_required"}}`)
	if code, _, errOut := h.run("workspaces", "members", "role", "u-1", "--role", "member"); code != exitFailure || !strings.Contains(errOut, "last_owner_required") {
		t.Fatalf("last owner: %d %q", code, errOut)
	}
	h.json("DELETE /v1/customer/tenants/t-1/members/u-1", 409, `{"error":{"code":"self_member_removal"}}`)
	if code, _, errOut := h.run("workspaces", "members", "remove", "u-1"); code != exitFailure || !strings.Contains(errOut, "self_member_removal") {
		t.Fatalf("self removal: %d %q", code, errOut)
	}
	h.json("GET /v1/customer/tenants/t-1/invitations", 403, `{"error":{"code":"permission_required"}}`)
	if code, _, errOut := h.run("workspaces", "invitations", "list"); code != exitAccessDenied || !strings.Contains(errOut, "does not allow this change") {
		t.Fatalf("permission denied: %d %q", code, errOut)
	}
	h.json("GET /v1/customer/tenants/t-1/settings", 200, `{"name":"missing required fields"}`)
	if code, _, errOut := h.run("workspaces", "settings", "show"); code != exitFailure || !strings.Contains(errOut, "malformed_response") {
		t.Fatalf("malformed settings: %d %q", code, errOut)
	}
	h.env["RZV_API_KEY"] = "all-key"
	h.json("GET /v1/customer/tenants/t-1/settings", 200, workspaceSettingsJSON)
	if code, _, _ := h.run("workspaces", "settings", "show"); code != 0 {
		t.Fatalf("API key settings: %d", code)
	}
}

func TestWorkspaceAccessHelpListsSubcommands(t *testing.T) {
	h := newHarness(t)
	code, out, _ := h.run("workspaces", "invitations", "--help")
	if code != 0 || !strings.Contains(out, "workspaces settings update") || !strings.Contains(out, "workspaces members role") || !strings.Contains(out, "workspaces invitations accept") {
		t.Fatalf("workspace access help: %d %q", code, out)
	}
}

func TestWorkspaceAccessNotFoundErrors(t *testing.T) {
	h := selectedHarness(t)
	h.json("PATCH /v1/customer/tenants/t-1/members/u-missing", 404, `{"error":{"code":"not_found"}}`)
	code, _, errOut := h.run("workspaces", "members", "role", "u-missing", "--role", "member", "--json")
	if code != exitNotFound || !strings.Contains(errOut, `"error":"not_found"`) || !strings.Contains(errOut, "member was not found") {
		t.Fatalf("missing member: %d %q", code, errOut)
	}
	h.json("POST /v1/customer/tenants/t-1/invitations/i-missing/resend", 404, `{"error":{"code":"not_found"}}`)
	code, _, errOut = h.run("workspaces", "invitations", "resend", "i-missing", "--json")
	if code != exitNotFound || !strings.Contains(errOut, `"error":"not_found"`) || !strings.Contains(errOut, "invitation was not found") {
		t.Fatalf("missing invitation: %d %q", code, errOut)
	}

	fresh := newHarness(t)
	fresh.login()
	fresh.json("POST /v1/customer/invitations/accept", 404, `{"error":{"code":"not_found"}}`)
	code, _, errOut = fresh.run("workspaces", "invitations", "accept", "--token", "synthetic-token", "--json")
	if code != exitNotFound || !strings.Contains(errOut, `"error":"not_found"`) || !strings.Contains(errOut, "no longer available") {
		t.Fatalf("unavailable invitation: %d %q", code, errOut)
	}
	fresh.json("POST /v1/customer/invitations/accept", 403, `{"error":{"code":"invitation_recipient_verification_required"}}`)
	code, _, errOut = fresh.run("workspaces", "invitations", "accept", "--token", "synthetic-token", "--json")
	if code != exitAccessDenied || !strings.Contains(errOut, `"error":"invitation_recipient_verification_required"`) || !strings.Contains(errOut, "verified email") {
		t.Fatalf("invitation recipient verification: %d %q", code, errOut)
	}
}
