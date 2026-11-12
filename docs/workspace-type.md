# Workspace type

- A workspace is `personal` (one person, one member) or an `organization`. The commands call this its `kind`.
- `rzv workspaces list` shows the type after the role, for example `- Acme [id] (acme, owner), personal (unconfirmed)`.
- `(unconfirmed)` means the service only inferred the type, so treat it as provisional.
- A workspace with no type shown has none set yet (an owner can confirm it), or the service is older and does not report types.
- `rzv workspaces confirm-kind --kind <personal|organization> [--yes] [--workspace <id|slug|name>] [--json]` confirms it. Owner only; needs a recent sign-in, and API keys are refused.
- Without `--yes` it is a dry run: it prints what would be set, changes nothing, and ends with `Run again with --yes to confirm.`
- With `--yes` it confirms the type and prints the change id. Once confirmed, only Runzivo staff can change it.
- Only an unconfirmed type can be confirmed; a confirmed one is refused with `kind_already_confirmed`.
- `personal` is refused when you already own another personal workspace, when the workspace has more than one member, or when it has a connected account that is not a personal account. Use `--kind organization` only if the workspace really belongs to an organization; otherwise contact Runzivo support before confirming.
- Once `personal` is confirmed, the workspace cannot be shared: invitations, new members and role increases are refused with `personal_workspace_not_shared` (exit 1, `--json` error). Demoting or removing an extra member still works.
- Nothing is moved, merged or renamed by confirming.
- `--json` prints `{"workspace_id", "confirmed", "workspace_kind":{"kind","kind_source","change_id"}}`; `change_id` is present only after `--yes`.
- Exit codes:
  - 1: the service refused (the type is already confirmed, or `personal` is not allowed).
  - 2: usage error, or the service rejected the request (`invalid_request`).
  - 3: the sign-in is too old; run `rzv login`, then retry.
  - 4: refused for access (not a member, not an owner, or an API key).
  - 5: the service is unavailable: it does not have the command yet, answered 500 or 503, or could not be reached.

Back to [Workspaces](workspaces.md) and the [command index](workspace-commands.md).
