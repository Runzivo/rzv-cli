# Running commands in a sandbox

```sh
rzv sandboxes exec <sandbox-id> [--cwd <dir>] [--timeout <seconds>] [--env A=1,B=2] -- <command...>
```

`exec` runs one command (through `bash -lc`) in a running sandbox and streams its output. Put `--` before it so its own options are not read by `rzv`.

- One argument after `--` is a shell script, so pipes, `;` and `$VARS` work: `rzv sandboxes exec <id> -- 'make test | tail -5'`.
- Two or more arguments are run as that exact argument list. `rzv` quotes each one for the shell, so spaces, quotes, `$`, `;` and newlines arrive unchanged: `rzv sandboxes exec <id> -- sh -c 'exit 7'` exits with 7.

Behaviour change: pipes, `&&`, `$VARS` and `NAME=value` prefixes given as separate arguments are now passed literally. Put the whole command in one argument to use shell syntax: instead of `-- FOO=1 make test`, use `-- 'FOO=1 make test'`.

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
