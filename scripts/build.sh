#!/usr/bin/env bash
# Cross-compiles the rzv release files into <out-dir>:
#   rzv-linux-x64, rzv-linux-arm64, rzv-darwin-arm64, rzv-darwin-x64,
#   rzv-win32-x64.exe, a .sha256 per executable, install.sh, LATEST (the tag)
#   and SHA256SUMS over everything except the .sha256 files.
# Static (CGO_ENABLED=0) and reproducible: -trimpath, no build ID, and the
# build date is the commit time, so the same commit gives the same bytes.
#   bash scripts/build.sh <out-dir> <release-tag>
set -euo pipefail

out="${1:?usage: build.sh <out-dir> <release-tag>}"
tag="${2:?usage: build.sh <out-dir> <release-tag>}"
root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

version="$(tr -d '[:space:]' <VERSION)"
commit="$(git rev-parse HEAD)"
date="$(TZ=UTC git log -1 --format=%cd --date=format-local:%Y-%m-%dT%H:%M:%SZ)"
pkg=github.com/Runzivo/rzv-cli/internal/version
ldflags="-s -w -buildid= -X ${pkg}.Version=${version} -X ${pkg}.Commit=${commit} -X ${pkg}.Date=${date}"

# GOOS/GOARCH -> published file name (names kept from earlier releases).
targets=(
  "linux amd64 rzv-linux-x64"
  "linux arm64 rzv-linux-arm64"
  "darwin arm64 rzv-darwin-arm64"
  "darwin amd64 rzv-darwin-x64"
  "windows amd64 rzv-win32-x64.exe"
)

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'; else shasum -a 256 "$1" | awk '{print $1}'; fi
}

rm -rf "$out"
mkdir -p "$out"
for target in "${targets[@]}"; do
  read -r goos goarch name <<<"$target"
  echo "Building ${name} (${goos}/${goarch}, rzv ${version})" >&2
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -trimpath -ldflags "$ldflags" -o "${out}/${name}" ./cmd/rzv
  printf '%s  %s\n' "$(sha256 "${out}/${name}")" "$name" >"${out}/${name}.sha256"
done
cp install.sh "${out}/install.sh"
printf '%s\n' "$tag" >"${out}/LATEST"
(
  cd "$out"
  for f in rzv-* install.sh LATEST; do
    case "$f" in *.sha256) continue ;; esac
    printf '%s  %s\n' "$(sha256 "$f")" "$f"
  done
) >"${out}/SHA256SUMS"
echo "Built rzv ${version} (${commit:0:7}) into ${out}" >&2
