#!/bin/sh
set -eu

tag=${1:-}
main_ref=${2:-origin/main}

if ! printf '%s\n' "$tag" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
  echo "release tag must use vX.Y.Z" >&2
  exit 1
fi

revision=$(git rev-parse --verify "$tag^{commit}")
if ! git merge-base --is-ancestor "$revision" "$main_ref"; then
  echo "release tag $tag does not belong to $main_ref" >&2
  exit 1
fi

printf 'VERSION=%s\nREVISION=%s\n' "$tag" "$revision"
