package cli

import (
	"strings"
	"testing"
)

// permission_required comes from more than billing (GitLab connections,
// workspace settings), so every command must print the same generic message.
func TestPermissionRequiredMessageIsSharedAcrossCommands(t *testing.T) {
	h := selectedHarness(t)
	const want = "Your workspace role does not allow this change."
	h.json("GET /v1/customer/tenants/t-1/jobs", 403, `{"error":{"code":"permission_required"}}`)
	if code, _, errOut := h.run("jobs", "list"); code != exitAccessDenied || !strings.Contains(errOut, want) {
		t.Fatalf("jobs list permission_required = %d %q; want %q", code, errOut, want)
	}
	h.json("GET /v1/customer/tenants/t-1/billing", 403, `{"error":{"code":"permission_required"}}`)
	if code, _, errOut := h.run("billing"); code != exitAccessDenied || !strings.Contains(errOut, want) {
		t.Fatalf("billing permission_required = %d %q; want %q", code, errOut, want)
	}
}
