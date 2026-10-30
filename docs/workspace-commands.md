# Workspace commands

Most workspace commands accept `RZV_API_KEY` within the key's scopes. When using a workspace API key, settings, members, and invitations commands require the `all:all` scope. `rzv workspaces invitations accept` always needs your own sign-in. Other workspace commands act on the workspace chosen with `rzv workspaces select`, or on `--workspace <id>`. Notification commands always require your own sign-in.

| Page                                | Commands                                                                    |
| ----------------------------------- | --------------------------------------------------------------------------- |
| [Workspaces](workspaces.md)         | `workspaces` list, select, create, settings, members, invitations, closures |
| [Jobs, insights and usage](jobs.md) | `jobs`, `insights`, `usage`, `concurrency`                                  |
| [Billing](billing.md)               | `billing`, checkout, portal, auto-recharge, codes                           |
| [Activity](activity.md)             | `activity` list, export, status                                             |
| [Notifications](notifications.md)   | `notifications` list, read, settings                                        |
| [GitHub](github.md)                 | `github`, `repositories`, `runner-profiles`, `runners`, onboarding          |
| [Teams and permissions](teams.md)   | `teams`, `permissions`                                                      |
| [API keys](api-keys.md)             | `api-keys` list, create, rename, revoke                                     |
| [Support tickets](tickets.md)       | `tickets` list, open, show, reply, close                                    |
| [Sandboxes](sandboxes.md)           | `sandboxes`                                                                 |
| [AI settings and usage](ai.md)      | `ai` providers, settings, usage                                             |

Back to [Commands](commands.md).
