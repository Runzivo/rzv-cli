# Sandboxes

A sandbox is a Linux virtual machine in your workspace. Your files live in `/workspace` and survive a stop and a start until the sandbox expires.

## Lifetime

Choose how long a sandbox lives when you create it: `--lifetime 8h` (the default), `12h` or `24h`. No other value is accepted. If you leave the flag out, the sandbox lives 8 hours. 12h and 24h are offered only where Runzivo has turned them on; `list` shows the lifetimes on offer. There the timer starts when the sandbox first runs. Elsewhere only 8h is offered and the timer starts at the first start. Asking for one that is not offered fails with `lifetime_unavailable` (HTTP 409). Choose another, or omit `--lifetime`. A Runzivo service from before the lifetime choice refuses `--lifetime 12h` or `24h` as an invalid request; `rzv` checks that the server really sends no lifetimes list, and only then says it does not support choosing a lifetime yet. Any other invalid request is reported as it is.

- The timer starts when the sandbox first becomes running. Until then the full lifetime is left and `list` and `get` show "starts when running" for the expiry.
- Stopping, starting and restarting do not move the timer. Time spent stopped counts.
- The lifetime cannot be extended. Create a new sandbox instead.
- At expiry the virtual machine stops, sessions end, the temporary files (the workspace disk) are deleted and compute billing stops.

`list` and `get` show the lifetime, the exact expiry in your local time and in UTC (for example `2026-11-02 18:04 EST (23:04 UTC)`) and the time left (for example `7h 42m`). `--json` includes `lifetime_hours`, `expires_at` and `remaining_seconds`. An older Runzivo service may not send `lifetime_hours`; the lifetime then shows as `-`.

## Cleanup pending

A sandbox shows `cleanup pending` (`cleanup_pending` in JSON) when it expired or was deleted but its files are not deleted yet. It is not deleted until that finishes. Runzivo operations is alerted and keeps retrying, so you do not need to do anything.

## Commands

```sh
rzv sandboxes create --name dev [--size 2vcpu-8gb|4vcpu-16gb|8vcpu-32gb|16vcpu-64gb] [--disk-gib 20] [--lifetime 8h|12h|24h]
rzv sandboxes list                       # status, lifetime, expiry, time left and credits used
rzv sandboxes get <sandbox-id>
rzv sandboxes stop <sandbox-id>          # keeps the files until expiry, stops the charge
rzv sandboxes start <sandbox-id>
rzv sandboxes delete <sandbox-id>        # deletes the machine and its files
```

Add `--json` to list, get, create, start, stop, delete, exec or terminal-ticket. A workspace may run a limited number of sandboxes at once; `list` shows the limit and the sizes and lifetimes this Runzivo environment offers.

`create` takes `--idempotency-key <key>` (8 to 200 characters). Running it again with the same key returns the sandbox the first run created instead of a second one.

| Page                                            | What it covers                                    |
| ----------------------------------------------- | ------------------------------------------------- |
| [Running commands](sandboxes-exec.md)           | `rzv sandboxes exec`, its options and exit status |
| [SSH and file transfers](sandboxes-ssh.md) | OpenSSH, scp, sftp, rsync and Remote SSH editors |
| [VS Code and Cursor](editors.md)                | Remote SSH editors in `/workspace`                |
| [Terminal](sandboxes-terminal.md)               | `terminal`, `ssh` and `terminal-ticket`         |
| [Herdr](herdr.md)                               | Agent panes that outlive the SSH connection       |
| [Billing and permissions](sandboxes-billing.md) | What is charged and who may change sandboxes      |
| [OpenCode](opencode.md)                         | Run OpenCode with your own AI provider key        |
| [Cursor Cloud Agents](cursor-worker.md)         | A Cursor worker for Cloud Agents (not yet verified) |

Back to the [documentation index](README.md).
