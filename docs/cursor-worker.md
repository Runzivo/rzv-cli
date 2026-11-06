# Cursor Cloud Agents on a sandbox

Not yet verified: this page has not been tested with a live Cursor account.

Cursor's [Self-Hosted Machines](https://cursor.com/docs/cloud-agent/self-hosted) let Cursor Cloud Agents run their tools on a machine you manage. Cursor keeps the agent loop and the model calls. The machine runs the terminal commands and file edits. A Runzivo sandbox can be that machine: the worker only needs outbound HTTPS. Cursor bills you for the agent's model use. Runzivo charges only the sandbox's compute ([billing](sandboxes-billing.md)) and never sees your Cursor key.

## What you need

- A Cursor plan with Cloud Agents. Cursor lists "Premium and higher, Teams, Enterprise, or a legacy paid plan".
- For one sandbox of your own ([My Machines](https://cursor.com/docs/cloud-agent/self-hosted/my-machines)): a user API key from Cursor Dashboard, API Keys. On a team, an admin must allow personal or self-hosted machines.
- Team pools ([Self-Hosted Pool](https://cursor.com/docs/cloud-agent/self-hosted/pool)) need Cursor Enterprise and a service-account key. This page covers My Machines only.
- A running sandbox and the SSH alias from `rzv sandboxes ssh-config <sandbox-id> --write` ([SSH](sandboxes-ssh.md)).

## Install the Cursor CLI

```sh
ssh <sandbox-id>.rzv 'curl -fsS https://cursor.com/install | bash && ~/.local/bin/agent --version'
```

## Add your key

Send the key over SSH, from a password manager or a hidden prompt. It goes into a file only you can read, outside `/workspace`:

```sh
<password-manager-command> | ssh <sandbox-id>.rzv 'umask 077; mkdir -p ~/.config/cursor-worker && cat > ~/.config/cursor-worker/api-key'
```

Don't pass the key with `rzv sandboxes exec --env`, in an `exec` command or as `--api-key` on the command line. Runzivo keeps exec requests until the host receives them. Your shell history keeps command lines, and other processes can read them.

## Start the worker

Run it in a terminal you keep open, for example `rzv sandboxes ssh <sandbox-id>` or a [Herdr](herdr.md) pane:

```sh
export PATH="$HOME/.local/bin:$PATH"
cd /workspace
CURSOR_API_KEY="$(cat ~/.config/cursor-worker/api-key)" agent worker start --worker-dir /workspace --name <sandbox-id>
```

The sandbox then appears in Cursor's machine picker for Cloud Agents. Choose it when you start an agent. The worker runs until you stop it with Ctrl-C or the sandbox stops.

## Lifetime and limits

- A stop, expiry or deletion ends the worker. Only `/workspace` survives a stop and start at the moment, so after a `start` install the CLI and add your key again.
- The sandbox needs outbound HTTPS to `api2.cursor.sh`, `api2direct.cursor.sh`, `downloads.cursor.com` and Cursor's artifact hosts ([network list](https://cursor.com/docs/cloud-agent/self-hosted/pool)). It also needs your Git host and package registries.
- The agent can run commands and change files in the sandbox. Review its work before you push.
- Remove the key early with `rm ~/.config/cursor-worker/api-key`, and revoke it in Cursor Dashboard when you no longer need it.

Back to [Sandboxes](sandboxes.md).
