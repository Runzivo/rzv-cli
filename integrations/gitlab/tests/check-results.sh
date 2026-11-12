#!/bin/sh
set -eu

if [ "$#" -gt 1 ]; then
  echo "Usage: check-results.sh [artifact-directory]" >&2
  exit 2
fi

artifacts=${1:-.catalog-smoke}

if [ ! -f "$artifacts/default.txt" ] || [ ! -f "$artifacts/custom.txt" ]; then
  echo "Both component jobs must provide their result artifacts." >&2
  exit 1
fi

if [ "$(cat "$artifacts/default.txt")" != "default job passed" ] ||
  [ "$(cat "$artifacts/custom.txt")" != "custom job passed" ]; then
  echo "A component result artifact has unexpected contents." >&2
  exit 1
fi

echo "Both component job results verified."
