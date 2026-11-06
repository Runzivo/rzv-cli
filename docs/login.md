# Sign-in and credentials

`rzv login` opens your browser at Runzivo's identity provider, where you sign in with GitHub. `rzv` listens on `http://127.0.0.1:43821/oauth/callback` for the result, so run it on the machine where the browser runs. It uses the OpenID Connect authorization code flow with PKCE (S256), checks the returned tokens' signature, issuer, audience and expiry, then stores the access token.

- `--no-browser` prints the sign-in URL instead of opening a browser.
- The sign-in renews itself. The access token lasts minutes; shortly before it expires, `rzv` exchanges the stored refresh token for a new access token and a new refresh token. Your session follows Runzivo's customer session policy: it ends after 14 days without use or 30 days after sign-in, whichever comes first, or when you log out. Then commands stop with exit code 3; run `rzv login` again. `rzv whoami` shows when the session ends if unused.
- Renewal works for SSH, Orca, editors and scripts too, because they all run `rzv`. Parallel `rzv` runs renew once: an operating-system lock on a file beside the credentials makes them take turns, since each refresh token works only once. If `rzv` is killed in the moment between the provider issuing a new token and `rzv` saving it, the session ends and you sign in again.

## Environments

| `--env`         | API                              | Identity realm |
| --------------- | -------------------------------- | -------------- |
| `prd` (default) | `https://api.runzivo.dev`        | `id.runzivo.dev`, realm `rzv-prd` |
| `shared`        | `https://shared-api.runzivo.dev` | `rzv-shrd`     |

`RZV_ENV` sets the default. The shared environment is only reachable from allowlisted networks. The sign-in remembers its environment; later commands use it.

## Profiles

Each profile holds one sign-in. `rzv login` signs in to the profile in use; `rzv login -p work` signs in to `work` and leaves the other profiles alone. `--workspace <id, slug or name>` (or `-w`) selects a workspace at sign-in; the sign-in is saved only if that workspace is one you belong to. Signing in makes its profile the active one.

`rzv profiles list` shows the profiles with each one's identity and workspace (tokens are never printed), `rzv profiles use <profile>` switches the active one, and `rzv profiles remove <profile>` deletes a profile's local sign-in and ends its session at the identity provider.

Every command accepts `-p <profile>` (or `--profile`) for one run, and `RZV_PROFILE` sets the profile for a shell. The profile in use is chosen in this order: `-p`, then `RZV_PROFILE`, then the active profile, then `default`. Profile names are lowercase letters, digits and hyphens, up to 32 characters.

## Where the token is stored

The `default` profile is `credentials.json`; every other profile is `profiles/<name>.json`. The active profile is in `profile.json`. All are in your user configuration directory, in a `runzivo` folder:

| OS      | Path                                              |
| ------- | ------------------------------------------------- |
| Linux   | `$XDG_CONFIG_HOME/runzivo` or `~/.config/runzivo` |
| macOS   | `~/Library/Application Support/runzivo`           |
| Windows | `%AppData%\runzivo`                               |

`RZV_CONFIG_DIR` overrides the folder. On Linux and macOS the folder is `0700` and the file `0600`; on Windows the file inherits your profile's permissions. `rzv` refuses a credentials file that is a symbolic link. The file holds the access token and the refresh token; `rzv` never prints or logs either, and never passes them to SSH, sandboxes or plugins. `rzv logout` ends the session at the identity provider, so a copy of the file can no longer renew (an access token already issued keeps working until it expires, at most 15 minutes), and deletes the profile in use's file. If the provider does not confirm, logout still deletes the file and the session ends when it expires.

## API keys

Set `RZV_API_KEY` to use a workspace API key instead of a sign-in, for example in CI. Pass `--workspace <id>`; `RZV_API_URL` or `RZV_ENV` picks the API. Commands that act for a person (`whoami`, `workspaces`, managing API keys) refuse to run while `RZV_API_KEY` is set.

Back to the [documentation index](README.md).
