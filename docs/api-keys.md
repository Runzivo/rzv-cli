# API keys

Managing keys needs your own sign-in, not `RZV_API_KEY`.

| Command                                                                                                                  | What it does                          |
| ------------------------------------------------------------------------------------------------------------------------ | ------------------------------------- |
| `rzv api-keys list`                                                                                                      | Keys, scopes, permissions and expiry. |
| `rzv api-keys create --name <n> (--scopes <s,...> \| --permissions <p,...>) --idempotency-key <k> [--expires-at <time>]` | Create a key; prints its secret once. |
| `rzv api-keys rename <key-id> --name <n>`                                                                                | Rename a key.                         |
| `rzv api-keys remove-members-write <key-id>`                                                                             | Drop `members:write`; same secret.    |
| `rzv api-keys revoke <key-id>`                                                                                           | Revoke a key.                         |

Provide exactly one of `--scopes` or `--permissions` when creating a key. Scopes: `all:all` (full access, owners only), optionally with `members:write` so the key can manage members, teams and invitations (`all:all` alone never can), or any of `audit:read`, `jobs:read`, `logs:read`, `usage:read`, `runners:read`, `integrations:read`, `billing:read`. Permission names are checked against your current grants by the service. Keys expire after 365 days unless `--expires-at` is earlier. Retrying `create` with the same `--idempotency-key` does not create a second key.

`remove-members-write` removes `members:write` from a key without changing its secret. When Runzivo introduced `members:write`, existing full-access keys received it so that integrations keep working.

Back to [Workspace commands](workspace-commands.md).
