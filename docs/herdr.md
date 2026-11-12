# Herdr in a sandbox

[Herdr](https://github.com/herdrdev/herdr) is an open-source (Apache-2.0) terminal multiplexer for AI coding agents. If you run its server inside a sandbox, your panes and agents keep running there while you are disconnected, until the sandbox stops. Herdr uses system OpenSSH, so it connects through the same `<sandbox-id>.rzv` alias as [sandbox SSH](sandboxes-ssh.md). It uses no port forwarding. This page was checked with Herdr 0.9.5 and rzv 0.24.0.

## Set up

Install Herdr on your computer with its [install guide](https://herdr.dev/docs/install/), for example `brew install herdr`. Then create the sandbox and its SSH configuration:

```sh
rzv sandboxes create --name agents --lifetime 12h
rzv sandboxes ssh-config <sandbox-id> --write
ssh <sandbox-id>.rzv true          # confirm plain SSH works first
```

The first time you run `herdr --remote <sandbox-id>.rzv`, Herdr asks before installing a matching build into the sandbox's `~/.local/bin/herdr`. To install it yourself instead, pinned to 0.9.5 with its SHA-256 checked:

```sh
ssh <sandbox-id>.rzv 'set -e; mkdir -p ~/.local/bin
curl -fsSL -o /tmp/herdr https://github.com/herdrdev/herdr/releases/download/v0.9.5/herdr-linux-x86_64
echo "14d9e8bc8b4a9e0118d77c32266f76d461f09b40342eb23752960bd89f59929d  /tmp/herdr" | sha256sum -c -
install -m 0755 /tmp/herdr ~/.local/bin/herdr && rm /tmp/herdr'
```

Herdr publishes these checksums in `https://herdr.dev/latest.json` and on its GitHub release. On an aarch64 sandbox, use `herdr-linux-aarch64` with `17d7536f2da50859523680ef279dffcac8e4a235f4e46fe8da6c2ec44141897c`.

## Connect

```sh
herdr --remote <sandbox-id>.rzv              # Herdr UI on your computer, server and panes in the sandbox
ssh -t <sandbox-id>.rzv '~/.local/bin/herdr' # or run the whole UI in the sandbox
herdr machine add <sandbox-id>.rzv           # or keep the sandbox in the sidebar with your other machines
```

Open a pane, `cd /workspace` and start an agent. `claude` and `codex` are preinstalled. Detach with `ctrl+b q`, or close the terminal. While the sandbox keeps running, the server, its panes and their processes keep running, and the same command reattaches to them. It also works from another computer once you have run `ssh-config --write` there.

Each new SSH connection gets a fresh 15-minute certificate. A client that stays attached keeps working after its certificate expires, and you can reattach later as long as your `rzv` sign-in is valid. Herdr keeps one shared OpenSSH connection open for up to 10 idle minutes.

## Stop, start and expiry

A stop and start keeps the sandbox's disk. After `rzv sandboxes start <sandbox-id>`:

- Herdr, its configuration and its saved session in `~/.config/herdr` are still there, and so are `/workspace` and agent sign-ins in the home directory, such as `~/.claude`.
- The Herdr server and every pane and process have ended.

Run `herdr --remote <sandbox-id>.rzv` again to start the server, then start new panes. At expiry the sandbox stops for good and everything in it is deleted, including Herdr and `/workspace`. The lifetime cannot be extended.

## Other limits

- Sandboxes do not enable agent forwarding, so `ForwardAgent` has no effect. If your agents push to Git, set up Git credentials inside the sandbox.
- Windows sandbox SSH is not supported yet, so Herdr cannot connect from Windows.
- `scripts/20-herdr-e2e.sh` runs these steps against one sandbox and deletes it afterwards.

Back to [Sandboxes](sandboxes.md) or the [documentation index](README.md).
