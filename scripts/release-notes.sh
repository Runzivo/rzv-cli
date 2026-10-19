#!/usr/bin/env bash
# Prints the GitHub release notes for <tag>: its CHANGELOG.md section, the
# install command and the checksum table from <release-dir>/SHA256SUMS.
# Fails when CHANGELOG.md has no "## <tag> " section.
#   bash scripts/release-notes.sh <tag> <release-dir>
set -euo pipefail

tag="${1:?usage: release-notes.sh <tag> <release-dir>}"
dist="${2:?usage: release-notes.sh <tag> <release-dir>}"
root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"

section="$(awk -v tag="$tag" '
  index($0, "## " tag " ") == 1 { found = 1; next }
  found && /^## / { exit }
  found { print }
' "${root}/CHANGELOG.md")"
if [ -z "$(printf '%s' "$section" | tr -d '[:space:]')" ]; then
  echo "release-notes: CHANGELOG.md has no section starting with '## ${tag} '" >&2
  exit 1
fi

printf '%s\n\n' "$(printf '%s\n' "$section" | sed -e '/./,$!d')"
# shellcheck disable=SC2016 # backticks are Markdown
printf 'Install: `curl -fsSL https://runzivo.dev/downloads/rzv/install.sh | sh`\n\n'
printf '| File | SHA-256 |\n| --- | --- |\n'
awk '{ printf "| %s | `%s` |\n", $2, $1 }' "${dist}/SHA256SUMS"
