# Sandbox SSH and file transfers

On macOS and Linux, install OpenSSH and sign in with `rzv login`. The sandbox must be running and the Runzivo environment must have the matching SSH service enabled. Windows sandbox SSH is not supported yet; use `sandboxes exec` there.

```sh
rzv sandboxes ssh <sandbox-id>
rzv sandboxes ssh <sandbox-id> -- 'uname -a'
rzv sandboxes ssh <sandbox-id> -- 'exit 7'
rzv sandboxes ssh <sandbox-id> -- -N -L 8080:localhost:3000
```

The remote command's exit status is preserved. OpenSSH handles terminal resizing and signals. For several shell operations, pass one quoted command. The wrapper accepts `-t`, `-tt`, `-T`, `-N`, `-n`, `-v`, `-vv`, `-vvv` and `-L`; use plain `ssh` with generated configuration for other options.

With an API key, generating configuration requires `sandboxes:read`; connecting requires `sandboxes:write` (or `all:all`).

## Standard clients

```sh
rzv sandboxes ssh-config <sandbox-id> --write
ssh <sandbox-id>.rzv
scp file.txt <sandbox-id>.rzv:/workspace/
sftp <sandbox-id>.rzv
rsync -e ssh ./project/ <sandbox-id>.rzv:/workspace/project/
ssh -N -L 8080:localhost:3000 <sandbox-id>.rzv
```

`ssh-config` prints a Host block unless you pass `--write`. Without an id it handles the running sandboxes in the selected workspace. Add `--workspace <id, slug or name>` or `--profile <profile>` when generating configuration.

`--write` stores each owned block in `~/.ssh/rzv/` and prepends one Include to `~/.ssh/config`, preserving your other configuration and file permissions. An existing Include is reused anywhere in the file, including CRLF files. Symlinked configs are refused with instructions to include the printed block in your managed dotfile. Re-running it updates that sandbox's owned block. The alias is its sandbox id, so host moves and sandbox restarts do not change the connection name.

`--write` honors `$HOME`. OpenSSH uses the account’s passwd home for its default config and `~` expansion. Keep these homes aligned; otherwise place the printed block in the config OpenSSH reads.

## VS Code and Cursor

Install your editor's Remote SSH extension. Generate the configuration with `ssh-config --write`, then choose `<sandbox-id>.rzv` in its SSH connection picker and open `/workspace`. Confirm plain `ssh <sandbox-id>.rzv` works first.

Local forwarding reaches services on the sandbox's localhost. It requires the server release that permits local forwarding. Agent forwarding, X11 forwarding and remote forwarding are not enabled by this setup. No password or public guest SSH address is needed. Editor installation and operation have not been tested against a live sandbox in this release.

Editor references: [VS Code Remote SSH](https://code.visualstudio.com/docs/remote/ssh) and [Cursor Remote SSH troubleshooting](https://cursor.com/help/troubleshooting/network).

## Credentials and reconnection

Private keys, certificates and known-hosts state live under the CLI configuration directory with private permissions. Configuration pins Core's host CA for the exact sandbox alias; strict host-key checking stays on. A new guest host key signed by that CA is accepted after restart.

OpenSSH runs a silent preparation command before loading the certificate and uses `ssh --stdio` as its ProxyCommand. The 15-minute certificate is refreshed on reconnect, and each relay ticket is single-use for about 30 seconds. A prepared ticket is stored privately until the proxy claims and removes it; an abandoned ticket expires and is replaced on the next connection. Credentials are not included in command arguments or printed.

Keep the selected profile signed in. With `RZV_API_KEY`, the connecting process must inherit the key and API configuration; use an exact workspace id. Prepared state is bound to the API, identity, profile, workspace and sandbox. After changing identity, regenerate the SSH configuration.

A stopped, expired or deleted sandbox disconnects. Idle and unread-output limits still apply on the service. Reconnect with the same command; the CLI does not replay remote commands automatically. A long-running established session does not reconnect merely because its login certificate expires.

Use `rzv sandboxes terminal <sandbox-id>` for the existing PTY shell. When OpenSSH is missing or the environment has SSH disabled or lacks its routes, `ssh` falls back to that shell only for an interactive connection without extra arguments. Commands and file transfers receive guidance instead of being replayed through the terminal.

Back to [Sandboxes](sandboxes.md).
