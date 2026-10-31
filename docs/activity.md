# Activity

| Command               | What it does                                                                                                                          |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| `rzv activity list`   | Activity rows, newest first, with optional category, actor, result, time, query and paging filters.                                   |
| `rzv activity export` | Matching activity rows as CSV on stdout (or a `{workspace_id,csv}` JSON wrapper with `--json`), limited by the server to 10,000 rows. |
| `rzv activity status` | Current component signals and execution evidence for 7, 30, or 60 days (30 by default).                                               |

List defaults to 50 rows. Its text and JSON output include the returned scope, retention period and next cursor for pagination.

Back to [Workspace commands](workspace-commands.md).
