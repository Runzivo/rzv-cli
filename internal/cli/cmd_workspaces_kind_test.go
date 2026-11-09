package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

const kindRoute = "POST /v1/customer/tenants/t-1/kind"

// kindServer answers the kind route and records the last body.
func kindServer(h *harness, seen *map[string]any, status int, body string) {
	h.routes[kindRoute] = func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		*seen = nil
		_ = json.Unmarshal(raw, seen)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

func TestConfirmKindDryRun(t *testing.T) {
	h := selectedHarness(t)
	var seen map[string]any
	for _, tc := range []struct{ kind, want string }{
		{"personal", "Workspace t-1 would be confirmed as a personal workspace. Nothing was changed."},
		{"organization", "Workspace t-1 would be confirmed as an organization workspace. Nothing was changed."},
	} {
		kindServer(h, &seen, 200, `{"kind":"`+tc.kind+`","kind_source":"declared"}`)
		code, out, errOut := h.run("workspaces", "confirm-kind", "--kind", tc.kind)
		if code != 0 || errOut != "" || !strings.Contains(out, tc.want) || !strings.Contains(out, "Once the type is confirmed, only Runzivo staff can change it.") || !strings.Contains(out, "Run again with --yes to confirm.") {
			t.Fatalf("%s dry run: %d %q %q", tc.kind, code, out, errOut)
		}
		// The default of confirm is false, so the field is left out.
		if len(seen) != 1 || seen["kind"] != tc.kind {
			t.Fatalf("%s dry run body = %#v", tc.kind, seen)
		}
	}
	kindServer(h, &seen, 200, `{"kind":"personal","kind_source":"declared"}`)
	// A slug resolves like every other workspace command.
	if code, out, _ := h.run("workspaces", "confirm-kind", "--kind", "personal", "--workspace", "acme"); code != 0 || !strings.Contains(out, "Workspace t-1 would") {
		t.Fatalf("by slug: %d %q", code, out)
	}
	code, out, _ := h.run("workspaces", "confirm-kind", "--kind", "personal", "--json")
	var got struct {
		WorkspaceID string `json:"workspace_id"`
		Confirmed   bool   `json:"confirmed"`
		Kind        struct {
			Kind       string `json:"kind"`
			KindSource string `json:"kind_source"`
			ChangeID   string `json:"change_id"`
		} `json:"workspace_kind"`
	}
	if err := json.Unmarshal([]byte(out), &got); code != 0 || err != nil || got.WorkspaceID != "t-1" || got.Confirmed || got.Kind.Kind != "personal" || got.Kind.KindSource != "declared" || got.Kind.ChangeID != "" || strings.Contains(out, "change_id") {
		t.Fatalf("dry run --json: %d %q %v", code, out, err)
	}
	kindServer(h, &seen, 200, `{"kind":"organization","kind_source":"declared"}`)
	h.env["LANG"] = "fr_CA.UTF-8"
	code, out, _ = h.run("workspaces", "confirm-kind", "--kind", "organization")
	if code != 0 || !strings.Contains(out, "serait confirmé comme espace d’organisation. Rien n’a été modifié.") || !strings.Contains(out, "Une fois le type confirmé, seul le personnel de Runzivo peut le modifier.") || !strings.Contains(out, "Relancez la commande avec --yes pour confirmer.") {
		t.Fatalf("fr-CA dry run: %d %q", code, out)
	}
}

func TestConfirmKindConfirm(t *testing.T) {
	h := selectedHarness(t)
	var seen map[string]any
	for _, tc := range []struct{ kind, want string }{
		{"personal", "Confirmed workspace t-1 as a personal workspace (change wkc_0123456789abcdef01234567). Only Runzivo staff can change the type now."},
		{"organization", "Confirmed workspace t-1 as an organization workspace (change wkc_0123456789abcdef01234567). Only Runzivo staff can change the type now."},
	} {
		kindServer(h, &seen, 200, `{"kind":"`+tc.kind+`","kind_source":"declared","change_id":"wkc_0123456789abcdef01234567"}`)
		code, out, errOut := h.run("workspaces", "confirm-kind", "--kind", tc.kind, "--yes")
		if code != 0 || errOut != "" || !strings.Contains(out, tc.want) || strings.Contains(out, "Run again") {
			t.Fatalf("%s confirm: %d %q %q", tc.kind, code, out, errOut)
		}
		if len(seen) != 2 || seen["kind"] != tc.kind || seen["confirm"] != true {
			t.Fatalf("%s confirm body = %#v", tc.kind, seen)
		}
	}
	code, out, _ := h.run("workspaces", "confirm-kind", "--kind", "organization", "--yes", "--json")
	var got struct {
		WorkspaceID string `json:"workspace_id"`
		Confirmed   bool   `json:"confirmed"`
		Kind        struct {
			Kind       string `json:"kind"`
			KindSource string `json:"kind_source"`
			ChangeID   string `json:"change_id"`
		} `json:"workspace_kind"`
	}
	if err := json.Unmarshal([]byte(out), &got); code != 0 || err != nil || got.WorkspaceID != "t-1" || !got.Confirmed || got.Kind.Kind != "organization" || got.Kind.KindSource != "declared" || got.Kind.ChangeID != "wkc_0123456789abcdef01234567" {
		t.Fatalf("confirm --json: %d %q %v", code, out, err)
	}
	kindServer(h, &seen, 200, `{"kind":"personal","kind_source":"declared","change_id":"wkc_0123456789abcdef01234567"}`)
	h.env["LANG"] = "fr_CA.UTF-8"
	code, out, _ = h.run("workspaces", "confirm-kind", "--kind", "personal", "--yes")
	if code != 0 || !strings.Contains(out, "a été confirmé comme espace personnel (modification wkc_0123456789abcdef01234567). Seul le personnel de Runzivo peut maintenant en modifier le type.") {
		t.Fatalf("fr-CA confirm: %d %q", code, out)
	}
}

func TestConfirmKindRejectsIncompleteAnswers(t *testing.T) {
	h := selectedHarness(t)
	var seen map[string]any
	for _, tc := range []struct {
		args []string
		body string
	}{
		{[]string{"--kind", "personal"}, `{}`},
		{[]string{"--kind", "personal"}, `{"kind":"organization","kind_source":"declared"}`},
		{[]string{"--kind", "personal", "--yes"}, `{"kind":"personal","kind_source":"declared"}`},
		{[]string{"--kind", "personal", "--yes"}, `{"kind":"personal","kind_source":"declared","change_id":"wkc_0123"}`},
		{[]string{"--kind", "personal", "--yes"}, `{"kind":"personal","kind_source":"declared","change_id":"wkc_0123456789ABCDEF01234567"}`},
		{[]string{"--kind", "personal", "--yes"}, `{"kind":"personal","kind_source":"inferred","change_id":"wkc_0123456789abcdef01234567"}`},
		{[]string{"--kind", "personal"}, `{"kind":"personal","kind_source":"staff"}`},
		{[]string{"--kind", "personal"}, `{"kind":"personal","kind_source":"declared","change_id":"wkc_0123456789abcdef01234567"}`},
	} {
		kindServer(h, &seen, 200, tc.body)
		if code, _, _ := h.run(append([]string{"workspaces", "confirm-kind"}, tc.args...)...); code == 0 {
			t.Fatalf("%v accepted %s", tc.args, tc.body)
		}
	}
}

func TestConfirmKindRefusals(t *testing.T) {
	h := selectedHarness(t)
	var seen map[string]any
	for _, tc := range []struct {
		status int
		code   string
		exit   int
		want   string
		json   string
	}{
		{400, "invalid_request", exitUsage, "--kind must be personal or organization", "invalid_request"},
		{403, "tenant_membership_required", exitAccessDenied, "You are not a member of this workspace.", "tenant_membership_required"},
		{403, "workspace_owner_required", exitAccessDenied, "Only a workspace owner can confirm the workspace type.", "workspace_owner_required"},
		{403, "reauth_required", exitAuth, "Sign in again with `rzv login`, then retry.", "reauth_required"},
		{403, "api_key_scope_required", exitAccessDenied, "API keys cannot do it", "api_key_scope_required"},
		{409, "kind_already_confirmed", exitFailure, "type is already confirmed. Only Runzivo staff can change it.", "kind_already_confirmed"},
		{409, "personal_workspace_exists", exitFailure, "so this one cannot also be personal. Use --kind organization only if this workspace belongs to an organization. Otherwise, contact Runzivo support before confirming.", "personal_workspace_exists"},
		{409, "personal_workspace_has_members", exitFailure, "and this workspace has more. Use --kind organization only if this workspace belongs to an organization. Otherwise, contact Runzivo support before confirming.", "personal_workspace_has_members"},
		{409, "personal_requires_user_account", exitFailure, "so it cannot be personal. Use --kind organization only if this workspace belongs to an organization. Otherwise, contact Runzivo support before confirming.", "personal_requires_user_account"},
		{404, "not_found", exitUnavailable, "not available in this Runzivo environment yet", "not_found"},
		{500, "internal_error", exitUnavailable, "500 internal_error", "internal_error"},
	} {
		for _, extra := range [][]string{nil, {"--yes"}} {
			kindServer(h, &seen, tc.status, `{"error":{"code":"`+tc.code+`"}}`)
			args := append([]string{"workspaces", "confirm-kind", "--kind", "personal"}, extra...)
			code, out, errOut := h.run(args...)
			if code != tc.exit || out != "" || !strings.Contains(errOut, tc.want) {
				t.Fatalf("%s %v = %d %q %q", tc.code, extra, code, out, errOut)
			}
			code, out, errOut = h.run(append(args, "--json")...)
			if code != tc.exit || out != "" || !strings.Contains(errOut, `"error":"`+tc.json+`"`) {
				t.Fatalf("%s %v --json = %d %q %q", tc.code, extra, code, out, errOut)
			}
		}
	}
	h.env["LANG"] = "fr_CA.UTF-8"
	kindServer(h, &seen, 409, `{"error":{"code":"personal_workspace_exists"}}`)
	if code, _, errOut := h.run("workspaces", "confirm-kind", "--kind", "personal", "--yes"); code != exitFailure || !strings.Contains(errOut, "ne peut donc pas l’être aussi. Utilisez --kind organization uniquement si cet espace appartient à une organisation. Sinon, communiquez avec le soutien de Runzivo avant de confirmer.") || strings.Count(errOut, "\n") != 1 {
		t.Fatalf("fr-CA personal refusal = %d %q", code, errOut)
	}
	kindServer(h, &seen, 409, `{"error":{"code":"kind_already_confirmed"}}`)
	if code, _, errOut := h.run("workspaces", "confirm-kind", "--kind", "personal", "--yes"); code != exitFailure || !strings.Contains(errOut, "déjà confirmé") {
		t.Fatalf("fr-CA refusal = %d %q", code, errOut)
	}
}

func TestConfirmKindAPIKey(t *testing.T) {
	h := newHarness(t)
	h.env["RZV_API_KEY"] = "rzv_test_key"
	h.env["RZV_API_URL"] = h.api.URL
	h.json("POST /v1/customer/tenants/t-5/kind", 403, `{"error":{"code":"api_key_scope_required"}}`)
	code, _, errOut := h.run("workspaces", "confirm-kind", "--kind", "personal", "--yes", "--workspace", "t-5")
	if code != exitAccessDenied || !strings.Contains(errOut, "rzv login") {
		t.Fatalf("api key = %d %q", code, errOut)
	}
}

func TestConfirmKindValidatesBeforeHTTP(t *testing.T) {
	h := selectedHarness(t)
	before := len(h.seenAuth)
	for _, args := range [][]string{
		{"workspaces", "confirm-kind"},
		{"workspaces", "confirm-kind", "--kind", "team"},
		{"workspaces", "confirm-kind", "--kind", "Personal"},
		{"workspaces", "confirm-kind", "--kind", " "},
		{"workspaces", "confirm-kind", "--kind", "personal", "extra"},
		{"workspaces", "confirm-kind", "--kind", "personal", "--bogus", "x"},
		{"workspaces", "confirm-kind", "--kind", "personal", "--yes=true"},
		{"workspaces", "confirm-kind", "--kind", "personal", "--yes=false"},
		{"workspaces", "confirm-kind", "--kind", "personal", "-y"},
		{"workspaces", "confirm-kind", "--kind", "personal", "--yes", "--yes"},
	} {
		if code, _, errOut := h.run(args...); code != exitUsage {
			t.Fatalf("%v = %d %q; want usage", args, code, errOut)
		}
	}
	if len(h.seenAuth) != before {
		t.Fatalf("validation made HTTP requests: %v", h.seenAuth[before:])
	}
	if _, _, errOut := h.run("workspaces", "confirm-kind", "--kind", "team"); !strings.Contains(errOut, "--kind must be personal or organization.") {
		t.Fatalf("message = %q", errOut)
	}
	h.env["LANG"] = "fr_CA.UTF-8"
	if _, _, errOut := h.run("workspaces", "confirm-kind", "--kind", "team"); !strings.Contains(errOut, "--kind doit être personal ou organization.") {
		t.Fatalf("fr-CA message = %q", errOut)
	}
	if len(h.seenAuth) != before {
		t.Fatalf("validation made HTTP requests: %v", h.seenAuth[before:])
	}
}

// Server-supplied text must not reach the terminal as control characters:
// human output escapes it, and --json keeps JSON escaping. The workspace id
// is the only server string the messages print.
func TestConfirmKindEscapesServerText(t *testing.T) {
	const esc = "\x1b"
	h := selectedHarness(t)
	h.json("GET /v1/customer/memberships", 200, `{"memberships":[{"tenant_id":"t-9\u001b[2Jx","tenant_slug":"nine","tenant_name":"Nine","role":"owner"}]}`)
	var seen map[string]any
	kindServer(h, &seen, 200, `{"kind":"personal","kind_source":"declared","change_id":"wkc_0123456789abcdef01234567"}`)
	h.routes["POST /v1/customer/tenants/t-9\x1b[2Jx/kind"] = h.routes[kindRoute]
	for _, args := range [][]string{{"--yes"}, {}} {
		if len(args) == 0 {
			kindServer(h, &seen, 200, `{"kind":"personal","kind_source":"declared"}`)
			h.routes["POST /v1/customer/tenants/t-9\x1b[2Jx/kind"] = h.routes[kindRoute]
		}
		code, out, errOut := h.run(append([]string{"workspaces", "confirm-kind", "--kind", "personal", "--workspace", "nine"}, args...)...)
		if code != 0 || strings.Contains(out, esc) || !strings.Contains(out, `t-9\u001b[2Jx`) {
			t.Fatalf("human output = %d %q %q", code, out, errOut)
		}
	}
	kindServer(h, &seen, 200, `{"kind":"personal","kind_source":"declared","change_id":"wkc_0123456789abcdef01234567"}`)
	h.routes["POST /v1/customer/tenants/t-9\x1b[2Jx/kind"] = h.routes[kindRoute]
	code, out, _ := h.run("workspaces", "confirm-kind", "--kind", "personal", "--workspace", "nine", "--yes", "--json")
	if code != 0 || strings.Contains(out, esc) || !json.Valid([]byte(out)) || !strings.Contains(out, `t-9\u001b[2Jx`) {
		t.Fatalf("--json = %d %q", code, out)
	}
}

func TestWorkspacesListShowsKind(t *testing.T) {
	h := selectedHarness(t)
	h.json("GET /v1/customer/memberships", 200, `{"memberships":[
		{"tenant_id":"t-1","tenant_slug":"acme","tenant_name":"Acme","role":"owner","kind":"personal","kind_source":"inferred"},
		{"tenant_id":"t-2","tenant_slug":"beta","tenant_name":"Beta","role":"member","kind":"organization","kind_source":"declared"},
		{"tenant_id":"t-3","tenant_slug":"gamma","tenant_name":"Gamma","role":"owner","kind":"organization","kind_source":"staff"},
		{"tenant_id":"t-4","tenant_slug":"delta","tenant_name":"Delta","role":"owner","kind":"x\u001b[2J","kind_source":"declared"},
		{"tenant_id":"t-5","tenant_slug":"eps","tenant_name":"Eps","role":"owner"}]}`)
	code, out, _ := h.run("workspaces", "list")
	for _, want := range []string{
		"- Acme [t-1] (acme, owner), personal (unconfirmed) (selected)",
		"- Beta [t-2] (beta, member), organization\n",
		"- Gamma [t-3] (gamma, owner), organization\n",
		`, x\u001b[2J` + "\n",
		"- Eps [t-5] (eps, owner)",
	} {
		if code != 0 || !strings.Contains(out+"\n", want) {
			t.Fatalf("list missing %q: %d %q", want, code, out)
		}
	}
	if strings.Contains(out, "\x1b") || strings.Contains(out, "(eps, owner),") {
		t.Fatalf("list = %q", out)
	}
	h.env["LANG"] = "fr_CA.UTF-8"
	if _, out, _ := h.run("workspaces", "list"); !strings.Contains(out, "(acme, owner), personnel (non confirmé) (selected)") || !strings.Contains(out, "(beta, member), organisation\n") {
		t.Fatalf("fr-CA list = %q", out)
	}
	delete(h.env, "LANG")
	code, out, _ = h.run("workspaces", "list", "--json")
	if code != 0 || !strings.Contains(out, `"kind":"personal","kind_source":"inferred"`) {
		t.Fatalf("list --json = %d %q", code, out)
	}
}
