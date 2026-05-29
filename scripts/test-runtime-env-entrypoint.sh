#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
TMP_DIR=$(mktemp -d)
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT INT TERM

ENV_FILE="$TMP_DIR/runtime.env"
cat > "$ENV_FILE" <<'EOF_ENV'
# shell-style runtime env file
TZ=Asia/Shanghai
ACCESS_LOG=false
export NOTE_HOST=http://localhost:1314
NOTE_HTTP_PORT=0
SESSION_SECRET="0123456789abcdef0123456789abcdef"
EOF_ENV

RUNTIME_ENV_FILE="$ENV_FILE" sh "$ROOT_DIR/docker-entrypoint.sh" sh -ec '
  test "$TZ" = "Asia/Shanghai"
  test "$ACCESS_LOG" = "false"
  test "$NOTE_HOST" = "http://localhost:1314"
  test "$NOTE_HTTP_PORT" = "0"
  test "$SESSION_SECRET" = "0123456789abcdef0123456789abcdef"
'

CRLF_ENV_FILE="$TMP_DIR/runtime-crlf.env"
printf 'ACCESS_LOG=true\r\nNOTE_HTTP_PORT=1315\r\n' > "$CRLF_ENV_FILE"
RUNTIME_ENV_FILE="$CRLF_ENV_FILE" sh "$ROOT_DIR/docker-entrypoint.sh" sh -ec '
  test "$ACCESS_LOG" = "true"
  test "$NOTE_HTTP_PORT" = "1315"
'

CONFIG_DIR="$TMP_DIR/config"
DEFAULT_CONFIG_DIR="$TMP_DIR/default-config"
mkdir -p "$CONFIG_DIR" "$DEFAULT_CONFIG_DIR"
printf 'server:\n  port: "1314"\n' > "$DEFAULT_CONFIG_DIR/config.yaml"
printf '# runtime env template\n# ACCESS_LOG=false\n' > "$DEFAULT_CONFIG_DIR/runtime.env.example"
printf 'ACCESS_LOG=from_config_dir\n' > "$CONFIG_DIR/runtime.env"
ECHO_NOISE_CONFIG_DIR="$CONFIG_DIR" \
ECHO_NOISE_DEFAULT_CONFIG_DIR="$DEFAULT_CONFIG_DIR" \
sh "$ROOT_DIR/docker-entrypoint.sh" sh -ec '
  test -f "$ECHO_NOISE_CONFIG_DIR/config.yaml"
  test -f "$ECHO_NOISE_CONFIG_DIR/runtime.env.example"
  test "$ACCESS_LOG" = "from_config_dir"
'

EXISTING_CONFIG_DIR="$TMP_DIR/existing-config"
mkdir -p "$EXISTING_CONFIG_DIR"
printf 'server:\n  port: "1314"\n' > "$EXISTING_CONFIG_DIR/config.yaml"
ECHO_NOISE_CONFIG_DIR="$EXISTING_CONFIG_DIR" \
ECHO_NOISE_DEFAULT_CONFIG_DIR="$DEFAULT_CONFIG_DIR" \
sh "$ROOT_DIR/docker-entrypoint.sh" sh -ec '
  test -f "$ECHO_NOISE_CONFIG_DIR/runtime.env.example"
  test ! -f "$ECHO_NOISE_CONFIG_DIR/runtime.env"
'

RUNTIME_ENV_FILE="$TMP_DIR/missing.env" sh "$ROOT_DIR/docker-entrypoint.sh" sh -ec '
  test -z "${ACCESS_LOG:-}"
'

echo "runtime env entrypoint test passed"
