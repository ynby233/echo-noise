#!/bin/sh
if [ "${1:-}" = --build-info ]; then
  echo '{"identity":"v0.0.0","version":"v0.0.0","revision":"1111111111111111111111111111111111111111","built_at":"2000-01-01T00:00:00Z"}'
  exit 0
fi
exec sleep 3600
