#!/bin/bash
set -e
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
cd "$ROOT"
mkdir -p desktop/tauri/src-tauri/bin

copy_exec() {
  local src="$1"
  local dest="$2"
  [ -f "$dest" ] && chmod u+w "$dest" 2>/dev/null || true
  rm -f "$dest"
  cp "$src" "$dest"
  chmod +x "$dest" 2>/dev/null || true
}

case "$(uname -s)" in
  Darwin)
    ARCH=$(uname -m)
    GOARCH=$( [ "$ARCH" = "arm64" ] && echo arm64 || echo amd64 )
    CGO_ENABLED=0 GOOS=darwin GOARCH=$GOARCH go build -trimpath -ldflags "-s -w -buildid=" -o desktop/tauri/src-tauri/bin/server ./cmd/server/main.go
    if [ "$GOARCH" = "arm64" ]; then
      copy_exec desktop/tauri/src-tauri/bin/server desktop/tauri/src-tauri/bin/server-aarch64-apple-darwin
    else
      copy_exec desktop/tauri/src-tauri/bin/server desktop/tauri/src-tauri/bin/server-x86_64-apple-darwin
    fi

    if command -v ffmpeg >/dev/null 2>&1; then
      FF=$(command -v ffmpeg)
      copy_exec "$FF" desktop/tauri/src-tauri/bin/ffmpeg
      if [ "$GOARCH" = "arm64" ]; then
        copy_exec desktop/tauri/src-tauri/bin/ffmpeg desktop/tauri/src-tauri/bin/ffmpeg-aarch64-apple-darwin
      else
        copy_exec desktop/tauri/src-tauri/bin/ffmpeg desktop/tauri/src-tauri/bin/ffmpeg-x86_64-apple-darwin
      fi
    fi
    ;;
  Linux)
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w -buildid=" -o desktop/tauri/src-tauri/bin/server ./cmd/server/main.go
    copy_exec desktop/tauri/src-tauri/bin/server desktop/tauri/src-tauri/bin/server-x86_64-unknown-linux-gnu

    if command -v ffmpeg >/dev/null 2>&1; then
      FF=$(command -v ffmpeg)
      copy_exec "$FF" desktop/tauri/src-tauri/bin/ffmpeg
      copy_exec desktop/tauri/src-tauri/bin/ffmpeg desktop/tauri/src-tauri/bin/ffmpeg-x86_64-unknown-linux-gnu
    fi
    ;;
  MINGW*|MSYS*|CYGWIN*)
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -buildid=" -o desktop/tauri/src-tauri/bin/server.exe ./cmd/server/main.go
    copy_exec desktop/tauri/src-tauri/bin/server.exe desktop/tauri/src-tauri/bin/server-x86_64-pc-windows-msvc.exe

    if command -v ffmpeg >/dev/null 2>&1; then
      FF=$(command -v ffmpeg)
      copy_exec "$FF" desktop/tauri/src-tauri/bin/ffmpeg.exe
      copy_exec desktop/tauri/src-tauri/bin/ffmpeg.exe desktop/tauri/src-tauri/bin/ffmpeg-x86_64-pc-windows-msvc.exe
    fi
    ;;
esac
echo "sidecar built at desktop/tauri/src-tauri/bin/"
