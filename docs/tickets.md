# Support tickets

Use `rzv tickets` to work with support tickets you can access. The command uses your selected workspace unless you pass `--personal`. `--workspace <id>` selects a different workspace for one command. `--account <id>` remains an alias for `--workspace`.

```sh
rzv tickets list
rzv tickets open --kind technical_help --subject "Login issue" --body "What happened and when"
rzv tickets show <reference>
rzv tickets reply <reference> --body "Additional details"
```

For tickets outside a workspace, add `--personal`:

```sh
rzv tickets list --personal
rzv tickets open --personal --kind signup_help --subject "Account access" --body "Details"
```

`--personal` cannot be combined with `--workspace` or `--account`. Personal ticket commands use your signed-in identity and reject `RZV_API_KEY`. Workspace ticket commands accept an API key only when it has the `all:all` scope. The API still enforces the caller's workspace permissions.

The supported kinds are `technical_help`, `credit_request`, `professional_services`, and `signup_help`. To open a ticket, pass `--kind`, `--subject`, and `--body`. To reply, pass `--body`. Subjects are limited to 255 bytes. The encoded request, including its JSON structure, must fit within 16 KiB.

References are opaque values returned by `list` or `open`. Pass them unchanged to `show`, `reply` and `close`. Lists return the latest 100 visible tickets and do not support pagination. Ticket conversations also have no pagination.

All commands support `--json`. Text output escapes terminal control characters in ticket content. Responses larger than the CLI's 8 MiB response limit cannot be displayed.

`rzv tickets close <reference>` closes a ticket you opened; the ticket becomes `resolved` and the thread gets a closing note. Only the person who opened a ticket can close it, an already closed ticket is refused, and the customer API does not provide a reopen operation, so `rzv` has no such command.

Back to [Commands](commands.md).
