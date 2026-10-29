# Sign-in and credentials

`rzv login` opens your browser at Runzivo's identity provider, where you sign in with GitHub. `rzv` listens on `http://127.0.0.1:43821/oauth/callback` for the result, so run it on the machine where the browser runs. It uses the OpenID Connect authorization code flow with PKCE (S256), checks the returned tokens' signature, issuer, audience and expiry, then stores the access token.

- `--no-browser` prints the sign-in URL instead of opening a browser.
- The session lasts as long as the access token. When it expires, commands stop with exit code 3; run `rzv login` again.

## Environments

| `--env`         | API                              | Identity realm |
| --------------- | -------------------------------- | -------------- |
| `prd` (default) | `https://api.runzivo.dev`        | `id.runzivo.dev`, realm `rzv-prd` |
| `shared`        | `https://shared-api.runzivo.dev` | `rzv-shrd`     |

`RZV_ENV` sets the default. The shared environment is only reachable from allowlisted networks. The sign-in remembers its environment; later commands use it.

## Profiles

Each profile holds one sign-in. `rzv login` signs in to the profile in use; `rzv login -p work` signs in to `work` and leaves the other profiles alone. `--workspace <slug or id>` (or `-w`) selects a workspace at sign-in; the sign-in is saved only if that workspace is one you belong to. Signing in makes its profile the active one.

`rzv profiles list` shows the profiles, `rzv profiles use <profile>` switches the active one, and `rzv profiles remove <profile>` deletes a profile's local sign-in. Removing does not revoke the session; it expires as usual.

## Where the token is stored

The `default` profile is `credentials.json`; every other profile is `profiles/<name>.json`. The active profile is in `profile.json`. All are in your user configuration directory, in a `runzivo` folder:

| OS      | Path                                              |
| ------- | ------------------------------------------------- |
| Linux   | `$XDG_CONFIG_HOME/runzivo` or `~/.config/runzivo` |
| macOS   | `~/Library/Application Support/runzivo`           |
| Windows | `%AppData%\runzivo`                               |

`RZV_CONFIG_DIR` overrides the folder. On Linux and macOS the folder is `0700` and the file `0600`; on Windows the file inherits your profile's permissions. `rzv` refuses a credentials file that is a symbolic link. `rzv logout` deletes the profile in use's file.

## API keys

Set `RZV_API_KEY` to use a workspace API key instead of a sign-in, for example in CI. Pass `--workspace <id>`; `RZV_API_URL` or `RZV_ENV` picks the API. Commands that act for a person (`whoami`, `workspaces`, managing API keys) refuse to run while `RZV_API_KEY` is set.

Back to the [documentation index](README.md).
