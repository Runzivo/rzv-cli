package cli

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Runzivo/rzv-cli/internal/api"
)

const apiKeysUsage = `Usage:
  rzv api-keys list [--workspace <id>] [--json]
  rzv api-keys create --name <name> --scopes <scope,...> --idempotency-key <key>
      [--expires-at <RFC 3339 time>] [--workspace <id>] [--json]
  rzv api-keys rename <key-id> --name <name> [--workspace <id>] [--json]
  rzv api-keys revoke <key-id> [--workspace <id>] [--json]

Scopes: all:all on its own (full access, owners only), or any of audit:read,
jobs:read, logs:read, usage:read, runners:read, integrations:read,
billing:read. Without --expires-at a key expires in 365 days, the longest
allowed. The secret is shown once, at creation. Use your own sign-in:
RZV_API_KEY cannot manage API keys.`

func init() {
	register("api-keys", "Manage workspace API keys", runAPIKeys)
	commandUsage["api-keys"] = apiKeysUsage
}

func runAPIKeys(a *app, cmd *command) error {
	if err := a.requireHuman(); err != nil {
		return err
	}
	if len(cmd.args) == 0 {
		return usageError("%s", apiKeysUsage)
	}
	switch cmd.args[0] {
	case "list":
		return apiKeysList(a, cmd)
	case "create":
		return apiKeysCreate(a, cmd)
	case "rename":
		return apiKeysRename(a, cmd)
	case "revoke":
		return apiKeysRevoke(a, cmd)
	default:
		return usageError("%s", apiKeysUsage)
	}
}

func apiKeysList(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	if err := cmd.expectArgs(1, "rzv api-keys list [--workspace <id>] [--json]"); err != nil {
		return err
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	keys, err := client.APIKeys(a.ctx, id)
	if err != nil {
		return apiError(err, "")
	}
	a.output(cmd, map[string]any{"workspace_id": id, "api_keys": keys.APIKeys, "allowed_scopes": keys.AllowedScopes, "capabilities": keys.Capabilities}, renderAPIKeys(id, keys.APIKeys))
	return nil
}

func renderAPIKeys(id string, keys []api.APIKey) string {
	if len(keys) == 0 {
		return "No API keys for workspace " + display(id) + "."
	}
	lines := []string{"API keys for workspace " + display(id) + ":"}
	for _, k := range keys {
		state := "active"
		if k.RevokedAt != nil {
			state = "revoked"
		}
		scopes := make([]string, len(k.Scopes))
		for i, s := range k.Scopes {
			scopes[i] = display(s)
		}
		lines = append(lines, fmt.Sprintf("- %s [%s] %s… %s, scopes %s, expires %s, last used %s",
			display(k.Name), display(k.ID), display(k.Prefix), state, strings.Join(scopes, ","), displayOrDash(k.ExpiresAt), displayOrDash(k.LastUsedAt)))
	}
	return strings.Join(lines, "\n")
}

func parseScopes(raw string) ([]string, error) {
	var scopes []string
	for _, s := range strings.Split(raw, ",") {
		if s = strings.TrimSpace(s); s != "" {
			scopes = append(scopes, s)
		}
	}
	bad := len(scopes) == 0 || (slices.Contains(scopes, "all:all") && len(scopes) > 1)
	seen := map[string]bool{}
	for _, s := range scopes {
		if seen[s] || !slices.Contains(api.APIKeyScopes, s) {
			bad = true
		}
		seen[s] = true
	}
	if bad {
		return nil, usageError("--scopes must be all:all on its own or a comma-separated list without repeats of audit:read, jobs:read, logs:read, usage:read, runners:read, integrations:read, billing:read.")
	}
	return scopes, nil
}

func apiKeysCreate(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account", "name", "scopes", "expires-at", "idempotency-key"); err != nil {
		return err
	}
	if err := cmd.expectArgs(1, "rzv api-keys create --name <name> --scopes <scope,...> --idempotency-key <key>"); err != nil {
		return err
	}
	name, err := cmd.required("name")
	if err != nil {
		return err
	}
	rawScopes, err := cmd.required("scopes")
	if err != nil {
		return err
	}
	scopes, err := parseScopes(rawScopes)
	if err != nil {
		return err
	}
	key, err := cmd.required("idempotency-key")
	if err != nil {
		return err
	}
	expires, err := cmd.option("expires-at")
	if err != nil {
		return err
	}
	if expires != "" {
		if _, perr := time.Parse(time.RFC3339, expires); perr != nil {
			return usageError("--expires-at must be an RFC 3339 time, for example 2027-01-31T00:00:00Z.")
		}
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	created, err := client.CreateAPIKey(a.ctx, id, api.CreateAPIKeyInput{Name: name, Scopes: scopes, ExpiresAt: expires, IdempotencyKey: key})
	if err != nil {
		return apiError(err, "")
	}
	a.output(cmd, map[string]any{"workspace_id": id, "api_key": created.APIKey, "secret": created.Secret},
		fmt.Sprintf("Created API key %s [%s]. Save this secret now; it cannot be shown again:\n%s", display(created.APIKey.Name), display(created.APIKey.ID), created.Secret))
	return nil
}

func apiKeysRename(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account", "name"); err != nil {
		return err
	}
	if err := cmd.expectArgs(2, "rzv api-keys rename <key-id> --name <name>"); err != nil {
		return err
	}
	name, err := cmd.required("name")
	if err != nil {
		return err
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	key, err := client.RenameAPIKey(a.ctx, id, cmd.args[1], name)
	if err != nil {
		return apiError(err, "The API key was not found in this workspace.")
	}
	a.output(cmd, map[string]any{"workspace_id": id, "api_key": key}, fmt.Sprintf("Renamed API key %s to %s.", display(key.ID), display(key.Name)))
	return nil
}

func apiKeysRevoke(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	if err := cmd.expectArgs(2, "rzv api-keys revoke <key-id>"); err != nil {
		return err
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	keyID := cmd.args[1]
	if err := client.RevokeAPIKey(a.ctx, id, keyID); err != nil {
		return apiError(err, "The API key was not found in this workspace.")
	}
	a.output(cmd, map[string]any{"workspace_id": id, "revoked_api_key_id": keyID}, "Revoked API key "+display(keyID)+".")
	return nil
}
