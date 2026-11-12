# Orca workspaces

The [Runzivo plugin for Orca](https://github.com/Runzivo/rzv-orca-plugin) runs each Orca workspace in its own Runzivo sandbox. Its recipes call the commands below; you do not run `create`, `suspend`, `resume` or `destroy` yourself.

```sh
rzv orca defaults [--disk-gib <5-64>] [--lifetime 8h|12h|24h] [--size <size>] [--workspace <id>] [--reset]
rzv orca status [--workspace <id>] [--json]
rzv orca create [--size <size>] [--disk-gib <5-64>] [--lifetime 8h|12h|24h]   # run by Orca
rzv orca suspend | resume | destroy                                            # run by Orca, JSON on stdin
```

## What each step does

- **create** makes one sandbox named `orca-<workspace>-<id>` in your selected workspace (or the one `defaults --workspace` saved). The Orca workspace id is its idempotency key, so a retried create returns the same sandbox. It waits until the sandbox runs, writes its SSH config as `rzv sandboxes ssh-config --write` does, copies the commit Orca selected into `/workspace/<repo>` as a Git bundle over that SSH connection, and prints Orca's recipe result. Your local Git credentials fetch the commit if needed; none reach the sandbox, and user names and passwords are removed from `http(s)` remote URLs. If any step fails, create deletes the sandbox it made.
- **suspend** stops the sandbox. Files are kept and compute is not charged. The lifetime keeps running.
- **resume** starts it again, refreshes the SSH config and checks the checkout is still there.
- **destroy** deletes the sandbox, its checkout and every other file in it, and removes its file under `~/.ssh/rzv`.

Every step prints progress on stderr; only create and resume print JSON on stdout, which Orca reads.

## Size, disk, lifetime

The recipe picks the size. The disk (default 20 GiB) and lifetime (default 8h) come from `rzv orca defaults` for the current profile. A lifetime is 8, 12 or 24 hours from the first run and can never be extended. `create` refuses a size, lifetime or SSH setting this Runzivo environment does not offer, before it creates anything. Billing is the usual sandbox billing: [Billing and permissions](sandboxes-billing.md).

`rzv orca status` lists the sandboxes Orca made, with their expiry, time left and credits used.

## Requirements

macOS or Linux with OpenSSH and Git, an `rzv` sign-in or `RZV_API_KEY` with `sandboxes:write`, and Orca 1.4.246 or later. The connection uses the [sandbox SSH](sandboxes-ssh.md) path: short-lived certificates and strict host verification.

Back to the [documentation index](README.md).
