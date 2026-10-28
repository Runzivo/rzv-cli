package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// loginAs signs in with the fake provider, passing extra arguments such as
// -p <profile> and --workspace.
func loginAs(h *harness, extra ...string) (int, string, string) {
	args := append([]string{"login", "--issuer", h.provider.Issuer(), "--api-base-url", h.api.URL, "--allow-insecure-loopback", "--json"}, extra...)
	return h.run(args...)
}

func TestParseProfileAndWorkspaceShortOptions(t *testing.T) {
	cmd, err := parse([]string{"whoami", "-p", "work", "--json"})
	if err != nil || cmd.profile != "work" || !cmd.hasProfile || len(cmd.options) != 0 {
		t.Fatalf("-p: %+v, %v", cmd, err)
	}
	cmd, err = parse([]string{"activity", "list", "-w", "acme"})
	if err != nil || cmd.options["workspace"] != "acme" {
		t.Fatalf("-w: %+v, %v", cmd, err)
	}
	cmd, err = parse([]string{"whoami", "--profile=personal"})
	if err != nil || cmd.profile != "personal" {
		t.Fatalf("--profile=: %+v, %v", cmd, err)
	}
	if _, err := parse([]string{"whoami", "-p"}); err == nil {
		t.Fatal("-p without a value parsed")
	}
}

func TestProfilesKeepSeparateSignIns(t *testing.T) {
	h := newHarness(t)
	if code, _, errOut := loginAs(h, "-p", "work"); code != 0 {
		t.Fatalf("login work: %d %s", code, errOut)
	}
	workCreds, err := h.store.ForProfile("work").Read()
	if err != nil || workCreds == nil {
		t.Fatalf("work sign-in not stored: %v", err)
	}
	code, out, errOut := loginAs(h, "-p", "personal", "--workspace", "acme")
	if code != 0 {
		t.Fatalf("login personal: %d %s", code, errOut)
	}
	var signedIn map[string]any
	if err := json.Unmarshal([]byte(out), &signedIn); err != nil || signedIn["profile"] != "personal" || signedIn["selected_workspace_id"] != "t-1" {
		t.Fatalf("login output = %q, %v", out, err)
	}
	again, err := h.store.ForProfile("work").Read()
	if err != nil || again == nil || again.AccessToken != workCreds.AccessToken || again.SelectedWorkspace != "" {
		t.Fatal("signing in to personal changed the work profile")
	}
	if root, _ := h.store.Read(); root != nil {
		t.Fatal("named profiles wrote the default credentials file")
	}

	code, out, errOut = h.run("profiles", "list", "--json")
	if code != 0 || errOut != "" {
		t.Fatalf("profiles list: %d %s", code, errOut)
	}
	if strings.Contains(out, workCreds.AccessToken) {
		t.Fatal("profiles list printed a token")
	}
	var listed struct {
		ActiveProfile string `json:"active_profile"`
		Profiles      []struct {
			Name   string `json:"name"`
			Active bool   `json:"active"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal([]byte(out), &listed); err != nil {
		t.Fatal(err)
	}
	if listed.ActiveProfile != "personal" || len(listed.Profiles) != 2 || listed.Profiles[0].Name != "personal" || listed.Profiles[1].Name != "work" {
		t.Fatalf("profiles list = %+v", listed)
	}
}

func TestProfileSelectionOrder(t *testing.T) {
	h := newHarness(t)
	loginAs(h, "-p", "work")
	loginAs(h, "-p", "personal", "--workspace", "acme")
	profileOf := func(args ...string) string {
		t.Helper()
		code, out, errOut := h.run(append([]string{"whoami", "--json"}, args...)...)
		if code != 0 {
			t.Fatalf("whoami %v: %d %s", args, code, errOut)
		}
		var v struct {
			Profile string `json:"profile"`
		}
		if err := json.Unmarshal([]byte(out), &v); err != nil {
			t.Fatal(err)
		}
		return v.Profile
	}
	if got := profileOf(); got != "personal" {
		t.Fatalf("active profile = %q, want personal", got)
	}
	h.env["RZV_PROFILE"] = "work"
	if got := profileOf(); got != "work" {
		t.Fatalf("RZV_PROFILE = %q, want work", got)
	}
	if got := profileOf("-p", "personal"); got != "personal" {
		t.Fatalf("-p over RZV_PROFILE = %q, want personal", got)
	}
	h.env["RZV_PROFILE"] = ""
	before := len(h.seenAuth)
	if code, _, errOut := h.run("whoami", "-p", "Bad_Name"); code != exitUsage || !strings.Contains(errOut, "lowercase") {
		t.Fatalf("invalid profile: %d %s", code, errOut)
	}
	if len(h.seenAuth) != before {
		t.Fatal("invalid profile name reached the API")
	}
}

func TestProfilesUseAndRemove(t *testing.T) {
	h := newHarness(t)
	loginAs(h, "-p", "work")
	loginAs(h, "-p", "personal")
	if code, _, errOut := h.run("profiles", "use", "nope"); code == 0 || !strings.Contains(errOut, "No sign-in for profile nope") {
		t.Fatalf("use unknown: %d %s", code, errOut)
	}
	if code, out, _ := h.run("profiles", "use", "work"); code != 0 || !strings.Contains(out, "Active profile: work") {
		t.Fatalf("use work: %d %q", code, out)
	}
	if active, _ := h.store.ActiveProfile(); active != "work" {
		t.Fatalf("active = %q", active)
	}
	if code, _, errOut := h.run("profiles", "remove", "work"); code != 0 {
		t.Fatalf("remove work: %d %s", code, errOut)
	}
	if creds, _ := h.store.ForProfile("work").Read(); creds != nil {
		t.Fatal("removed profile still has credentials")
	}
	if active, _ := h.store.ActiveProfile(); active != "" {
		t.Fatalf("removing the active profile left it active: %q", active)
	}
	if code, _, _ := h.run("profiles", "remove", "work"); code == 0 {
		t.Fatal("second remove succeeded")
	}
	if code, _, errOut := h.run("profiles", "remove"); code != exitUsage {
		t.Fatalf("remove without a name: %d %s", code, errOut)
	}
}

func TestWorkspaceSlugAndRZVWorkspaceDefault(t *testing.T) {
	h := newHarness(t)
	h.json("GET /v1/customer/tenants/t-1/activity", 200, cliActivityJSON)
	loginAs(h, "-p", "work")
	if code, _, errOut := h.run("activity", "list", "--workspace", "acme"); code != 0 {
		t.Fatalf("--workspace slug: %d %s", code, errOut)
	}
	if code, _, errOut := h.run("activity", "list", "-w", "acme"); code != 0 {
		t.Fatalf("-w slug: %d %s", code, errOut)
	}
	h.env["RZV_WORKSPACE"] = "acme"
	if code, _, errOut := h.run("activity", "list"); code != 0 {
		t.Fatalf("RZV_WORKSPACE slug: %d %s", code, errOut)
	}
	if code, _, _ := h.run("activity", "list", "--workspace", "nope"); code != exitAccessDenied {
		t.Fatalf("unknown workspace: %d", code)
	}
}

func TestLoginWithUnknownWorkspaceSavesNothing(t *testing.T) {
	h := newHarness(t)
	if code, _, _ := loginAs(h, "-p", "fresh", "--workspace", "nope"); code == 0 {
		t.Fatal("login to an unknown workspace succeeded")
	}
	if creds, _ := h.store.ForProfile("fresh").Read(); creds != nil {
		t.Fatal("a refused workspace still saved the profile")
	}
	if active, _ := h.store.ActiveProfile(); active != "" {
		t.Fatalf("a refused workspace changed the active profile: %q", active)
	}
}
