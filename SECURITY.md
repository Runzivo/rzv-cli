# Security

Report a security problem in `rzv`, its installer or the Runzivo service to **security@runzivo.dev**. Please do not open a public issue.

Include the release (`release-…` tag), your platform and the steps to reproduce. Do not send passwords, access tokens or other secrets.

## Verify a download

Every release publishes `SHA256SUMS` and a `.sha256` file for each executable. The installer checks the checksum before it installs. To check by hand:

```sh
shasum -a 256 -c SHA256SUMS --ignore-missing   # macOS
sha256sum -c SHA256SUMS --ignore-missing       # Linux
```

## What the checksums protect

- Runzivo builds the executables in CI from the release commit and publishes them with their checksums on this repository's GitHub Releases. `runzivo.dev/downloads/rzv/` redirects there.
- The checksums come from the same release as the files, over HTTPS. They catch a corrupted or incomplete download. They do not prove who published the release: anyone who could change the release could change both.
- Releases are not signed yet (no GPG, Sigstore or cosign signature). Trust rests on HTTPS, GitHub and the Runzivo/rzv-cli repository.

Only download `rzv` from this repository's Releases or from `https://runzivo.dev/downloads/rzv/`.
