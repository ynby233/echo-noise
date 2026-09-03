#!/bin/sh
set -eu

CONFIG_DIR="${ECHO_NOISE_CONFIG_DIR:-/app/config}"
env_file="${RUNTIME_ENV_FILE:-$CONFIG_DIR/runtime.env}"

# A Docker healthcheck runs in a new process and therefore cannot see variables
# exported by the entrypoint. Load the same persisted runtime environment so a
# GUI-configured non-default HTTP port is checked correctly.
if [ -f "$env_file" ]; then
  tmp_file="/tmp/echo-noise-health-env.$$"
  cleanup_health_env() {
    rm -f "$tmp_file"
  }
  trap cleanup_health_env EXIT HUP INT TERM
  tr -d '\r' < "$env_file" > "$tmp_file"
  set +u
  set -a
  # shellcheck disable=SC1090
  . "$tmp_file"
  set +a
  set -u
  rm -f "$tmp_file"
  trap - EXIT HUP INT TERM
fi

port="${HTTP_PORT:-1314}"
case "$port" in
  ''|*[!0-9]*) exit 1 ;;
esac

wget -q -T 3 -O /dev/null "http://127.0.0.1:$port/api/health/ready"
