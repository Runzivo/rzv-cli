#!/usr/bin/env bash
# Creates a DRAFT GitHub release for <tag> with every file in <release-dir>
# and the notes from scripts/release-notes.sh. A person reviews and
# publishes it; nothing here makes a release public. Uses curl and jq only.
#   GITHUB_TOKEN=... GITHUB_REPOSITORY=Runzivo/rzv-cli \
#     bash scripts/publish-draft.sh <tag> <release-dir>
set -euo pipefail

tag="${1:?usage: publish-draft.sh <tag> <release-dir>}"
dist="${2:?usage: publish-draft.sh <tag> <release-dir>}"
repo="${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
: "${GITHUB_TOKEN:?GITHUB_TOKEN is required}"
api="${GITHUB_API_URL:-https://api.github.com}"
root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"

gh_api() {
  curl -fsS -H "Authorization: Bearer ${GITHUB_TOKEN}" -H "Accept: application/vnd.github+json" \
    -H "X-GitHub-Api-Version: 2022-11-28" "$@"
}

existing="$(gh_api "${api}/repos/${repo}/releases?per_page=100" | jq -r --arg t "$tag" '.[] | select(.tag_name == $t) | .id')"
if [ -n "$existing" ]; then
  echo "publish-draft: ${repo} already has a release for ${tag}; review or delete it first" >&2
  exit 1
fi

notes="$(bash "${root}/scripts/release-notes.sh" "$tag" "$dist")"
version="$(tr -d '[:space:]' <"${root}/VERSION")"
payload="$(jq -n --arg tag "$tag" --arg name "${tag} (rzv ${version})" --arg body "$notes" \
  '{tag_name: $tag, name: $name, body: $body, draft: true, prerelease: false}')"
release="$(gh_api -X POST "${api}/repos/${repo}/releases" -d "$payload")"
upload_url="$(jq -r '.upload_url | sub("\\{.*\\}$"; "")' <<<"$release")"
html_url="$(jq -r '.html_url' <<<"$release")"

for file in "$dist"/*; do
  name="$(basename "$file")"
  echo "Uploading ${name}" >&2
  gh_api -X POST -H "Content-Type: application/octet-stream" --data-binary "@${file}" \
    "${upload_url}?name=${name}" >/dev/null
done

echo "Draft ready: ${html_url}"
echo "Review it, then publish: gh release edit ${tag} --repo ${repo} --draft=false --latest"
