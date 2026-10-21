# Workspace commands

These commands act on the workspace chosen with `rzv workspaces select`, or on `--workspace <id>`. They also work with a workspace API key in `RZV_API_KEY` (see [Sign-in and credentials](login.md)), within the key's scopes.

## Jobs and usage

| Command                 | What it does                                                       |
| ----------------------- | ------------------------------------------------------------------ |
| `rzv jobs list`         | Every job, newest first: status, conclusion, repository, workflow. |
| `rzv jobs get <job-id>` | One job in detail. `rzv jobs status <job-id>` is the same.         |
| `rzv usage`             | Recorded billable minutes and job count.                           |
| `rzv billing`           | Minute balance for the period and the packs on sale.               |
| `rzv billing purchases` | Recent minute-pack orders.                                         |

A job's `status` is Runzivo's view of the run; only `conclusion` `success` means the GitHub workflow job succeeded.

## GitHub

| Command                               | What it does                                                         |
| ------------------------------------- | -------------------------------------------------------------------- |
| `rzv github status`                   | The linked GitHub App installation and its repositories.             |
| `rzv github connect`                  | Start linking an installation; prints a short-lived state.           |
| `rzv github authorize ...`            | Finish the link. Needs your own sign-in.                             |
| `rzv repositories list`               | Repositories every linked installation covers.                       |
| `rzv runner-profiles <repository-id>` | Runner sizes the repository can use now, with their `runs-on` label. |

`rzv github authorize --state <state> --installation-id <id> --github-client-id <id>` uses GitHub's device flow: it prints a code to enter at github.com, then sends the resulting GitHub token to Runzivo once to prove you control the installation. `rzv` does not store that token.

## Workspace closure requests

`rzv workspaces closures list` shows requests. `request` asks staff to close a workspace and `cancel` withdraws a pending request with its expected version. Neither command deletes a workspace immediately.

List returns the newest 20 requests and has no cursor. List requires an owner or admin. Request and cancel use the current owner's `workspace:delete` permission. API-key access requires `all:all`.

| Command | What it does |
| --- | --- |
| `rzv workspaces closures list` | List closure requests and their decision metadata. |
| `rzv workspaces closures request --reason <text> --idempotency-key <key>` | Request closure for staff handling. |
| `rzv workspaces closures cancel <request-id> --expected-version <n>` | Cancel a pending request. |

## API keys

Managing keys needs your own sign-in, not `RZV_API_KEY`.

| Command                                                                                       | What it does                          |
| --------------------------------------------------------------------------------------------- | ------------------------------------- |
| `rzv api-keys list`                                                                           | Keys, scopes and expiry.              |
| `rzv api-keys create --name <n> --scopes <s,...> --idempotency-key <k> [--expires-at <time>]` | Create a key; prints its secret once. |
| `rzv api-keys rename <key-id> --name <n>`                                                     | Rename a key.                         |
| `rzv api-keys revoke <key-id>`                                                                | Revoke a key.                         |

Scopes: `all:all` alone (full access, owners only), or any of `audit:read`, `jobs:read`, `logs:read`, `usage:read`, `runners:read`, `integrations:read`, `billing:read`. Keys expire after 365 days unless `--expires-at` is earlier. Retrying `create` with the same `--idempotency-key` does not create a second key.

Back to [Commands](commands.md).
