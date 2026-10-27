# Changelog

Releases of the `rzv` executable, newest first. Each release lists its files and SHA-256 checksums on its [GitHub release page](https://github.com/Runzivo/rzv-cli/releases).

## release-2026-10-27-000402 (rzv 0.16.0), 2026-10-26

### Added

- `rzv tickets close <reference> [--workspace <id> | --personal]`: close a support ticket you have open. The ticket is marked resolved. A ticket that is already closed says so.

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
