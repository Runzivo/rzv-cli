# Running commands in a sandbox

```sh
rzv sandboxes exec <sandbox-id> [--cwd <dir>] [--timeout <seconds>] [--env A=1,B=2] -- <command...>
```

`exec` runs one command (through `bash -lc`) in a running sandbox and streams its output. Quote it as you would for `ssh`; put `--` before it so its own options are not read by `rzv`.

| Option                | Meaning                                               |
| --------------------- | ----------------------------------------------------- |
| `--cwd <dir>`         | Working directory (default `/workspace`)              |
| `--timeout <seconds>` | 1 to 3600 (default 300)                               |
| `--env A=1,B=2`       | Extra environment variables (no commas in the values) |

## Exit status

`rzv` exits with the command's exit status, with these exceptions:

- 124: the command ran past its timeout.
- 137: its output passed 8 MiB and it was stopped. Write large output to a file in the sandbox instead.
- When the sandbox stops, expires or is deleted during the command, `rzv` says so and exits with the command's status, or 1 if it has none.

A sandbox runs at most 4 commands at once. Stopping `rzv` (Ctrl-C) cancels the command in the sandbox.

## JSON output

With `--json` each output chunk is a line `{"type":"stdout"|"stderr","data":"<base64>"}`. The last line is `{"type":"exit","exit_code":N,"timed_out":bool,"truncated":bool}`, with `reason` (`sandbox_stopped`, `sandbox_expired` or `sandbox_deleted`) when the sandbox ended the command.

Back to [Sandboxes](sandboxes.md).
