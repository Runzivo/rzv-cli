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
