# Workspace commands

Most workspace commands accept `RZV_API_KEY` within the key's scopes. When using a workspace API key, settings, members, and invitations commands require the `all:all` scope. `rzv workspaces invitations accept` always needs your own sign-in. Other workspace commands act on the workspace chosen with `rzv workspaces select`, or on `--workspace <id>`.

## Workspace access

| Command                                                                                 | What it does                                                                                                                                  |
| --------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| `rzv workspaces settings show`                                                          | Workspace name, logo state, and update capability.                                                                                            |
| `rzv workspaces settings update --name <name>`                                          | Change the workspace name.                                                                                                                    |
| `rzv workspaces settings update --logo <path>`                                          | Upload a PNG or JPEG logo up to 192 KiB, with width and height each at most 512 pixels. The encoded settings request must fit within 256 KiB. |
| `rzv workspaces settings update --remove-logo`                                          | Remove the current logo.                                                                                                                      |
| `rzv workspaces members list`                                                           | Members and the allowed actions and roles for each member.                                                                                    |
| `rzv workspaces members role <identity-id> --role <role>`                               | Change a member's role.                                                                                                                       |
| `rzv workspaces members remove <identity-id>`                                           | Remove a member.                                                                                                                              |
| `rzv workspaces invitations list`                                                       | Pending and prior invitations, teams, and allowed actions.                                                                                    |
| `rzv workspaces invitations invite --email <email> --role <role> [--team-ids <id,...>]` | Invite a person, optionally assigning known custom team IDs.                                                                                  |
| `rzv workspaces invitations resend <invitation-id>`                                     | Resend a pending invitation.                                                                                                                  |
| `rzv workspaces invitations revoke <invitation-id>`                                     | Revoke an invitation.                                                                                                                         |
| `rzv workspaces invitations teams <invitation-id> --team-ids <id,...>`                  | Replace custom teams. Pass an empty value to clear them.                                                                                      |
| `rzv workspaces invitations accept (--token <token> \| --token-file <path>)`            | Accept an invitation with your own sign-in. Your identity needs a verified email matching the invitation recipient.                           |

The service enforces the final permission, role, grant ceiling, self-removal, and last-owner checks. Invitation acceptance does not need a selected workspace and never saves or prints the acceptance token.

There is no public custom-team discovery command. Supply IDs you already have with `--team-ids`; `teams --team-ids=` clears the list.

## Jobs and usage

| Command                     | What it does                                                                                                          |
| --------------------------- | --------------------------------------------------------------------------------------------------------------------- |
| `rzv jobs list`             | Every job, newest first: status, conclusion, repository, workflow.                                                    |
| `rzv jobs get <job-id>`     | One job in detail. `rzv jobs status <job-id>` is the same.                                                            |
| `rzv usage`                 | Recorded billable minutes and job count.                                                                              |
| `rzv concurrency`           | Concurrent-job limit, jobs running now and what is left.                                                              |
| `rzv billing`               | Minute balance for the period and the packs on sale.                                                                  |
| `rzv billing purchases`     | Recent minute-pack orders.                                                                                            |
| `rzv billing invoices`      | Receipts and invoices. `--json` for the full rows.                                                                    |
| `rzv billing auto-recharge` | Auto-recharge settings. `set --enabled <true\|false> --threshold <minutes> --pack <id>` changes them (billing:write). |
| `rzv billing country`       | The workspace billing country. `set <code>` changes it for future purchases (billing:write).                          |
| `rzv billing marketplace`   | The GitHub Marketplace plan for the workspace, if any.                                                                |
| `rzv billing checkout`      | Starts a hosted checkout: `--pack <id> --operation-id <id>`. Reuse the same `--operation-id` when retrying.           |
| `rzv billing portal`        | A link to the payment provider's billing portal. Do not store it.                                                     |

Checkout and portal links are sign-in capabilities. Live payments stay off until Runzivo enables them.

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
