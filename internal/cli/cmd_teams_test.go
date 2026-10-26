package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

const teamJSON = `{"team_id":"team_1","name":"Release","description":"Ships builds","system_kind":null,"locked":false,"permissions":["jobs:read","logs:read"],"member_count":1,"members":[{"identity_id":"idn_1","email":"ada@example.com","added_by":null,"created_at":"2026-10-20T00:00:00Z"}],"created_at":"2026-10-20T00:00:00Z","updated_at":"2026-10-20T00:00:00Z"}`

func TestPermissions(t *testing.T) {
	h := selectedHarness(t)
	h.json("GET /v1/customer/tenants/t-1/permissions", 200, `{"catalogue":[{"name":"jobs:read","title":"View jobs","description":"See jobs.","group":"Jobs"},{"name":"members:write","title":"Manage members","description":"Invite people.","group":"Members"}],"presets":[{"name":"member","title":"Member","permissions":["jobs:read"]}],"permissions":["jobs:read"]}`)
	code, out, _ := h.run("permissions")
	if code != 0 || !strings.Contains(out, "Your permissions in workspace t-1: jobs:read") || !strings.Contains(out, "* jobs:read: See jobs.") || !strings.Contains(out, "  members:write: Invite people.") {
		t.Fatalf("permissions: %d %q", code, out)
	}
	code, out, _ = h.run("permissions", "--json")
	var got struct {
		WorkspaceID string   `json:"workspace_id"`
		Permissions []string `json:"permissions"`
		Catalogue   []any    `json:"catalogue"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &got) != nil || got.WorkspaceID != "t-1" || len(got.Permissions) != 1 || len(got.Catalogue) != 2 {
		t.Fatalf("permissions --json: %d %q", code, out)
	}
	h.json("GET /v1/customer/tenants/t-1/permissions", 200, `{"presets":[]}`)
	if code, _, _ := h.run("permissions"); code == 0 {
		t.Fatal("a response without the catalogue was accepted")
	}
	if code, _, _ := h.run("permissions", "extra"); code != exitUsage {
		t.Fatalf("extra argument: %d", code)
	}
}

func TestTeamsReads(t *testing.T) {
	h := selectedHarness(t)
	h.json("GET /v1/customer/tenants/t-1/teams", 200, `{"teams":[{"team_id":"team_o","name":"Owners","description":"","system_kind":"owners","locked":true,"permissions":["members:write"],"member_count":1,"created_at":"a","updated_at":"a"},`+strings.Replace(teamJSON, `"members":[{"identity_id":"idn_1","email":"ada@example.com","added_by":null,"created_at":"2026-10-20T00:00:00Z"}],`, "", 1)+`]}`)
	code, out, _ := h.run("teams", "list")
	if code != 0 || !strings.Contains(out, "- Owners [team_o] 1 member, system team") || !strings.Contains(out, "- Release [team_1] 1 member\n") && !strings.HasSuffix(out, "- Release [team_1] 1 member\n") {
		t.Fatalf("teams list: %d %q", code, out)
	}
	h.json("GET /v1/customer/tenants/t-1/teams", 200, `{"teams":[]}`)
	if code, out, _ := h.run("teams", "list"); code != 0 || !strings.Contains(out, "No teams in workspace t-1.") {
		t.Fatalf("empty list: %d %q", code, out)
	}
	h.json("GET /v1/customer/tenants/t-1/teams/team_1", 200, `{"team":`+teamJSON+`}`)
	code, out, _ = h.run("teams", "show", "team_1")
	if code != 0 || !strings.Contains(out, "Release [team_1]") || !strings.Contains(out, "Permissions: jobs:read, logs:read") || !strings.Contains(out, "- idn_1 (ada@example.com)") {
		t.Fatalf("teams show: %d %q", code, out)
	}
	h.json("GET /v1/customer/tenants/t-1/teams/missing", 404, `{"error":{"code":"not_found"}}`)
	if code, _, errOut := h.run("teams", "show", "missing"); code != exitNotFound || !strings.Contains(errOut, "team was not found") {
		t.Fatalf("missing team: %d %q", code, errOut)
	}
	h.json("GET /v1/customer/tenants/t-1/teams", 403, `{"error":{"code":"permission_required"}}`)
	if code, _, errOut := h.run("teams", "list"); code != exitAccessDenied || !strings.Contains(errOut, "does not allow this change") {
		t.Fatalf("permission refusal: %d %q", code, errOut)
	}
}

func TestTeamsWrites(t *testing.T) {
	h := selectedHarness(t)
	var bodies []map[string]any
	record := func(status int, response string) func(http.ResponseWriter, *http.Request) {
		return func(w http.ResponseWriter, r *http.Request) {
			data, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(data, &body)
			bodies = append(bodies, body)
			w.WriteHeader(status)
			_, _ = w.Write([]byte(response))
		}
	}
	h.routes["POST /v1/customer/tenants/t-1/teams"] = record(201, `{"team":`+teamJSON+`}`)
	code, out, _ := h.run("teams", "create", "--name", "Release", "--description", "Ships builds", "--permissions", "jobs:read, logs:read")
	if code != 0 || !strings.Contains(out, "Created team Release [team_1].") {
		t.Fatalf("create: %d %q", code, out)
	}
	perms, _ := bodies[0]["permissions"].([]any)
	if bodies[0]["name"] != "Release" || bodies[0]["description"] != "Ships builds" || len(perms) != 2 || perms[1] != "logs:read" {
		t.Fatalf("create body = %v", bodies[0])
	}
	// A team with no permissions is allowed, and a repeated name is a usage error.
	if code, _, _ := h.run("teams", "create", "--name", "Empty", "--permissions", ","); code != 0 {
		t.Fatalf("empty permissions: %d", code)
	}
	if perms, _ := bodies[1]["permissions"].([]any); perms == nil || len(perms) != 0 {
		t.Fatalf("empty permissions body = %v, want []", bodies[1])
	}
	for _, args := range [][]string{
		{"teams", "create", "--name", "X", "--permissions", "jobs:read,jobs:read"},
		{"teams", "create", "--permissions", "jobs:read"},
		{"teams", "create", "--name", "X"},
		{"teams", "rename", "team_1"},
		{"teams", "bogus"},
	} {
		if code, _, _ := h.run(args...); code != exitUsage {
			t.Errorf("%v accepted: %d", args, code)
		}
	}
	if len(bodies) != 2 {
		t.Fatalf("a usage error reached the API: %v", bodies)
	}

	h.routes["PATCH /v1/customer/tenants/t-1/teams/team_1"] = record(200, `{"team":`+teamJSON+`}`)
	if code, out, _ := h.run("teams", "rename", "team_1", "--name", "Release crew"); code != 0 || !strings.Contains(out, "Updated team") {
		t.Fatalf("rename: %d %q", code, out)
	}
	if last := bodies[len(bodies)-1]; last["name"] != "Release crew" || last["description"] != nil {
		t.Fatalf("rename body = %v, want only name set", last)
	}
	h.routes["PUT /v1/customer/tenants/t-1/teams/team_1/grants"] = record(200, `{"team":`+teamJSON+`}`)
	if code, out, _ := h.run("teams", "permissions", "team_1", "--permissions", "jobs:read,logs:read"); code != 0 || !strings.Contains(out, "Team Release now has: jobs:read, logs:read.") {
		t.Fatalf("permissions: %d %q", code, out)
	}
	h.routes["POST /v1/customer/tenants/t-1/teams/team_1/members"] = record(200, `{"team":`+teamJSON+`}`)
	if code, out, _ := h.run("teams", "add-member", "team_1", "idn_9"); code != 0 || !strings.Contains(out, "Added idn_9 to team Release.") {
		t.Fatalf("add-member: %d %q", code, out)
	}
	if last := bodies[len(bodies)-1]; last["identity_id"] != "idn_9" {
		t.Fatalf("add-member body = %v", last)
	}
	h.routes["DELETE /v1/customer/tenants/t-1/teams/team_1/members/idn_9"] = func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }
	if code, out, _ := h.run("teams", "remove-member", "team_1", "idn_9", "--json"); code != 0 || !strings.Contains(out, `"removed_identity_id":"idn_9"`) {
		t.Fatalf("remove-member: %d %q", code, out)
	}
	h.routes["DELETE /v1/customer/tenants/t-1/teams/team_1"] = func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }
	if code, out, _ := h.run("teams", "delete", "team_1", "--json"); code != 0 || !strings.Contains(out, `"deleted_team_id":"team_1"`) {
		t.Fatalf("delete: %d %q", code, out)
	}
}

func TestTeamsRefusals(t *testing.T) {
	h := selectedHarness(t)
	for _, check := range []struct {
		status        int
		code, message string
		exit          int
	}{
		{403, "grant_ceiling", "only grant permissions you hold", exitAccessDenied},
		{403, "owner_required", "workspace owner", exitAccessDenied},
		{409, "team_locked", "team_locked", exitFailure},
		{409, "team_name_taken", "team_name_taken", exitFailure},
	} {
		h.json("POST /v1/customer/tenants/t-1/teams", check.status, `{"error":{"code":"`+check.code+`"}}`)
		code, _, errOut := h.run("teams", "create", "--name", "X", "--permissions", "jobs:read")
		if code != check.exit || !strings.Contains(errOut, check.message) {
			t.Errorf("%s: %d %q", check.code, code, errOut)
		}
	}
}

func TestTeamsAddNonMember(t *testing.T) {
	h := selectedHarness(t)
	h.json("POST /v1/customer/tenants/t-1/teams/team_1/members", 404, `{"error":{"code":"not_a_member"}}`)
	if code, _, errOut := h.run("teams", "add-member", "team_1", "idn_x"); code != exitNotFound || !strings.Contains(errOut, "not a member of this workspace") {
		t.Fatalf("non-member: %d %q", code, errOut)
	}
}
