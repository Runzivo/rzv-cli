package config

import "testing"

func TestPrdIdentityUsesIdHostname(t *testing.T) {
	prd, err := LookupEnvironment("prd")
	if err != nil {
		t.Fatal(err)
	}
	if prd.Issuer != "https://id.runzivo.dev/realms/rzv-prd" {
		t.Fatalf("prd issuer = %q", prd.Issuer)
	}
	shared, err := LookupEnvironment("shared")
	if err != nil {
		t.Fatal(err)
	}
	if shared.Issuer != "https://shared-id.runzivo.dev/realms/rzv-shrd" {
		t.Fatalf("shared issuer changed: %q", shared.Issuer)
	}
}
