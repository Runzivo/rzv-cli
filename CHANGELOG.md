# Changelog

Releases of the `rzv` executable, newest first. Each release lists its files and SHA-256 checksums on its [GitHub release page](https://github.com/Runzivo/rzv-cli/releases).

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
