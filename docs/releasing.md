# Releasing

Releases are tagged `release-YYYY-MM-DD-HHMMSS` (UTC), as before. `VERSION` holds the rzv version the executables report.

1. On a branch, set `VERSION`, rename the `## Unreleased` heading in `CHANGELOG.md` to `## <tag> (rzv <version>), <date>`, and merge the pull request.
2. Tag the merge commit on `main` and push the tag: `git tag <tag> && git push origin <tag>`.
3. `.github/workflows/release.yml` runs on the Runzivo runner. It checks the tag and the changelog section, runs the tests, cross-compiles every executable (`scripts/build.sh`), runs the installer against them (`scripts/test-install.sh`) and creates a **draft** release (`scripts/publish-draft.sh`).
4. Review the draft: files, `SHA256SUMS`, notes. Then publish it: `gh release edit <tag> --repo Runzivo/rzv-cli --draft=false --latest`.

`https://runzivo.dev/downloads/rzv/` redirects `LATEST` and `install.sh` to the latest published release, so publishing moves the installer to the new version.

## What a release contains

`rzv-linux-x64`, `rzv-linux-arm64`, `rzv-darwin-arm64`, `rzv-darwin-x64`, `rzv-win32-x64.exe`, a `.sha256` per executable, `SHA256SUMS`, `install.sh` and `LATEST`. All executables are built on Linux with `CGO_ENABLED=0 -trimpath`, no build ID and the commit time as the build date, so rebuilding a commit gives identical files. Anyone can check a release by building the tagged commit with the Go version in `go.mod` and comparing checksums.

## Signing plan

Today releases carry SHA-256 checksums only (see [SECURITY.md](../SECURITY.md)). The Go linker signs macOS executables ad hoc, which Apple Silicon requires to run them. Next steps, in order:

1. **Provenance**: GitHub artifact attestations (`actions/attest-build-provenance`, pinned by SHA) for every file, checked with `gh attestation verify`. No keys to manage.
2. **Checksum signature**: sign `SHA256SUMS` keylessly with Sigstore cosign from the release workflow, and have `install.sh` verify it when `cosign` is installed.
3. **macOS**: sign with a Developer ID certificate and notarize with `notarytool`. This needs an Apple Developer account and a macOS signing step, so it waits. Until then a browser download is quarantined; remove that with `xattr -d com.apple.quarantine rzv`, or use the installer, which `curl` does not quarantine.
4. **Windows**: Authenticode signing once a code-signing certificate exists.
5. **Homebrew** tap after signing is in place.

Back to the [documentation index](README.md).
