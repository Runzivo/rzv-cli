#!/usr/bin/env bash
# Runs install.sh against a built release directory (scripts/build.sh output)
# through file:// URLs: a good install, the installed binary's version, a
# tampered checksum that must be refused, and uninstall.
#   bash scripts/test-install.sh <release-dir>
set -euo pipefail

dist="$(cd -- "${1:?usage: test-install.sh <release-dir>}" && pwd)"
root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
tag="$(tr -d '[:space:]' <"${dist}/LATEST")"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

fail() {
  echo "test-install: $1" >&2
  exit 1
}

# The installer reads <base>/<tag>/<file>; lay the release out that way.
mkdir -p "${work}/base/${tag}"
cp "${dist}"/rzv-* "${work}/base/${tag}/"
cp "${dist}/LATEST" "${work}/base/LATEST"
base="file://${work}/base"

RZV_INSTALL_BASE_URL="$base" RZV_INSTALL_DIR="${work}/bin" sh "${dist}/install.sh" >"${work}/out.txt"
grep -q "Checksum verified." "${work}/out.txt" || fail "no checksum confirmation"
"${work}/bin/rzv" version | grep -q "rzv $(tr -d '[:space:]' <"${root}/VERSION")" || fail "installed rzv reports the wrong version"

# Tamper with the checksum of this platform's file: install must fail.
case "$(uname -s)-$(uname -m)" in
  Linux-x86_64) asset=rzv-linux-x64 ;;
  Linux-aarch64) asset=rzv-linux-arm64 ;;
  Darwin-arm64) asset=rzv-darwin-arm64 ;;
  Darwin-x86_64) asset=rzv-darwin-x64 ;;
  *) fail "no release file for this platform" ;;
esac
printf '%064d  %s\n' 0 "$asset" >"${work}/base/${tag}/${asset}.sha256"
rm -rf "${work}/bin2"
if RZV_INSTALL_BASE_URL="$base" RZV_INSTALL_DIR="${work}/bin2" sh "${dist}/install.sh" >/dev/null 2>"${work}/err.txt"; then
  fail "a tampered checksum was accepted"
fi
grep -q "Refusing to install" "${work}/err.txt" || fail "no refusal message"
[ ! -e "${work}/bin2/rzv" ] || fail "rzv installed despite a bad checksum"

RZV_INSTALL_DIR="${work}/bin" sh "${dist}/install.sh" --uninstall | grep -q Removed || fail "uninstall"
[ ! -e "${work}/bin/rzv" ] || fail "rzv still present after uninstall"
echo "install.sh checks passed for ${tag}"
