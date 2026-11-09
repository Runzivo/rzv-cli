package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// The service refuses to share a workspace whose type is confirmed as
// personal; every sharing command explains it in one line, exit 1.
func TestWorkspaceSharingRefusal(t *testing.T) {
	refuse := func(h *harness, route string) {
		h.routes[route] = func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":{"code":"personal_workspace_not_shared"}}`))
		}
	}
	cases := []struct {
		route string
		args  []string
	}{
		{"POST /v1/customer/tenants/t-1/invitations", []string{"workspaces", "invitations", "invite", "--email", "a@example.com", "--role", "member"}},
		{"POST /v1/customer/tenants/t-1/invitations/inv-1/resend", []string{"workspaces", "invitations", "resend", "inv-1"}},
		{"PATCH /v1/customer/tenants/t-1/invitations/inv-1", []string{"workspaces", "invitations", "teams", "inv-1", "--team-ids", "team-1"}},
		{"PATCH /v1/customer/tenants/t-1/members/id-1", []string{"workspaces", "members", "role", "id-1", "--role", "admin"}},
		{"POST /v1/customer/invitations/accept", []string{"workspaces", "invitations", "accept", "--token", "tok"}},
	}
	for _, tc := range cases {
		h := selectedHarness(t)
		refuse(h, tc.route)
		code, out, errOut := h.run(tc.args...)
		if code != exitFailure || out != "" || !strings.Contains(errOut, "A personal workspace is not shared: nobody can be invited, added or given more access. To work with others, use an organization workspace.") || strings.Count(errOut, "\n") != 1 {
			t.Fatalf("%v = %d %q %q", tc.args, code, out, errOut)
		}
		code, out, errOut = h.run(append(tc.args, "--json")...)
		var got struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if code != exitFailure || out != "" || json.Unmarshal([]byte(errOut), &got) != nil || got.Error != "personal_workspace_not_shared" || !strings.HasPrefix(got.Message, "A personal workspace is not shared") {
			t.Fatalf("%v --json = %d %q %q", tc.args, code, out, errOut)
		}
		h.env["LANG"] = "fr_CA.UTF-8"
		if code, _, errOut := h.run(tc.args...); code != exitFailure || !strings.Contains(errOut, "Un espace personnel n’est pas partagé : on ne peut y inviter ni y ajouter personne, ni y accorder plus d’accès. Pour travailler avec d’autres personnes, utilisez un espace d’organisation.") {
			t.Fatalf("%v fr-CA = %d %q", tc.args, code, errOut)
		}
	}
	// Other errors keep the shared mapping, such as the not-found message.
	h := selectedHarness(t)
	h.routes["POST /v1/customer/tenants/t-1/invitations/inv-1/resend"] = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"not_found"}}`))
	}
	if code, _, errOut := h.run("workspaces", "invitations", "resend", "inv-1"); code == exitFailure || !strings.Contains(errOut, "The invitation was not found in this workspace.") {
		t.Fatalf("not found = %d %q", code, errOut)
	}
}
