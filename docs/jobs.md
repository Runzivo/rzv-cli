# Jobs, insights and usage

| Command                                             | What it does                                                       |
| --------------------------------------------------- | ------------------------------------------------------------------ |
| `rzv jobs list`                                     | Every job, newest first: status, conclusion, repository, workflow. |
| `rzv jobs get <job-id>`                             | One job in detail. `rzv jobs status <job-id>` is the same.         |
| `rzv jobs logs <job-id>`                            | Log text, or the API's reason it is unavailable.                   |
| `rzv insights summary [--days <n>]`                 | Job and minute totals for 1 to 90 UTC days.                        |
| `rzv insights heatmap [--days <n>]`                 | Queue activity by UTC weekday and hour.                            |
| `rzv insights slow-jobs [--days <n>] [--limit <n>]` | Workflows ranked by P90 duration, with a limit from 1 to 25.       |
| `rzv usage`                                         | Recorded billable minutes and job count.                           |
| `rzv concurrency`                                   | Concurrent-job limit, jobs running now and what is left.           |

`rzv concurrency` and `rzv runners` are read-only and need the `runners:read` scope when used with an API key. The limit is set by Runzivo for the workspace; `rzv` cannot change it. The running jobs on every listed size, plus the running jobs on the "no longer listed" line when `rzv runners` prints one, add up to the running count that `rzv concurrency` reports.

A job's `status` is Runzivo's view of the run; only `conclusion` `success` means the GitHub workflow job succeeded.

Back to [Workspace commands](workspace-commands.md).
