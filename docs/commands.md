# Commands

Every command accepts `--json` (one JSON object on stdout) and `--help`. Errors go to stderr: a plain line, or `{"error": "<code>", "message": "..."}` with `--json`.

## Sign-in

| Command      | What it does                                                                                                                     |
| ------------ | -------------------------------------------------------------------------------------------------------------------------------- |
| `rzv login`  | Sign in through the browser. See [Sign-in and credentials](login.md).                                                            |
| `rzv logout` | Delete the stored sign-in of the profile in use.                                                                                 |
| `rzv whoami` | Show the signed-in identity, profile, environment, session expiry and selected workspace, and check the sign-in against the API. |

Profiles (`-p <profile>`, `rzv profiles list|use|remove`) are in [Sign-in and credentials](login.md).

## Workspace commands

Workspace commands act on the selected workspace. [Workspaces](workspaces.md) covers choosing one; [Workspace commands](workspace-commands.md) lists every command group and its page.

## Other

| Command       | What it does                                      |
| ------------- | ------------------------------------------------- |
| `rzv version` | The rzv version, commit, build date and platform. |
| `rzv help`    | All commands; `rzv help <command>` for one.       |

Not in `rzv`: staff commands (staff console). Minutes are bought with `rzv billing checkout`.

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
