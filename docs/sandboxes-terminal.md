# Sandbox terminal

## ssh

```sh
rzv sandboxes ssh <sandbox-id>
```

`ssh` opens an interactive shell in a running sandbox. It connects through Runzivo; nothing reaches the machine directly. Every key, Ctrl-C included, goes to the sandbox, so leave with the shell's own `exit`. It needs a terminal on macOS or Linux; use [`exec`](sandboxes-exec.md) in scripts. A sandbox has at most 2 terminals open at once.

The session ends, and `rzv` says why, when:

- the sandbox stops, expires or is deleted;
- 30 minutes pass with no input and no output;
- input arrives faster than 256 KiB a second;
- output is not read fast enough.

A lost connection does not keep the shell. Run `ssh` again for a new session.

## terminal-ticket

```sh
rzv sandboxes terminal-ticket <sandbox-id> [--json]
```

`terminal-ticket` prints a WebSocket URL for the same terminal, with a ticket in its `ticket` query parameter. It is for a client that cannot send an `Authorization` header, such as a browser. The URL works once, within about 30 seconds. stdout holds only the URL; a one-line note on stderr gives the expiry. `--json` prints `workspace_id`, `sandbox_id`, `url`, `ticket`, `expires_at` and `connect_url` (the URL with the ticket added). Treat it as a password until it is used: anyone who has it can open the shell. `ssh` does not need a ticket.

A sandbox can hold 10 unused tickets; after that, wait for them to expire.

Back to [Sandboxes](sandboxes.md).
