# Install

```sh
curl -fsSL https://runzivo.dev/downloads/rzv/install.sh | sh
```

The installer detects your platform, downloads the matching executable and its `.sha256` file, refuses to install when the checksum does not match, and puts `rzv` in `$RZV_INSTALL_DIR` (default `~/.local/bin`).

Options:

- A specific release: `curl -fsSL https://runzivo.dev/downloads/rzv/install.sh | sh -s -- --version=<release-tag>`
- Another directory: `curl -fsSL https://runzivo.dev/downloads/rzv/install.sh | RZV_INSTALL_DIR=/opt/rzv/bin sh`
- Uninstall: `curl -fsSL https://runzivo.dev/downloads/rzv/install.sh | sh -s -- --uninstall`

## Platforms

| Platform             | File                |
| -------------------- | ------------------- |
| Linux, x86-64        | `rzv-linux-x64`     |
| Linux, arm64         | `rzv-linux-arm64`   |
| macOS, Apple Silicon | `rzv-darwin-arm64`  |
| macOS, Intel         | `rzv-darwin-x64`    |
| Windows, x86-64      | `rzv-win32-x64.exe` |

Every file is a static executable with no runtime dependencies. On Windows, download `rzv-win32-x64.exe` from [Releases](https://github.com/Runzivo/rzv-cli/releases), check it as below and put it on your `PATH`.

## Check a download by hand

Each release publishes `SHA256SUMS` and a `.sha256` file per executable:

```sh
shasum -a 256 -c SHA256SUMS --ignore-missing   # macOS
sha256sum -c SHA256SUMS --ignore-missing       # Linux
```

On Windows: `Get-FileHash rzv-win32-x64.exe -Algorithm SHA256`, then compare with `SHA256SUMS`. What checksums do and do not prove: [SECURITY.md](../SECURITY.md).

Back to the [documentation index](README.md).
