# Workspace commands

These commands act on the workspace chosen with `rzv workspaces select`, or on `--workspace <id>`. They also work with a workspace API key in `RZV_API_KEY` (see [Sign-in and credentials](login.md)), within the key's scopes.

## Jobs and usage

| Command                 | What it does                                                       |
| ----------------------- | ------------------------------------------------------------------ |
| `rzv jobs list`         | Every job, newest first: status, conclusion, repository, workflow. |
| `rzv jobs get <job-id>` | One job in detail. `rzv jobs status <job-id>` is the same.         |
| `rzv usage`             | Recorded billable minutes and job count.                           |
| `rzv concurrency`       | Concurrent-job limit, jobs running now and what is left.           |
| `rzv billing`           | Minute balance for the period and the packs on sale.               |
| `rzv billing purchases` | Recent minute-pack orders.                                         |

`rzv concurrency` and `rzv runners` are read-only and need the `runners:read` scope when used with an API key. The limit is set by Runzivo for the workspace; `rzv` cannot change it. The running jobs on every listed size, plus the running jobs on the "no longer listed" line when `rzv runners` prints one, add up to the running count that `rzv concurrency` reports.

A job's `status` is Runzivo's view of the run; only `conclusion` `success` means the GitHub workflow job succeeded.

## GitHub

| Command                               | What it does                                                           |
| ------------------------------------- | ---------------------------------------------------------------------- |
| `rzv github status`                   | The linked GitHub App installation and its repositories.               |
| `rzv github connect`                  | Start linking an installation; prints a short-lived state.             |
| `rzv github authorize ...`            | Finish the link. Needs your own sign-in.                               |
| `rzv repositories list`               | Repositories every linked installation covers.                         |
| `rzv runner-profiles <repository-id>` | Runner sizes the repository can use now, with their `runs-on` label.   |
| `rzv runners <repository-id>`         | The same sizes, plus the workspace's running and waiting jobs on each. |

`rzv github authorize --state <state> --installation-id <id> --github-client-id <id>` uses GitHub's device flow: it prints a code to enter at github.com, then sends the resulting GitHub token to Runzivo once to prove you control the installation. `rzv` does not store that token.

## Teams and permissions

| Command                                                                 | What it does                                                 |
| ----------------------------------------------------------------------- | ------------------------------------------------------------ |
| `rzv permissions`                                                       | Your permissions here, and every permission a team can have. |
| `rzv teams list`                                                        | Teams with their member counts.                              |
| `rzv teams show <team-id>`                                              | One team: permissions and members.                           |
| `rzv teams create --name <n> --permissions <p,...> [--description <t>]` | Create a custom team.                                        |
| `rzv teams rename <team-id> [--name <n>] [--description <t>]`           | Rename a team or change its description.                     |
| `rzv teams permissions <team-id> --permissions <p,...>`                 | Replace the team's permissions.                              |
| `rzv teams add-member <team-id> <identity-id>`                          | Add a workspace member to a team.                            |
| `rzv teams remove-member <team-id> <identity-id>`                       | Remove a member from a team.                                 |
| `rzv teams delete <team-id>`                                            | Delete a custom team.                                        |

Changing teams needs the Manage members permission, and you can only grant permissions you hold yourself. The system teams (Owners, Admins, Members, Viewers) cannot be renamed, deleted or given other permissions, and only an owner can change who is in Owners. With an API key, `rzv teams` and `rzv permissions` need a key with the `all:all` scope. Every change is recorded in the workspace activity log.

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
