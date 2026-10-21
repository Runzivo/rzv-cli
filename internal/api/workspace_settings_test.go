package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWorkspaceSettingsRoutesAndBodies(t *testing.T) {
	var requests []struct {
		Method string
		Path   string
		Body   map[string]any
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		requests = append(requests, struct {
			Method, Path string
			Body         map[string]any
		}{r.Method, r.URL.EscapedPath(), body})
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.EscapedPath() {
		case "GET /v1/customer/tenants/a%2Fb/settings", "PATCH /v1/customer/tenants/a%2Fb/settings":
			_, _ = w.Write([]byte(`{"tenant_id":"a/b","name":"Name","logo":null,"can_update":true}`))
		case "GET /v1/customer/tenants/a%2Fb/members":
			_, _ = w.Write([]byte(`{"current_identity_id":"me","can_manage_invitations":true,"can_invite":true,"members":[]}`))
		case "PATCH /v1/customer/tenants/a%2Fb/members/u%2F1":
			_, _ = w.Write([]byte(`{"identity_id":"u/1","role":"admin","email":null,"can_change_role":true,"can_remove":true,"allowed_roles":["owner","admin","member"]}`))
		case "DELETE /v1/customer/tenants/a%2Fb/members/u%2F1", "DELETE /v1/customer/tenants/a%2Fb/invitations/i%2F1":
			w.WriteHeader(http.StatusNoContent)
		case "GET /v1/customer/tenants/a%2Fb/invitations":
			_, _ = w.Write([]byte(`{"invitations":[]}`))
		case "POST /v1/customer/tenants/a%2Fb/invitations", "POST /v1/customer/tenants/a%2Fb/invitations/i%2F1/resend", "PATCH /v1/customer/tenants/a%2Fb/invitations/i%2F1":
			_, _ = w.Write([]byte(`{"invitation_id":"i/1","email":"person@example.test","role":"member","status":"pending","expires_at":"x","created_at":"x","can_resend":true,"can_revoke":true,"teams":[]}`))
		case "POST /v1/customer/invitations/accept":
			_, _ = w.Write([]byte(`{"tenant_id":"a/b","tenant_slug":"a","tenant_name":"Name","role":"member"}`))
		default:
			t.Fatalf("unexpected route: %s %s", r.Method, r.URL.EscapedPath())
		}
	}))
	defer server.Close()

	client, err := New(server.URL, "credential", "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	name := "Renamed"
	if _, err = client.WorkspaceSettings(ctx, "a/b"); err != nil {
		t.Fatal(err)
	}
	if _, err = client.UpdateWorkspaceSettings(ctx, "a/b", WorkspaceSettingsPatch{Name: &name}); err != nil {
		t.Fatal(err)
	}
	if _, err = client.WorkspaceMembers(ctx, "a/b"); err != nil {
		t.Fatal(err)
	}
	if _, err = client.UpdateWorkspaceMemberRole(ctx, "a/b", "u/1", "admin"); err != nil {
		t.Fatal(err)
	}
	if err = client.RemoveWorkspaceMember(ctx, "a/b", "u/1"); err != nil {
		t.Fatal(err)
	}
	if _, err = client.WorkspaceInvitations(ctx, "a/b"); err != nil {
		t.Fatal(err)
	}
	if _, err = client.CreateWorkspaceInvitation(ctx, "a/b", CreateWorkspaceInvitationInput{Email: "person@example.test", Role: "member", TeamIDs: []string{"team-1"}}); err != nil {
		t.Fatal(err)
	}
	if _, err = client.ResendWorkspaceInvitation(ctx, "a/b", "i/1"); err != nil {
		t.Fatal(err)
	}
	if _, err = client.UpdateWorkspaceInvitationTeams(ctx, "a/b", "i/1", []string{}); err != nil {
		t.Fatal(err)
	}
	if err = client.RevokeWorkspaceInvitation(ctx, "a/b", "i/1"); err != nil {
		t.Fatal(err)
	}
	if _, err = client.AcceptWorkspaceInvitation(ctx, "secret-token"); err != nil {
		t.Fatal(err)
	}
	if got := requests[1].Body; got["name"] != "Renamed" || len(got) != 1 {
		t.Fatalf("settings patch: %#v", got)
	}
	if got := requests[3].Body; got["role"] != "admin" {
		t.Fatalf("member role: %#v", got)
	}
	if got := requests[6].Body; got["email"] != "person@example.test" || got["role"] != "member" {
		t.Fatalf("invitation: %#v", got)
	}
	got := requests[8].Body
	teams, ok := got["team_ids"].([]any)
	if !ok || len(teams) != 0 {
		t.Fatalf("clear teams: %#v", got)
	}
	if got := requests[10].Body; got["token"] != "secret-token" {
		t.Fatalf("accept: %#v", got)
	}
}

func TestWorkspaceSettingsRejectsMalformedRequiredResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"name":"missing"}`)) }))
	defer server.Close()
	client, err := New(server.URL, "credential", "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.WorkspaceSettings(context.Background(), "tenant"); err == nil {
		t.Fatal("malformed response accepted")
	}
}

func TestWorkspaceListsRejectEntriesWithoutIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/customer/tenants/tenant/members":
			_, _ = w.Write([]byte(`{"current_identity_id":"me","can_manage_invitations":false,"can_invite":false,"members":[{}]}`))
		case "/v1/customer/tenants/tenant/invitations":
			_, _ = w.Write([]byte(`{"invitations":[{}]}`))
		}
	}))
	defer server.Close()
	client, err := New(server.URL, "credential", "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.WorkspaceMembers(context.Background(), "tenant"); err == nil {
		t.Fatal("member without identity ID accepted")
	}
	if _, err = client.WorkspaceInvitations(context.Background(), "tenant"); err == nil {
		t.Fatal("invitation without ID accepted")
	}
}
