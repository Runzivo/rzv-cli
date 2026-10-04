# rzv

`rzv` is the Runzivo command-line tool. It signs you in, lists the Runzivo accounts you belong to and shows the status of your jobs.

This repository publishes the official `rzv` executables and their release notes. It does not hold the source code.

## Install

```sh
curl -fsSL https://runzivo.dev/downloads/rzv/install.sh | sh
```

The installer downloads the latest release, checks its SHA-256 checksum and puts `rzv` in `$RZV_INSTALL_DIR` (default `~/.local/bin`). `rzv` is a single program with its runtime built in, so you do not install Node.js or anything else.

| Platform | Status |
| --- | --- |
| macOS, Apple Silicon (arm64) | Supported |
| Linux, x86-64 | Supported |
| macOS Intel, Windows, Linux arm64 | Not available yet |

On minimal Linux images, `rzv` needs `libatomic1`. The installer tells you if it is missing and prints the command to install it, for example `sudo apt-get install -y libatomic1`.

Other options:

- A specific release: `curl -fsSL https://runzivo.dev/downloads/rzv/install.sh | sh -s -- --version=release-2026-10-04-161209`
- Another directory: `curl -fsSL https://runzivo.dev/downloads/rzv/install.sh | RZV_INSTALL_DIR=/opt/rzv/bin sh`
- Uninstall: `curl -fsSL https://runzivo.dev/downloads/rzv/install.sh | sh -s -- --uninstall`

You can also download the files from [Releases](https://github.com/Runzivo/rzv-cli/releases) and check them against `SHA256SUMS`.

## Use

```sh
rzv login                          # sign in through your browser
rzv accounts list                  # accounts you can use
rzv accounts select <account-id>   # choose one for later commands
rzv jobs list                      # jobs for the selected account
rzv jobs get <job-id>              # one job's status
rzv logout                         # remove the stored sign-in
```

Add `--json` to any command for machine-readable output. `rzv` never prints your password or access token.

Runzivo is in Early Access. You need an approved Runzivo account to sign in. See the [CLI guide](https://runzivo.dev/docs/cli) and [getting started](https://runzivo.dev/docs/getting-started).

## Releases and changes

- [CHANGELOG.md](CHANGELOG.md) lists what changed in each release.
- The [website changelog](https://runzivo.dev/changelog) ([RSS](https://runzivo.dev/changelog/rss.xml)) covers the CLI and the Runzivo service.

## Help and security

- Questions: support@runzivo.dev
- Security issues: see [SECURITY.md](SECURITY.md).

## License

Proprietary. See [LICENSE](LICENSE). Bundled open-source components: [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
