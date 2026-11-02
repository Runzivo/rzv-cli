# Workspaces

| Command                                       | What it does                                                           |
| --------------------------------------------- | ---------------------------------------------------------------------- |
| `rzv workspaces list`                         | Workspaces you belong to, with name, slug, id and role.                              |
| `rzv workspaces select <workspace>`  | Use this workspace for later commands.                                 |
| `rzv workspaces create --slug <s> --name <n>` | Create a workspace and select it. Re-running the same command is safe. |

To create a workspace from a GitHub App installation, see [GitHub](github.md).

`rzv accounts ...` still works as the older name. Workspace-scoped commands take `--workspace <workspace>` (short `-w`, or the older `--account`) to override the selection. `RZV_WORKSPACE` sets the default workspace, and it is used when no `--workspace` is given. With `RZV_API_KEY`, pass the workspace id: a key cannot look up slugs or names.

A workspace reference matches an exact id first, then a slug without case sensitivity, then a display name without case sensitivity. Quote names with spaces, for example `rzv workspaces select "My Team"`. If several workspaces match, the command lists their ids and refuses to choose. Select an exact id instead. `select` and `whoami` show the selected name and id.

## Settings, members and invitations

| Command                                                                                 | What it does                                                                                                                                  |
| --------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| `rzv workspaces settings show`                                                          | Workspace name, logo state, update capability, and Docker cache setting.                                                                      |
| `rzv workspaces settings update --name <name>`                                          | Change the workspace name.                                                                                                                    |
| `rzv workspaces settings update --logo <path>`                                          | Upload a PNG or JPEG logo up to 192 KiB, with width and height each at most 512 pixels. The encoded settings request must fit within 256 KiB. |
| `rzv workspaces settings update --remove-logo`                                          | Remove the current logo.                                                                                                                      |
| `rzv workspaces settings update --docker-cache <enabled\|disabled>`                     | Turn the workspace Docker layer cache on or off.                                                                                              |
| `rzv workspaces settings update --docker-cache-size-mib <MiB>`                          | Set the cache size, a whole number of MiB from 64 to 16384. The service default is 1024.                                                     |
| `rzv workspaces members list`                                                           | Members and the allowed actions and roles for each member.                                                                                    |
| `rzv workspaces members role <identity-id> --role <role>`                               | Change a member's role.                                                                                                                       |
| `rzv workspaces members remove <identity-id>`                                           | Remove a member.                                                                                                                              |
| `rzv workspaces invitations list`                                                       | Pending and prior invitations, teams, and allowed actions.                                                                                    |
| `rzv workspaces invitations invite --email <email> --role <role> [--team-ids <id,...>]` | Invite a person, optionally assigning known custom team IDs.                                                                                  |
| `rzv workspaces invitations resend <invitation-id>`                                     | Resend a pending invitation.                                                                                                                  |
| `rzv workspaces invitations revoke <invitation-id>`                                     | Revoke an invitation.                                                                                                                         |
| `rzv workspaces invitations teams <invitation-id> --team-ids <id,...>`                  | Replace custom teams. Pass an empty value to clear them.                                                                                      |
| `rzv workspaces invitations accept (--token <token> \| --token-file <path>)`            | Accept an invitation with your own sign-in. Your identity needs a verified email matching the invitation recipient.                           |

`--docker-cache` and `--docker-cache-size-mib` can be used alone or together; a flag you leave out keeps its current value. The cache setting is a workspace preference. Jobs use the cache only when hosts in this environment can serve it, which needs an operator to turn the cache on and at least one execution host to support it. `settings show` prints `available in this environment` when hosts can serve the cache and `not available in this environment yet` when they cannot, and `--json` reports the same as `docker_cache.available`. Even when the cache is available, a job can still run without it because of assignments, quotas or host health. There is no charge for the cache today. Older services that do not report the setting show no Docker cache line.

The service enforces the final permission, role, grant ceiling, self-removal, and last-owner checks. Invitation acceptance does not need a selected workspace and never saves or prints the acceptance token.

Find team IDs with `rzv teams list` and pass them with `--team-ids`; `teams --team-ids=` clears the list.

## Closure requests

`rzv workspaces closures list` shows requests. `request` asks staff to close a workspace and `cancel` withdraws a pending request with its expected version. Neither command deletes a workspace immediately.

List returns the newest 20 requests and has no cursor. List requires an owner or admin. Request and cancel use the current owner's `workspace:delete` permission. API-key access requires `all:all`.

| Command                                                                   | What it does                                       |
| ------------------------------------------------------------------------- | -------------------------------------------------- |
| `rzv workspaces closures list`                                            | List closure requests and their decision metadata. |
| `rzv workspaces closures request --reason <text> --idempotency-key <key>` | Request closure for staff handling.                |
| `rzv workspaces closures cancel <request-id> --expected-version <n>`      | Cancel a pending request.                          |

Back to [Workspace commands](workspace-commands.md).
