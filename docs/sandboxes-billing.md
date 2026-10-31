# Sandbox billing and permissions

## Billing

Compute is charged per minute while the sandbox runs, at the same equivalent-minute rate as the runner of that size (2 vCPU is 1x, 4 vCPU is 2x, 8 vCPU is 4x, 16 vCPU is 8x). A stopped sandbox is not charged. `list` and `get` show the credits each sandbox has used so far.

Creating or starting a sandbox needs credit in the workspace. If the workspace runs out of credit while a sandbox runs, the sandbox fails with `credit_exhausted` and its machine stops. Start it again once credit is added.

## Permissions

Reading sandboxes needs the `sandboxes:read` permission. Creating, starting, stopping, deleting, `exec`, `ssh` and `terminal-ticket` need `sandboxes:write`. An API key reads with the `sandboxes:read` or `sandboxes:write` permission and makes changes with `sandboxes:write` (grant them with `rzv api-keys create --permissions`), or with the `all:all` scope.

Back to [Sandboxes](sandboxes.md).
