# rzv

`rzv` is the Runzivo command-line tool. It signs you in, lists your Runzivo workspaces and shows the status of your jobs.

This repository holds the source code of `rzv` (Go), its release notes and its releases. It is public so you can read the code you run; it does not accept outside contributions.

## Install

```sh
curl -fsSL https://runzivo.dev/downloads/rzv/install.sh | sh
```

The installer downloads the latest release, checks its SHA-256 checksum and puts `rzv` in `$RZV_INSTALL_DIR` (default `~/.local/bin`). `rzv` is one static executable with no runtime dependencies. More options, platforms and manual downloads: [docs/install.md](docs/install.md).

## Use

```sh
rzv login                              # sign in through your browser
rzv whoami                             # who you are signed in as
rzv workspaces list                    # workspaces you belong to
rzv workspaces select <workspace-id>   # choose one for later commands
rzv workspaces settings show            # name, logo and update capability
rzv workspaces members list             # members and permitted actions
rzv workspaces invitations list         # invitations and permitted actions
rzv jobs list                          # jobs for the selected workspace
rzv activity list                      # workspace activity log
rzv activity export > activity.csv     # matching CSV export
rzv activity status                    # audit signals and execution evidence
rzv jobs get <job-id>                  # one job's status
rzv api-keys create --name ci --permissions jobs:read,logs:read --idempotency-key <key>
rzv tickets list                       # support tickets for the selected workspace
rzv tickets open --kind technical_help --subject "Login issue" --body "Details"
rzv notifications list                  # notifications for the selected workspace
rzv notifications read --all            # mark every notification read
rzv logout                             # remove the stored sign-in
```

Add `--json` to any command for machine-readable output. `rzv` never prints your access token. Run `rzv help` for every command.

Runzivo is in Early Access. Sign in with your Runzivo account, then select a workspace you belong to. Runzivo runs jobs for repositories owned by GitHub organizations, including GitHub Free organizations. Personal repositories are unsupported. See the [CLI guide](https://runzivo.dev/docs/cli) and [getting started](https://runzivo.dev/docs/getting-started).

**Staff commands are not included.** Runzivo staff work stays in the staff console; `rzv` covers the customer API only.

## Documentation

- [docs/README.md](docs/README.md): index of the guides below
- [Sign-in and credentials](docs/login.md)
- [Commands](docs/commands.md), [workspace commands](docs/workspace-commands.md), [support tickets](docs/tickets.md), and [notifications](docs/notifications.md)
- [Development](docs/development.md)

## Releases and changes

- [CHANGELOG.md](CHANGELOG.md) lists what changed in each release.
- The [website changelog](https://runzivo.dev/changelog) ([RSS](https://runzivo.dev/changelog/rss.xml)) covers the CLI and the Runzivo service.

## Help and security

- Questions: support@runzivo.dev
- Security issues: see [SECURITY.md](SECURITY.md).

## License

Proprietary. See [LICENSE](LICENSE). Open-source components: [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
