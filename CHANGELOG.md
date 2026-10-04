# Changelog

Releases of the `rzv` executable, newest first. Each release lists its files and SHA-256 checksums on its [GitHub release page](https://github.com/Runzivo/rzv-cli/releases).

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
