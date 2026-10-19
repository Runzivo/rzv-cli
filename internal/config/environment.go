// Package config holds the Runzivo environment presets and the local
// credential store.
package config

import (
	"fmt"
	"sort"
	"strings"
)

// Environment is everything rzv needs to sign in to one Runzivo deployment
// and call its customer API.
type Environment struct {
	Name        string
	APIBaseURL  string
	PortalURL   string
	Issuer      string
	ClientID    string
	Audience    string
	BrokerClaim string
}

// DefaultEnvironment is used when neither --env nor RZV_ENV is set.
const DefaultEnvironment = "prd"

var environments = map[string]Environment{
	"prd": {
		Name:        "prd",
		APIBaseURL:  "https://api.runzivo.dev",
		PortalURL:   "https://app.runzivo.dev",
		Issuer:      "https://shared-id.runzivo.dev/realms/rzv-prd",
		ClientID:    "rzv-cli",
		Audience:    "rzv-customer-api",
		BrokerClaim: "identity_provider",
	},
	// The shared environment is reachable only from allowlisted networks.
	"shared": {
		Name:        "shared",
		APIBaseURL:  "https://shared-api.runzivo.dev",
		PortalURL:   "https://shared-app.runzivo.dev",
		Issuer:      "https://shared-id.runzivo.dev/realms/rzv-shrd",
		ClientID:    "rzv-cli",
		Audience:    "rzv-customer-api",
		BrokerClaim: "identity_provider",
	},
}

// aliases accept the longer spellings people type.
var aliases = map[string]string{
	"prod":       "prd",
	"production": "prd",
	"shrd":       "shared",
}

// LookupEnvironment returns the preset for name. An empty name means the
// default environment.
func LookupEnvironment(name string) (Environment, error) {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		key = DefaultEnvironment
	}
	if alias, ok := aliases[key]; ok {
		key = alias
	}
	env, ok := environments[key]
	if !ok {
		return Environment{}, fmt.Errorf("unknown environment %q; use one of %s", name, strings.Join(EnvironmentNames(), ", "))
	}
	return env, nil
}

// EnvironmentNames lists the preset names in a stable order.
func EnvironmentNames() []string {
	names := make([]string, 0, len(environments))
	for name := range environments {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
