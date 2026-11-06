# Herdr in a sandbox

[Herdr](https://github.com/herdrdev/herdr) is an open-source (Apache-2.0) terminal multiplexer for AI coding agents. If you run its server inside a sandbox, your panes and agents keep running there after you disconnect, for as long as the sandbox runs. Herdr uses system OpenSSH, so it connects through the same `<sandbox-id>.rzv` alias as [sandbox SSH](sandboxes-ssh.md). This page was checked with Herdr 0.9.5 and rzv 0.24.0.

## Set up

Install Herdr on your computer with its [install guide](https://herdr.dev/docs/install/), for example `brew install herdr`. Then create the sandbox and its SSH configuration:

```sh
rzv sandboxes create --name agents --lifetime 12h
rzv sandboxes ssh-config <sandbox-id> --write
ssh <sandbox-id>.rzv true          # confirm plain SSH works first
```

Only `/workspace` survives a stop. The home directory starts empty after each start. Install Herdr 0.9.5 under `/workspace/.herdr` and check its SHA-256:

```sh
ssh <sandbox-id>.rzv 'set -e; mkdir -p /workspace/.herdr/config
curl -fsSL -o /tmp/herdr https://github.com/herdrdev/herdr/releases/download/v0.9.5/herdr-linux-x86_64
echo "14d9e8bc8b4a9e0118d77c32266f76d461f09b40342eb23752960bd89f59929d  /tmp/herdr" | sha256sum -c -
install -m 0755 /tmp/herdr /workspace/.herdr/herdr && rm /tmp/herdr'
```

Herdr publishes these checksums in `https://herdr.dev/latest.json` and on its GitHub release. On an aarch64 sandbox, use `herdr-linux-aarch64` with `17d7536f2da50859523680ef279dffcac8e4a235f4e46fe8da6c2ec44141897c`.

After the install, and again after every `rzv sandboxes start`, link Herdr and its saved layout into the places Herdr looks:

```sh
ssh <sandbox-id>.rzv 'mkdir -p ~/.local/bin ~/.config && ln -sfn /workspace/.herdr/herdr ~/.local/bin/herdr && ln -sfn /workspace/.herdr/config ~/.config/herdr'
```

If you skip the install, `herdr --remote` offers to install a matching build in `~/.local/bin`. That copy and the saved layout are lost at the next stop.

## Connect

```sh
herdr --remote <sandbox-id>.rzv              # Herdr UI on your computer, server and panes in the sandbox
ssh -t <sandbox-id>.rzv '~/.local/bin/herdr' # or run the whole UI in the sandbox
herdr machine add <sandbox-id>.rzv           # or keep the sandbox in the sidebar with your other machines
```

Open a pane, `cd /workspace` and start an agent. `claude` and `codex` are preinstalled. Detach with `ctrl+b q`, or close the terminal: the panes keep running. To reattach, run the same command again. It also works from another computer once you have run `ssh-config --write` there. `herdr --remote` keeps image paste working because the UI runs on your computer.

Each new SSH connection gets a fresh 15-minute certificate, so you can reattach hours later as long as your `rzv` sign-in is valid. Herdr keeps one shared OpenSSH connection open for up to 10 idle minutes.

## Stop, start and expiry

- Stopping the sandbox ends the Herdr server and every pane process. `/workspace` is kept, including `/workspace/.herdr`.
- After `rzv sandboxes start <sandbox-id>`, run the link command and attach again. Herdr starts a new server and restores the saved workspaces and panes with fresh shells ([Herdr session state](https://herdr.dev/docs/session-state/)).
- At expiry the sandbox stops for good and its files are deleted. The lifetime cannot be extended.

## Limits

- Agent sign-ins and history kept in the home directory, such as `~/.claude`, are also gone after a stop, so agents start fresh sessions.
- Sandboxes do not enable agent forwarding, so `ForwardAgent` has no effect. If your agents push to Git, set up Git credentials inside the sandbox.
- Windows sandbox SSH is not supported yet, so Herdr cannot connect from Windows.
- `scripts/20-herdr-e2e.sh` runs these steps against one sandbox and deletes it afterwards.

Back to [Sandboxes](sandboxes.md) or the [documentation index](README.md).
