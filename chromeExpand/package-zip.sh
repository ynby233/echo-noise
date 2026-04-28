#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MANIFEST_PATH="$SCRIPT_DIR/manifest.json"
DIST_DIR="$SCRIPT_DIR/dist"

if [ ! -f "$MANIFEST_PATH" ]; then
  echo "ERROR: manifest.json not found in $SCRIPT_DIR"
  exit 1
fi

VERSION="$(grep -oE '"version"[[:space:]]*:[[:space:]]*"[^"]+"' "$MANIFEST_PATH" | head -n1 | sed -E 's/.*"([^"]+)".*/\1/')"
if [ -z "$VERSION" ]; then
  VERSION="0.0.0"
fi

ZIP_NAME="saynote-browser-extension-v${VERSION}.zip"
ZIP_PATH="$DIST_DIR/$ZIP_NAME"

mkdir -p "$DIST_DIR"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

# Copy extension source to a temp directory, excluding non-distribution files.
rsync -a \
  --exclude "dist" \
  --exclude ".DS_Store" \
  --exclude "*.map" \
  --exclude ".git" \
  "$SCRIPT_DIR/" "$TMP_DIR/chromeExpand/"

(cd "$TMP_DIR/chromeExpand" && zip -qr "$ZIP_PATH" .)

echo "Extension package created:"
echo "$ZIP_PATH"
