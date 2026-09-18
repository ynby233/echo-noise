#!/bin/sh
set -eu

current=${1:-}
target=${2:-}
current_version=${3:-}
target_version=${4:-}
hex40='^[0-9a-fA-F]{40}$'

if ! printf '%s\n' "$target" | grep -Eq "$hex40"; then
  echo "target revision must be a full Git SHA" >&2
  exit 1
fi
if [ -z "$current" ]; then
  echo advance
  exit 0
fi
if ! printf '%s\n' "$current" | grep -Eq "$hex40"; then
  echo "current channel revision is missing or invalid" >&2
  exit 1
fi
if [ "$current" = "$target" ] && [ "$current_version" = "$target_version" ]; then
  echo keep
elif git merge-base --is-ancestor "$current" "$target"; then
  if printf '%s\n%s\n' "$current_version" "$target_version" | grep -Eqv '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
    echo advance
  elif ! awk -v current="$current_version" -v target="$target_version" '
      function valid(v) { return v ~ /^v[0-9]+\.[0-9]+\.[0-9]+$/ }
      BEGIN {
        if (!valid(current) || !valid(target)) exit 2
        sub(/^v/, "", current); sub(/^v/, "", target)
        split(current, c, "."); split(target, t, ".")
        for (i = 1; i <= 3; i++) {
          if (t[i] > c[i]) exit 0
          if (t[i] < c[i]) exit 1
        }
        exit 0
      }
    '; then
    echo keep
  else
    echo advance
  fi
elif git merge-base --is-ancestor "$target" "$current"; then
  echo keep
else
  echo "channel and target revisions have diverged" >&2
  exit 1
fi
