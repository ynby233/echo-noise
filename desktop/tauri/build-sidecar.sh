#!/bin/bash
set -e
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
cd "$ROOT"
mkdir -p desktop/tauri/src-tauri/bin
case "$(uname -s)" in
  Darwin)
    ARCH=$(uname -m)
    GOARCH=$( [ "$ARCH" = "arm64" ] && echo arm64 || echo amd64 )
    CGO_ENABLED=0 GOOS=darwin GOARCH=$GOARCH go build -trimpath -ldflags "-s -w -buildid=" -o desktop/tauri/src-tauri/bin/server ./cmd/server/main.go
    if [ "$GOARCH" = "arm64" ]; then
      cp desktop/tauri/src-tauri/bin/server desktop/tauri/src-tauri/bin/server-aarch64-apple-darwin
    else
      cp desktop/tauri/src-tauri/bin/server desktop/tauri/src-tauri/bin/server-x86_64-apple-darwin
    fi
    ;;
  Linux)
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w -buildid=" -o desktop/tauri/src-tauri/bin/server ./cmd/server/main.go
    cp desktop/tauri/src-tauri/bin/server desktop/tauri/src-tauri/bin/server-x86_64-unknown-linux-gnu
    ;;
  MINGW*|MSYS*|CYGWIN*)
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -buildid=" -o desktop/tauri/src-tauri/bin/server.exe ./cmd/server/main.go
    cp desktop/tauri/src-tauri/bin/server.exe desktop/tauri/src-tauri/bin/server-x86_64-pc-windows-msvc.exe
    ;;
esac
echo "sidecar built at desktop/tauri/src-tauri/bin/"
