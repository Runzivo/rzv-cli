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

## Activity

| Command | What it does |
| --- | --- |
| `rzv activity list` | Activity rows, newest first, with optional category, actor, result, time, query and paging filters. |
| `rzv activity export` | Matching activity rows as CSV on stdout (or a `{workspace_id,csv}` JSON wrapper with `--json`), limited by the server to 10,000 rows. |
| `rzv activity status` | Current component signals and execution evidence for 7, 30, or 60 days (30 by default). |

List defaults to 50 rows. Its text and JSON output include the returned scope, retention period and next cursor for pagination.

## GitHub

| Command                               | What it does                                                         |
| ------------------------------------- | -------------------------------------------------------------------- |
| `rzv github status`                   | The linked GitHub App installation and its repositories.             |
| `rzv github connect`                  | Start linking an installation; prints a short-lived state.           |
| `rzv github authorize ...`            | Finish the link. Needs your own sign-in.                             |
| `rzv repositories list`               | Repositories every linked installation covers.                       |
| `rzv runner-profiles <repository-id>` | Runner sizes the repository can use now, with their `runs-on` label. |

`rzv github authorize --state <state> --installation-id <id> --github-client-id <id>` uses GitHub's device flow: it prints a code to enter at github.com, then sends the resulting GitHub token to Runzivo once to prove you control the installation. `rzv` does not store that token.

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
