# API keys

Managing keys needs your own sign-in, not `RZV_API_KEY`.

| Command                                                                                                                  | What it does                          |
| ------------------------------------------------------------------------------------------------------------------------ | ------------------------------------- |
| `rzv api-keys list`                                                                                                      | Keys, scopes, permissions and expiry. |
| `rzv api-keys create --name <n> (--scopes <s,...> \| --permissions <p,...>) --idempotency-key <k> [--expires-at <time>]` | Create a key; prints its secret once. |
| `rzv api-keys rename <key-id> --name <n>`                                                                                | Rename a key.                         |
| `rzv api-keys revoke <key-id>`                                                                                           | Revoke a key.                         |

Provide exactly one of `--scopes` or `--permissions` when creating a key. Scopes: `all:all` alone (full access, owners only), or any of `audit:read`, `jobs:read`, `logs:read`, `usage:read`, `runners:read`, `integrations:read`, `billing:read`, `sandboxes:read`, `sandboxes:write`. Permission names are checked against your current grants by the service. Keys expire after 365 days unless `--expires-at` is earlier. Retrying `create` with the same `--idempotency-key` does not create a second key.

Back to [Workspace commands](workspace-commands.md).
