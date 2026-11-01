package cli

import (
	"strings"
	"testing"

	"github.com/Runzivo/rzv-cli/internal/api"
)

func TestMatchWorkspacePrecedenceAndAmbiguity(t *testing.T) {
	a := &app{Deps: Deps{Getenv: func(string) string { return "" }}}
	memberships := []api.Membership{
		{TenantID: "exact", TenantSlug: "first", TenantName: "Same"},
		{TenantID: "second", TenantSlug: "EXACT", TenantName: "Other Name"},
		{TenantID: "third", TenantSlug: "third-slug", TenantName: "same"},
		{TenantID: "fourth", TenantSlug: "FIRST", TenantName: "Fourth"},
	}
	for _, tc := range []struct {
		ref, want string
		ambiguous bool
	}{
		{"exact", "exact", false}, {"ExAcT", "second", false},
		{"tHiRd-SlUg", "third", false}, {"oThEr NaMe", "second", false},
		{"same", "", true}, {"First", "", true}, {"absent", "", false}, {"", "", false},
	} {
		t.Run(tc.ref, func(t *testing.T) {
			got, err := a.matchWorkspace(memberships, tc.ref)
			if got != tc.want || (err != nil) != tc.ambiguous {
				t.Fatalf("got %q, %v", got, err)
			}
			if tc.ambiguous && (!strings.Contains(err.Error(), "[exact]") || !strings.Contains(err.Error(), "More than one")) {
				t.Fatal(err)
			}
		})
	}
}

func TestWorkspaceNamesAcrossCommands(t *testing.T) {
	h := newHarness(t)
	h.json("GET /v1/customer/memberships", 200, `{"memberships":[{"tenant_id":"t-1","tenant_slug":"acme-slug","tenant_name":"Acme Team","role":"owner"}]}`)
	h.json("GET /v1/customer/tenants/t-1/activity", 200, cliActivityJSON)
	if code, _, e := loginAs(h, "--workspace", "ACME TEAM"); code != 0 {
		t.Fatal(code, e)
	}
	for _, ref := range []string{"t-1", "AcMe-SlUg", "acme team"} {
		if code, out, e := h.run("workspaces", "select", ref); code != 0 || !strings.Contains(out, "Acme Team [t-1]") {
			t.Fatal(code, out, e)
		}
	}
	if code, out, e := h.run("whoami"); code != 0 || !strings.Contains(out, "Acme Team [t-1]") {
		t.Fatal(code, out, e)
	}
	if code, out, e := h.run("workspaces", "list"); code != 0 || !strings.Contains(out, "acme-slug") {
		t.Fatal(code, out, e)
	}
	if code, _, e := h.run("activity", "list", "--workspace", "ACME TEAM"); code != 0 {
		t.Fatal(code, e)
	}
	h.env["RZV_WORKSPACE"] = "acme team"
	if code, _, e := h.run("activity", "list"); code != 0 {
		t.Fatal(code, e)
	}
	h.env["LANG"] = "fr_CA.UTF-8"
	if code, out, e := h.run("workspaces", "select", "Acme Team"); code != 0 || !strings.Contains(out, "Espace de travail sélectionné") {
		t.Fatal(code, out, e)
	}
}

func TestAmbiguousWorkspaceDoesNotChangeSelectionOrCallWorkspace(t *testing.T) {
	h := selectedHarness(t)
	h.json("GET /v1/customer/memberships", 200, `{"memberships":[{"tenant_id":"t-1","tenant_slug":"one","tenant_name":"Shared Name","role":"owner"},{"tenant_id":"t-2","tenant_slug":"two","tenant_name":"Shared Name","role":"owner"}]}`)
	for _, args := range [][]string{
		{"workspaces", "select", "SHARED NAME"},
		{"activity", "list", "--workspace", "Shared Name"},
	} {
		if code, _, e := h.run(args...); code != exitUsage || !strings.Contains(e, "[t-1]") || !strings.Contains(e, "[t-2]") {
			t.Fatal(code, e)
		}
	}
	h.env["RZV_WORKSPACE"] = "Shared Name"
	if code, _, e := h.run("activity", "list"); code != exitUsage {
		t.Fatal(code, e)
	}
	if code, _, e := loginAs(h, "--workspace", "Shared Name"); code != exitUsage {
		t.Fatal(code, e)
	}
	creds, err := h.store.Read()
	if err != nil || creds.SelectedWorkspace != "t-1" {
		t.Fatal("ambiguous selection changed credentials")
	}
	if code, _, _ := h.run("workspaces", "select", "missing"); code != exitAccessDenied {
		t.Fatal(code)
	}
}

func TestWorkspaceAmbiguityFrenchAndEscaping(t *testing.T) {
	a := &app{Deps: Deps{Getenv: func(string) string { return "fr_CA.UTF-8" }}}
	_, err := a.matchWorkspace([]api.Membership{
		{TenantID: "t-1", TenantName: "Same\nName"},
		{TenantID: "t-2", TenantName: "Same\nName"},
	}, "Same\nName")
	if err == nil || !strings.Contains(err.Error(), "Plusieurs espaces de travail") || !strings.Contains(err.Error(), `Same\u000aName [t-2]`) {
		t.Fatal(err)
	}
}
