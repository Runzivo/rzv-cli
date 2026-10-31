# Teams and permissions

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

Changing teams needs the Manage members permission, and you can only grant permissions you hold yourself. The system teams (Owners, Admins, Members) cannot be renamed, deleted or given other permissions, and only an owner can change who is in Owners. With an API key, `rzv teams` and `rzv permissions` need a key with the `all:all` scope. Every change is recorded in the workspace activity log.

Back to [Workspace commands](workspace-commands.md).
