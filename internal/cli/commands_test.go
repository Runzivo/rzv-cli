package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
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

func TestBillingCodesRedeem(t *testing.T) {
	h := selectedHarness(t)
	codeFile := h.t.TempDir() + "/code"
	if err := os.WriteFile(codeFile, []byte("input-code\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var body map[string]string
	status := http.StatusCreated
	h.routes["POST /v1/customer/tenants/t-1/billing/codes/redeem"] = func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"redemption":{"id":"r1","kind":"voucher","code":"1234","status":"credited","equivalent_minutes":1,"credit_expires_at":null},"replayed":false}`))
	}
	code, out, errOut := h.run("billing", "codes", "redeem", "--code-file", codeFile)
	if code != 0 || errOut != "" || !strings.Contains(out, "ID: r1") || !strings.Contains(out, "Equivalent minutes: 1") || !strings.Contains(out, "Credit expires: -") || strings.Contains(out, "input-code") || body["code"] != "input-code" {
		t.Fatalf("redeem: %d %q %q %v", code, out, errOut, body)
	}
	status = http.StatusOK
	h.routes["POST /v1/customer/tenants/t-1/billing/codes/redeem"] = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"redemption":{"id":"r2","kind":"promo","code":"SAFE","status":"pending_eligibility","equivalent_minutes":10,"credit_expires_at":"2027-01-01T00:00:00Z"},"replayed":true}`))
	}
	code, out, _ = h.run("billing", "codes", "redeem", "--code-file", codeFile, "--json")
	if code != 0 || !strings.Contains(out, `"replayed":true`) || !strings.Contains(out, `"credit_expires_at":"2027-01-01T00:00:00Z"`) {
		t.Fatalf("replay: %d %q", code, out)
	}
	for _, errorCase := range []struct {
		status int
		code   string
		exit   int
	}{
		{http.StatusForbidden, "insufficient_role", exitAccessDenied},
		{http.StatusNotFound, "code_not_found", exitFailure},
		{http.StatusConflict, "code_already_redeemed", exitFailure},
		{http.StatusGone, "code_expired", exitFailure},
		{http.StatusTooManyRequests, "code_redeem_rate_limited", exitFailure},
		{http.StatusServiceUnavailable, "promo_policy_not_configured", exitUnavailable},
	} {
		h.json("POST /v1/customer/tenants/t-1/billing/codes/redeem", errorCase.status, `{"error":{"code":"`+errorCase.code+`"}}`)
		code, out, errOut = h.run("billing", "codes", "redeem", "--code-file", codeFile)
		if code != errorCase.exit || out != "" || strings.Contains(errOut, "input-code") {
			t.Errorf("error %s: %d %q %q", errorCase.code, code, out, errOut)
		}
	}
	for _, input := range []struct {
		path string
		data string
	}{
		{path: "missing"},
		{path: "empty", data: "\n"},
		{path: "long", data: strings.Repeat("x", maxRedeemCodeBytes+1)},
		{path: "embedded-newline", data: "input\ncode"},
	} {
		path := input.path
		if input.data != "" {
			path = h.t.TempDir() + "/" + input.path
			if err := os.WriteFile(path, []byte(input.data), 0600); err != nil {
				t.Fatal(err)
			}
		}
		calls := len(h.seenAuth)
		code, out, errOut = h.run("billing", "codes", "redeem", "--code-file", path)
		if code != exitUsage || out != "" || len(h.seenAuth) != calls || (strings.TrimSpace(input.data) != "" && strings.Contains(errOut, strings.TrimSpace(input.data))) {
			t.Errorf("input %s: %d %q %q", input.path, code, out, errOut)
		}
	}
	h.json("POST /v1/customer/tenants/t-1/billing/codes/redeem", http.StatusForbidden, `{"error":{"code":"code_not_eligible"}}`)
	code, out, errOut = h.run("billing", "codes", "redeem", "--code-file", codeFile)
	if code != exitAccessDenied || out != "" || !strings.Contains(errOut, "not eligible") || strings.Contains(errOut, "input-code") {
		t.Fatalf("code eligibility: %d %q %q", code, out, errOut)
	}
	code, out, errOut = h.run("billing", "codes", "redeem", "--code-file", codeFile, "--json")
	if code != exitAccessDenied || out != "" || !strings.Contains(errOut, `"error":"code_not_eligible"`) || strings.Contains(errOut, "input-code") {
		t.Fatalf("code eligibility json: %d %q %q", code, out, errOut)
	}
	maxCode := strings.Repeat("x", maxRedeemCodeBytes)
	for _, ending := range []string{"\n", "\r\n"} {
		path := h.t.TempDir() + "/max-code"
		if err := os.WriteFile(path, []byte(maxCode+ending), 0600); err != nil {
			t.Fatal(err)
		}
		if read, err := readRedeemCode(path); err != nil || read != maxCode {
			t.Errorf("64-byte code with %q: %q %v", ending, read, err)
		}
	}
	h.json("POST /v1/customer/tenants/t-1/billing/codes/redeem", http.StatusCreated, `{}`)
	code, out, errOut = h.run("billing", "codes", "redeem", "--code-file", codeFile)
	if code != exitFailure || out != "" || strings.Contains(errOut, "input-code") {
		t.Fatalf("malformed response: %d %q %q", code, out, errOut)
	}
	keyHarness := newHarness(t)
	keyHarness.env["RZV_API_KEY"] = "key-credential"
	keyHarness.env["RZV_API_URL"] = keyHarness.api.URL
	keyHarness.routes["POST /v1/customer/tenants/t-5/billing/codes/redeem"] = func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer key-credential" {
			t.Errorf("unexpected API-key authorization %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"redemption":{"id":"r3","kind":"promo","code":"SAFE","status":"credited","equivalent_minutes":1,"credit_expires_at":null},"replayed":false}`))
	}
	code, out, errOut = keyHarness.run("billing", "codes", "redeem", "--workspace", "t-5", "--code-file", codeFile)
	if code != 0 || errOut != "" || !strings.Contains(out, "ID: r3") {
		t.Fatalf("API-key redeem: %d %q %q", code, out, errOut)
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
	h.json("GET /v1/customer/tenants/t-1/api-keys", 200, `{"api_keys":[{"id":"k1","name":"ci","prefix":"rzv_ab","scopes":["jobs:read"],"created_at":"a","last_used_at":null,"revoked_at":null,"expires_at":"2027-10-18T00:00:00Z"}],"allowed_scopes":["jobs:read"],"capabilities":{"can_create":true,"can_rename":true,"can_revoke":true}}`)
	if code, out, _ := h.run("api-keys", "list"); code != 0 || !strings.Contains(out, "- ci [k1] rzv_ab… active, scopes jobs:read") {
		t.Fatalf("list: %d %q", code, out)
	}
	var body map[string]any
	h.routes["POST /v1/customer/tenants/t-1/api-keys"] = func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &body)
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"api_key":{"id":"k2","name":"deploy","prefix":"rzv_cd","scopes":["jobs:read","usage:read"],"created_at":"a","last_used_at":null,"revoked_at":null,"expires_at":null},"secret":"one-time-secret"}`))
	}
	code, out, _ := h.run("api-keys", "create", "--name", "deploy", "--scopes", "jobs:read, usage:read", "--idempotency-key", "op-1")
	if code != 0 || !strings.Contains(out, "one-time-secret") || body["idempotency_key"] != "op-1" {
		t.Fatalf("create: %d %q %v", code, out, body)
	}
	for _, scopes := range []string{"all:all,jobs:read", "jobs:read,jobs:read", "admin:all", ""} {
		if code, _, _ := h.run("api-keys", "create", "--name", "x", "--scopes="+scopes, "--idempotency-key", "op"); code != exitUsage {
			t.Errorf("scopes %q accepted: %d", scopes, code)
		}
	}
	h.routes["DELETE /v1/customer/tenants/t-1/api-keys/k1"] = func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }
	if code, out, _ := h.run("api-keys", "revoke", "k1", "--json"); code != 0 || !strings.Contains(out, `"revoked_api_key_id":"k1"`) {
		t.Fatalf("revoke: %d %q", code, out)
	}
	h.json("PATCH /v1/customer/tenants/t-1/api-keys/k1", 200, `{"api_key":{"id":"k1","name":"renamed","prefix":"rzv_ab","scopes":["jobs:read"],"created_at":"a","last_used_at":null,"revoked_at":null,"expires_at":null}}`)
	if code, out, _ := h.run("api-keys", "rename", "k1", "--name", "renamed"); code != 0 || !strings.Contains(out, "Renamed API key k1 to renamed.") {
		t.Fatalf("rename: %d %q", code, out)
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
