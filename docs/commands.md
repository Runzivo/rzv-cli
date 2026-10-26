# Commands

Every command accepts `--json` (one JSON object on stdout) and `--help`. Errors go to stderr: a plain line, or `{"error": "<code>", "message": "..."}` with `--json`.

## Sign-in

| Command      | What it does                                                                                                            |
| ------------ | ----------------------------------------------------------------------------------------------------------------------- |
| `rzv login`  | Sign in through the browser. See [Sign-in and credentials](login.md).                                                   |
| `rzv logout` | Delete the stored sign-in.                                                                                              |
| `rzv whoami` | Show the signed-in identity, environment, session expiry and selected workspace, and check the sign-in against the API. |

## Workspaces

| Command                                       | What it does                                                           |
| --------------------------------------------- | ---------------------------------------------------------------------- |
| `rzv workspaces list`                         | Workspaces you belong to, with your role.                              |
| `rzv workspaces select <workspace-id\|slug>`  | Use this workspace for later commands.                                 |
| `rzv workspaces create --slug <s> --name <n>` | Create a workspace and select it. Re-running the same command is safe. |

`rzv accounts ...` still works as the older name. Workspace-scoped commands take `--workspace <id>` (or the older `--account <id>`) to override the selection.

## Workspace commands

`jobs`, `usage`, `concurrency`, `billing`, `github`, `repositories`, `runner-profiles`, `runners` and `api-keys` act on the selected workspace: see [Workspace commands](workspace-commands.md).

## Support tickets

`rzv tickets` lists, opens, shows and replies to support tickets. Commands use the selected workspace by default. Pass `--personal` for tickets outside a workspace. See [Support tickets](tickets.md).

## Other

| Command       | What it does                                      |
| ------------- | ------------------------------------------------- |
| `rzv version` | The rzv version, commit, build date and platform. |
| `rzv help`    | All commands; `rzv help <command>` for one.       |

Not in `rzv`: staff commands (staff console), workspace settings, members, invitations and buying minutes (Runzivo portal). The customer API does not provide close or reopen operations for tickets.

## Exit codes

| Code | Meaning                                         |
| ---- | ----------------------------------------------- |
| 0    | Success                                         |
| 1    | Request failed, conflict or local file problem  |
| 2    | Invalid command, option or configuration        |
| 3    | Not signed in, or the sign-in expired           |
| 4    | Access denied                                   |
| 5    | Runzivo or the identity provider is unavailable |
| 6    | Not found                                       |

Back to the [documentation index](README.md).
