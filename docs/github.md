# GitHub

| Command                               | What it does                                                           |
| ------------------------------------- | ---------------------------------------------------------------------- |
| `rzv github status`                   | The linked GitHub App installation and its repositories.               |
| `rzv github installations`            | Every linked installation, including setup and repository status.      |
| `rzv github connect`                  | Start linking an installation; prints a short-lived state.             |
| `rzv github authorize ...`            | Finish the link. Needs your own sign-in.                               |
| `rzv repositories list`               | Repositories every linked installation covers.                         |
| `rzv runner-profiles <repository-id>` | Runner sizes the repository can use now, with their `runs-on` label.   |
| `rzv runners <repository-id>`         | The same sizes, plus the workspace's running and waiting jobs on each. |

`rzv github authorize --state <state> --installation-id <id> --github-client-id <id>` uses GitHub's device flow: it prints a code to enter at github.com, then sends the resulting GitHub token to Runzivo once to prove you control the installation. `rzv` does not store that token.

`rzv github installations` supports a workspace API key with
`integrations:read` or `all:all`; the service still checks workspace access.
The linking and onboarding commands require your own sign-in.

## GitHub-backed workspace onboarding

Use `rzv github workspaces start` to create a short-lived onboarding state.
After installing the GitHub App, run `rzv github workspaces complete --state
<state> --installation-id <id> --github-client-id <id>`. It uses the same
device flow and does not store the resulting token. The result identifies
whether the workspace was created, was already available, is still pending,
or requires access. A returned workspace becomes the selected workspace.

Back to [Workspace commands](workspace-commands.md).
