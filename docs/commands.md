# Commands

Every command accepts `--json` (one JSON object on stdout) and `--help`. Errors go to stderr: a plain line, or `{"error": "<code>", "message": "..."}` with `--json`.

## Sign-in

| Command      | What it does                                                                                                            |
| ------------ | ----------------------------------------------------------------------------------------------------------------------- |
| `rzv login`  | Sign in through the browser. See [Sign-in and credentials](login.md).                                                   |
| `rzv logout` | Delete the stored sign-in of the profile in use.                                                                        |
| `rzv whoami` | Show the signed-in identity, profile, environment, session expiry and selected workspace, and check the sign-in against the API. |
| `rzv profiles list` | List local profiles, the active one, and each profile's identity and workspace. Tokens are never printed.          |
| `rzv profiles use <profile>` | Make a profile the active one for later commands.                                                              |
| `rzv profiles remove <profile>` | Delete a profile's local sign-in. The session itself is not revoked.                                        |

## Profiles

A profile is one sign-in on this machine. `rzv login -p <profile>` signs in to a named profile without replacing the others. Every command accepts `-p <profile>` (or `--profile`) for one run, and `RZV_PROFILE` sets the profile for a shell. The profile in use is chosen in this order: `-p`, then `RZV_PROFILE`, then the active profile (set by `rzv profiles use` or by the last `rzv login`), then `default`. Profile names are lowercase letters, digits and hyphens, up to 32 characters. See [Sign-in and credentials](login.md).

## Workspaces

| Command                                       | What it does                                                                                                |
| --------------------------------------------- | ----------------------------------------------------------------------------------------------------------- |
| `rzv workspaces list`                         | Workspaces you belong to, with your role.                                                                   |
| `rzv workspaces select <workspace-id\|slug>`  | Use this workspace for later commands.                                                                      |
| `rzv workspaces create --slug <s> --name <n>` | Create a workspace and select it. Re-running the same command is safe.                                      |
| `rzv workspaces settings ...`                 | Read or update workspace name and logo. See [Workspace commands](workspace-commands.md).                    |
| `rzv workspaces members ...`                  | List members and manage roles or removal. See [Workspace commands](workspace-commands.md).                  |
| `rzv workspaces invitations ...`              | List, send, change, resend, revoke, or accept invitations. See [Workspace commands](workspace-commands.md). |
| `rzv github workspaces start`                 | Start GitHub-backed workspace onboarding.                               |
| `rzv github workspaces complete ...`          | Complete GitHub-backed workspace onboarding.                            |

`rzv accounts ...` still works as the older name. Workspace-scoped commands take `--workspace <slug or id>` (short `-w`, or the older `--account`) to override the selection. `RZV_WORKSPACE` sets the default workspace, and it is used when no `--workspace` is given. With `RZV_API_KEY`, pass the workspace id: a key cannot look up slugs.

## Workspace commands

`jobs`, `usage`, `insights`, `concurrency`, `billing`, `github`, `repositories`, `runner-profiles`, `runners`, `teams`, `permissions`, `api-keys`, `notifications`, and the workspace access subcommands act on the selected workspace: see [Workspace commands](workspace-commands.md). Invitation acceptance is the exception: it uses your own sign-in and does not need a selected workspace. GitHub-backed workspace onboarding uses your own sign-in before a workspace is selected.

## Support tickets

`rzv tickets` lists, opens, shows and replies to support tickets. Commands use the selected workspace by default. Pass `--personal` for tickets outside a workspace. See [Support tickets](tickets.md).

## Activity

`rzv activity list` reads workspace activity (50 rows by default), `rzv activity export` writes matching CSV to stdout, and `rzv activity status` shows current audit signals and execution evidence (30 days by default). With `--json`, export returns `{workspace_id,csv}`. List output includes scope, retention and the next cursor. CSV exports contain at most 10,000 server-selected rows.

## Notifications

`rzv notifications` lists workspace notifications, marks them read, and reads or updates notification settings. These commands require your own sign-in; API keys are not accepted. See [Notifications](notifications.md).

## AI

`rzv ai providers` lists, adds, updates, removes, tests, and lists models for workspace AI connections. `rzv ai settings` shows or updates configuration, and `rzv ai usage` reports summary, calls, and the current limit. See [AI settings and usage](ai.md). Provider credentials are accepted only through `--key-file` and are never displayed.

## Other

| Command       | What it does                                      |
| ------------- | ------------------------------------------------- |
| `rzv version` | The rzv version, commit, build date and platform. |
| `rzv help`    | All commands; `rzv help <command>` for one.       |

Not in `rzv`: staff commands (staff console). Minutes are bought with `rzv billing checkout`. The customer API does not provide a reopen operation for tickets.

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
