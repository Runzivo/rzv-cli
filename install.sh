#!/bin/sh
# Installs the rzv executable for this machine and verifies its SHA-256
# checksum before putting it in place. POSIX sh; needs curl and sha256sum or
# shasum.
#
#   curl -fsSL https://runzivo.dev/downloads/rzv/install.sh | sh
#   curl -fsSL https://runzivo.dev/downloads/rzv/install.sh | sh -s -- --version=<release-tag>
#   curl -fsSL https://runzivo.dev/downloads/rzv/install.sh | sh -s -- --uninstall
#
# RZV_INSTALL_DIR: install location (default $HOME/.local/bin).
# RZV_INSTALL_BASE_URL: download base (default https://runzivo.dev/downloads/rzv,
# which redirects to this repository's GitHub releases). Tests point it at a
# local directory.
set -eu

BASE_URL="${RZV_INSTALL_BASE_URL:-https://runzivo.dev/downloads/rzv}"
INSTALL_DIR="${RZV_INSTALL_DIR:-${HOME}/.local/bin}"

version=""
uninstall=false
for arg in "$@"; do
  case "$arg" in
    --version=*) version="${arg#--version=}" ;;
    --uninstall) uninstall=true ;;
    *)
      echo "Unknown option: $arg" >&2
      exit 2
      ;;
  esac
done

detect_target() {
  os="$(uname -s)"
  arch="$(uname -m)"
  case "$os" in
    Darwin) platform="darwin" ;;
    Linux) platform="linux" ;;
    MINGW* | MSYS* | CYGWIN*)
      echo "On Windows, download rzv-win32-x64.exe from https://github.com/Runzivo/rzv-cli/releases and check it against SHA256SUMS." >&2
      exit 1
      ;;
    *)
      echo "rzv does not support this operating system ($os)." >&2
      exit 1
      ;;
  esac
  case "$arch" in
    arm64 | aarch64) arch="arm64" ;;
    x86_64 | amd64) arch="x64" ;;
    *)
      echo "rzv does not support this architecture ($arch)." >&2
      exit 1
      ;;
  esac
  printf '%s-%s' "$platform" "$arch"
}

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

do_uninstall() {
  if [ -e "${INSTALL_DIR}/rzv" ]; then
    rm -f "${INSTALL_DIR}/rzv"
    echo "Removed ${INSTALL_DIR}/rzv."
  else
    echo "rzv is not installed in ${INSTALL_DIR}."
  fi
}

main() {
  if [ "$uninstall" = true ]; then
    do_uninstall
    return
  fi
  target="$(detect_target)"
  if [ -z "$version" ]; then
    version="$(curl -fsSL "${BASE_URL}/LATEST" | tr -d '[:space:]')"
  fi
  case "$version" in
    "" | */* | *..*)
      echo "Could not resolve a valid release to install." >&2
      exit 1
      ;;
  esac

  asset="rzv-${target}"
  work_dir="$(mktemp -d)"
  trap 'rm -rf "$work_dir"' EXIT

  echo "Downloading rzv ${version} for ${target} ..."
  curl -fsSL -o "${work_dir}/${asset}" "${BASE_URL}/${version}/${asset}"
  curl -fsSL -o "${work_dir}/${asset}.sha256" "${BASE_URL}/${version}/${asset}.sha256"

  expected="$(awk '{print $1}' "${work_dir}/${asset}.sha256")"
  actual="$(sha256_of "${work_dir}/${asset}")"
  if [ -z "$expected" ] || [ "$expected" != "$actual" ]; then
    echo "Checksum verification failed for ${asset}: expected ${expected:-nothing}, got ${actual}. Refusing to install." >&2
    exit 1
  fi
  echo "Checksum verified."

  mkdir -p "$INSTALL_DIR"
  chmod 755 "${work_dir}/${asset}"
  mv "${work_dir}/${asset}" "${INSTALL_DIR}/rzv"
  echo "Installed rzv ${version} to ${INSTALL_DIR}/rzv"
  case ":$PATH:" in
    *":${INSTALL_DIR}:"*) ;;
    *) echo "${INSTALL_DIR} is not on your PATH. Add it, for example: export PATH=\"${INSTALL_DIR}:\$PATH\"" ;;
  esac
}

main
