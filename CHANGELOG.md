# Changelog

Releases of the `rzv` executable, newest first. Each release lists its files and SHA-256 checksums on its [GitHub release page](https://github.com/Runzivo/rzv-cli/releases).

## Unreleased

### Added

- [Claude Code in a sandbox](docs/claude-code.md): use the preinstalled Claude Code with your own Anthropic API key or Claude subscription login. The key or login stays in the sandbox, and only `/workspace` survives a stop and start. Runzivo charges only the sandbox's compute.
- [Cursor Cloud Agents on a sandbox](docs/cursor-worker.md): run a Cursor Self-Hosted Machines worker in a sandbox with your own Cursor user API key. The key is sent over SSH into an owner-only file outside `/workspace`. Runzivo charges only the sandbox's compute. Not yet verified with a live Cursor account.

## release-2026-11-06-113624 (rzv 0.25.0), 2026-11-06

### Added

- `rzv runners profiles list|create|delete` manage [custom runner profiles](docs/custom-runner-profiles.md): vCPU (2 to 32), memory (4 to 128 GiB) and temporary disk (16 to 256 GiB) chosen independently, saved under a label whose workflow label goes in `runs-on`. `rzv runners availability` shows whether each profile, or a shape given with `--vcpu` and `--mem-gib`, can run now, per region, with its equivalent-minute rate and a shared CPU (overcommitted) note. All support `--json`. A Runzivo environment where custom runner profiles are off or not available says so and exits with the unavailable code.
- [OpenCode in a sandbox](docs/opencode.md): install a pinned, checksummed OpenCode 1.19.5 in a sandbox and use it with your own AI provider key. The key stays in the sandbox. Runzivo charges only the sandbox's compute.
- `rzv sandboxes ssh-config <sandbox-id> --write` prints the commands to connect with `ssh`, VS Code (`code --remote ssh-remote+<sandbox-id>.rzv /workspace`) and Cursor. The new [VS Code and Cursor Remote SSH](docs/editors.md) guide has the editor steps, settings for sandboxes without outbound internet, and the limits. VS Code Remote - SSH has been tested against a live sandbox.
- [Herdr in a sandbox](docs/herdr.md) explains how to run [Herdr](https://github.com/herdrdev/herdr) 0.9.5 against a sandbox over the `<sandbox-id>.rzv` SSH alias with `herdr --remote`, `ssh -t` or `herdr machine add`. It covers Herdr's own install prompt and a pinned, checksum-verified manual install. Panes and agents keep running while you are disconnected and the sandbox runs. A stop removes Herdr, its server and its panes; only `/workspace` is kept. `scripts/20-herdr-e2e.sh` checks these steps against one sandbox and deletes it afterwards.

## release-2026-11-06-005258 (rzv 0.24.0), 2026-11-06

### Changed

- Sign-ins renew themselves. `rzv login` keeps the identity provider's refresh token in the owner-only credentials file, and any command (including SSH, `rzv orca` and editors that run `rzv`) renews the access token shortly before it expires. Sessions follow the customer session policy: 14 days without use or 30 days after sign-in. Parallel runs renew once under a lock. `rzv logout` and `rzv profiles remove` end the session at the identity provider. Sign-ins from before this release keep working until their access token expires; then run `rzv login` once.

## release-2026-11-05-233903 (rzv 0.23.0), 2026-11-05

### Added

- `rzv orca create|suspend|resume|destroy` run each Orca workspace in its own sandbox for the [Runzivo plugin for Orca](https://github.com/Runzivo/rzv-orca-plugin): one sandbox per workspace (the workspace id is the idempotency key), the selected commit copied in as a Git bundle with no Git credentials, SSH through the `ssh-config --write` config, and Orca's recipe result on stdout. A failed create deletes its sandbox. `rzv orca defaults` sets the disk, lifetime, size and workspace these use; `rzv orca status` lists them with expiry and credits used. See [Orca workspaces](docs/orca.md).
- `rzv sandboxes create --lifetime 8h|12h|24h` sets how long a sandbox lives (default 8h). Where Runzivo offers 12h and 24h, the timer starts when the sandbox first runs; elsewhere the 8h timer starts at the first start. `--lifetime 8h` is left out of the request, so it works with older services; a service without the lifetime choice explains that 12h and 24h are not supported yet. `sandboxes list` and `get` show the lifetime, the exact expiry in local time and UTC, and the time left. A sandbox shows `cleanup pending` when it expired or was deleted but its files are not deleted yet. Older services without the field show the lifetime as `-`, and their lifetimes on offer as 8h.
- `rzv workspaces settings update --docker-cache enabled|disabled` and `--docker-cache-size-mib <MiB>` (64 to 16384) set the workspace Docker layer cache. Either flag works alone. `settings show` and `update` print the cache state, size and whether hosts in this environment can serve the cache (`available`); `--json` includes `docker_cache`. The cache is available only after an operator turns it on and an execution host in the environment supports it. Older services without the setting show no cache line.

## release-2026-11-01-185141 (rzv 0.22.0), 2026-11-01

### Added

- On macOS and Linux, `rzv sandboxes ssh` uses system OpenSSH with short-lived user certificates, strict host CA verification, remote exit status and local forwarding. `ssh --stdio` / `ssh-proxy` carry raw bytes with half-close support. `ssh-config` prints configuration or maintains an owned Include with `--write`, enabling ssh, scp, sftp, rsync and Remote SSH editors. Certificate refresh runs before each connection. Requires the matching Core SSH release and enablement.
- `rzv sandboxes terminal` preserves the existing PTY terminal and is the fallback for an interactive shell when OpenSSH is missing or the environment has SSH disabled or lacks its routes. Native sandbox OpenSSH on Windows is not included yet.

- `rzv activity status` includes the running lease count in human output and `--json`. Older Core responses without the field show unknown in human output and omit the count in JSON.

### Changed

- Behaviour change: when `rzv sandboxes exec` gets two or more arguments after `--`, pipes, `&&`, `$VARS` and `NAME=value` prefixes given as separate arguments are now passed literally. Put the whole command in one argument to use shell syntax: instead of `-- FOO=1 make test`, use `-- 'FOO=1 make test'`.

### Fixed

- `rzv sandboxes ssh`, `ssh --stdio` and the `ssh-config` refresh wait up to 60 seconds, honouring `Retry-After`, while a newly started sandbox's SSH service starts, with one status line on stderr. They no longer fail with a host-key error in the first seconds after a start.

- SSH remote commands beginning with option-like text stay remote commands. Managed SSH config preserves file modes and existing Include lines; symlinked configs are refused with guidance. SSH lifetime checks tolerate modest clock skew, and normal lifecycle closes report their reason.

- Workspace selection accepts exact ids, case-insensitive slugs and case-insensitive display names, in that order. Ambiguous names or slugs are refused with matching ids. Lists show slugs, and selection and `whoami` show the name and id.

- `rzv sandboxes exec` with two or more arguments after `--` runs that exact argument list. Each argument is quoted for the shell instead of joined with spaces, so `rzv sandboxes exec <id> -- sh -c 'exit 7'` exits with 7 and arguments with spaces, quotes, `$` or `;` are no longer split or expanded. One argument is still run as a shell script.

## release-2026-10-31-193326 (rzv 0.21.0), 2026-10-31

### Added

- `rzv sandboxes create --size` accepts `8vcpu-32gb` and `16vcpu-64gb` as well as `2vcpu-8gb` and `4vcpu-16gb`. Which sizes can start now depends on the environment's capacity; `rzv sandboxes list` shows them.

### Fixed

- `rzv sandboxes exec` quotes each argument, so the sandbox runs exactly the command you typed.
- The sandboxes help and docs describe API key access correctly: `sandboxes:read` and `sandboxes:write` are permissions, granted with `rzv api-keys create --permissions`, or use the `all:all` scope.

## release-2026-10-31-012059 (rzv 0.20.0), 2026-10-31

### Added

- `rzv sandboxes list|get|create|start|stop|delete|exec|ssh`: create sandboxes (Linux virtual machines that last eight hours from their first start), run commands in them with `exec` (streamed output, exit status passed through) and open an interactive terminal with `ssh` on macOS and Linux (your terminal is put back on exit, SIGTERM and SIGHUP). `--json` on every command except `ssh`; `list` shows the sizes the environment offers. Needs a Runzivo environment with Sandboxes turned on.
- `rzv sandboxes terminal-ticket`: print a single-use terminal URL for a client that cannot send an `Authorization` header, such as a browser. `exec` exits with 137 when output passes 8 MiB and says when the sandbox stopped, expired or was deleted during the command (`--json` exit lines carry `truncated` and `reason`). `ssh` says why the terminal closed after 30 idle minutes, too much input or unread output.

### Fixed

- A command run without its subcommand, such as `rzv sandboxes`, prints its usage on separate lines instead of escaping each newline as `\u000a`. Other control characters in error messages are still escaped.

## release-2026-10-29-002253 (rzv 0.19.0), 2026-10-29

### Changed

- The prd sign-in moves to `https://id.runzivo.dev/realms/rzv-prd`. Run `rzv login` after upgrading; a sign-in made with an older rzv does not work with the new identity host.

## release-2026-10-28-202940 (rzv 0.18.0), 2026-10-28

### Added

- `rzv login -p <profile> [-w <slug|id>]`: one sign-in per profile, so a second sign-in no longer replaces the first. `-p`/`--profile` works on every command, and `RZV_PROFILE` sets the profile for a shell.
- `rzv profiles list|use|remove`: list local profiles, switch the active one, or delete a profile's local sign-in. Tokens are never printed.
- `RZV_WORKSPACE`: the default workspace (slug or id) when a command names none.
- `rzv whoami` shows the profile in use.

### Changed

- `--workspace` (and `-w`) accepts a workspace slug as well as an id, on every workspace-scoped command. With `RZV_API_KEY`, pass the id.

## release-2026-10-28-172656 (rzv 0.17.0), 2026-10-28

### Added

- `rzv activity list|export|status`: workspace activity with filters and paging, CSV export, and audit signals with execution evidence.
- `rzv api-keys create --permissions <p,...>`: fine-grained API key permissions; list, create and rename show them.
- `rzv notifications list|read|settings`: list and mark workspace notifications read, and read or change delivery settings. Needs your own sign-in.
- `rzv jobs logs <job-id>` and `rzv insights summary|heatmap|slow-jobs`: job log text and workspace job metrics.
- `rzv ai providers|settings|usage`: workspace AI provider connections, configuration and usage. Provider keys are read only from `--key-file`.
- `rzv github installations` and `rzv github workspaces start|complete`: every linked installation with its setup state, and GitHub-backed workspace onboarding.
- `rzv billing codes redeem --code-file <path>`: redeem a promo code or prepaid voucher.
- `rzv workspaces closures list|request|cancel`: ask staff to close a workspace, or withdraw the request. Nothing is deleted immediately.

## release-2026-10-27-000402 (rzv 0.16.0), 2026-10-27

### Added

- `rzv tickets close <reference> [--workspace <id> | --personal]`: close a support ticket you have open. The ticket is marked resolved. A ticket that is already closed says so.

## release-2026-10-26-235057 (rzv 0.15.0), 2026-10-27

### Added

- `rzv teams list|show|create|rename|permissions|add-member|remove-member|delete`: manage the teams of the selected workspace.
- `rzv permissions`: your permissions in the workspace, with the permission catalogue and presets.

## release-2026-10-26-231251 (rzv 0.14.0), 2026-10-26

### Added

- `rzv workspaces settings show|update`: read and change the workspace name and logo.
- `rzv workspaces members list|role|remove`: list workspace members, change a member's role, or remove a member.
- `rzv workspaces invitations list|invite|resend|revoke|teams|accept`: manage invitations to a workspace, and accept one with a token.
- `rzv billing country reset`: returns the workspace billing country to unset. Use it when the country was chosen in error. Future purchases then ask for a country again.

## release-2026-10-26-230022 (rzv 0.13.0), 2026-10-26

### Added

- `rzv billing invoices`, `rzv billing auto-recharge` (read and `set`), `rzv billing country` (read and `set`), `rzv billing marketplace`, `rzv billing checkout --pack --operation-id`, and `rzv billing portal`.
- Turning auto-recharge on needs a person signed in, not an API key.
- A refused change says why: a workspace role that does not allow it, or `rzv login` again when the sign-in is too old.

## release-2026-10-26-185254 (rzv 0.12.0), 2026-10-26

### Added

- `rzv concurrency`: the concurrent-job limit, jobs running now and what is left.
- `rzv runners`: runner profiles and inventory for the workspace.

## release-2026-10-26-185007 (rzv 0.11.0), 2026-10-26

### Added

- `rzv gitlab connections`: the GitLab connections for the selected workspace.
- `rzv gitlab connect` and `rzv gitlab disconnect`. A connection id must match `glc_` plus 24 lowercase hex digits.

### Changed

- When an integration is not enabled for the account, the command says so in plain words instead of printing the raw error code.

## release-2026-10-26-181559 (rzv 0.10.0), 2026-10-26

### Added

- `rzv tickets list|open|show|reply`: support tickets for the selected workspace. `--personal` works with your own tickets instead, and rejects `RZV_API_KEY`. Output supports `--json`.

### Changed

- The onboarding docs say Runzivo is for organizations only.

## release-2026-10-19-122318 (rzv 0.9.0), 2026-10-19

### Changed

- `rzv` is rewritten in Go and its source code is now in this repository. Each executable is one static file with no runtime dependencies; Linux no longer needs `libatomic1`.
- `rzv accounts` is now `rzv workspaces`; the old name still works. `--workspace` replaces `--account`, which still works.

### Added

- Executables for Linux arm64, macOS Intel and Windows x86-64, next to Linux x86-64 and macOS Apple Silicon. Builds are reproducible from the tagged commit.
- `rzv whoami`: the signed-in identity, environment, session expiry and selected workspace.
- `rzv login --env prd|shared` and `--no-browser`. Production is the default, with no other options needed.
- `rzv version`.
- Workspace commands at parity with the 0.8.0 CLI: `rzv jobs list|get|status`, `rzv usage`, `rzv billing` and `rzv billing purchases`, `rzv github status|connect|authorize`, `rzv runner-profiles`, `rzv api-keys list|create|rename|revoke`, and the new `rzv repositories list`.
- `RZV_API_KEY` works without a stored sign-in: pass `--workspace`, and `RZV_API_URL` or `RZV_ENV` picks the API.

## release-2026-10-04-161209 (rzv 0.1.0), 2026-10-04

### Fixed

- Linux: the installer now checks that the system libraries `rzv` needs are present. On minimal images such as Ubuntu 24.04 or Debian 12 slim it names the missing library (usually `libatomic1`) and prints the command to install it. Before, `rzv` installed but failed to start with no explanation.

### Unchanged

- The `rzv` executables are the same as in `release-2026-10-03-033543` (same checksums).

### Service changes in the same release

- The first request after a Runzivo service restart no longer fails with "The customer API is unavailable".

## release-2026-10-03-033543 (rzv 0.1.0), 2026-10-03

First public release.

### Added

- One-command installer for macOS (Apple Silicon) and Linux (x86-64), with checksum verification. No Node.js needed.
- `rzv login` and `rzv logout`: sign in through the browser; the token is stored locally and never printed.
- `rzv accounts list` and `rzv accounts select`: see and choose the Runzivo accounts you belong to.
- `rzv jobs list` and `rzv jobs get`: read job status for the selected account.
- `--json` output on every command.
