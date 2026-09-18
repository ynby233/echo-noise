#!/bin/sh
set -eu

ref=${1:?image reference is required}
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

if docker buildx imagetools inspect --raw "$ref" >"$work/manifest.json" 2>"$work/error"; then
  node -e '
    const fs = require("node:fs")
    const annotations = JSON.parse(fs.readFileSync(process.argv[1], "utf8")).annotations || {}
    console.log("STATE=exists")
    console.log("REVISION=" + (annotations["org.opencontainers.image.revision"] || ""))
    console.log("VERSION=" + (annotations["org.opencontainers.image.version"] || ""))
  ' "$work/manifest.json"
elif grep -Eqi '(^|: )not found$|manifest unknown|name_unknown|manifest_unknown' "$work/error"; then
  echo STATE=missing
else
  cat "$work/error" >&2
  exit 1
fi
