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
    ;;
  Linux)
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w -buildid=" -o desktop/tauri/src-tauri/bin/server ./cmd/server/main.go
    ;;
  MINGW*|MSYS*|CYGWIN*)
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -buildid=" -o desktop/tauri/src-tauri/bin/server.exe ./cmd/server/main.go
    ;;
esac
echo "sidecar built at desktop/tauri/src-tauri/bin/"
