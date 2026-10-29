# Sandboxes

A sandbox is a Linux virtual machine in your workspace. Your files live in `/workspace` and survive a stop and a start. It lasts **eight hours from its first start**, running or stopped; then the machine and its files are deleted. Time stopped counts toward the eight hours, and the lifetime cannot be extended.

```sh
rzv sandboxes create --name dev [--size 2vcpu-8gb|4vcpu-16gb] [--disk-gib 20]
rzv sandboxes list                       # status, time left and credits used
rzv sandboxes get <sandbox-id>
rzv sandboxes exec <sandbox-id> -- npm test
rzv sandboxes ssh <sandbox-id>           # an interactive terminal (macOS and Linux)
rzv sandboxes stop <sandbox-id>          # keeps the files, stops the charge
rzv sandboxes start <sandbox-id>
rzv sandboxes delete <sandbox-id>        # deletes the machine and its files
```

Add `--json` to any command except `ssh`. A workspace may run a limited number of sandboxes at once; `list` shows the limit.

## Running commands

`exec` runs one command (through `bash -lc`) in `/workspace` and streams its output. Quote it as you would for `ssh`; put `--` before it so its own options are not read by `rzv`.

| Option                | Meaning                                               |
| --------------------- | ----------------------------------------------------- |
| `--cwd <dir>`         | Working directory (default `/workspace`)              |
| `--timeout <seconds>` | 1 to 3600 (default 300)                               |
| `--env A=1,B=2`       | Extra environment variables (no commas in the values) |

`rzv` exits with the command's exit status, and with 124 if it timed out. With `--json` each output chunk is a line with base64 `data`, followed by an `exit` line.

`ssh` connects through Runzivo; nothing reaches the machine directly. Every key, Ctrl-C included, goes to the sandbox, so leave with the shell's own `exit`. It needs a terminal on macOS or Linux; use `exec` in scripts.

## Billing and permissions

Compute is charged per minute while the sandbox runs, at the same equivalent-minute rate as the runner of that size (2 vCPU is 1x, 4 vCPU is 2x). A stopped sandbox is not charged. If the workspace runs out of credit the sandbox is stopped (`credit_exhausted`) and starts again once credit is added.

Changing sandboxes needs the `sandboxes:write` permission, which an API key gets only with the `all:all` scope; reading needs `sandboxes:read`.

Back to the [documentation index](README.md).
