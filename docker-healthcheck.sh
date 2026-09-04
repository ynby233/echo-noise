#!/bin/sh
set -eu

CONFIG_DIR="${ECHO_NOISE_CONFIG_DIR:-/app/config}"
env_file="${RUNTIME_ENV_FILE:-$CONFIG_DIR/runtime.env}"
config_file="$CONFIG_DIR/config.yaml"

# A Docker healthcheck runs in a new process and therefore cannot see variables
# exported by the entrypoint. Load the persisted runtime environment first;
# when it does not override HTTP_PORT, fall back to the server port used by the
# application's persisted YAML configuration.
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

port="${HTTP_PORT:-}"
if [ -z "$port" ] && [ -f "$config_file" ]; then
  port="$(awk '
    /^[[:space:]]*server:[[:space:]]*(#.*)?$/ {
      in_server = 1
      next
    }
    in_server && /^[^[:space:]]/ {
      exit
    }
    in_server && /^[[:space:]]*port:[[:space:]]*/ {
      value = $0
      sub(/^[[:space:]]*port:[[:space:]]*/, "", value)
      sub(/[[:space:]]*#.*/, "", value)
      gsub(/^[[:space:]\"]+|[[:space:]\"]+$/, "", value)
      print value
      exit
    }
  ' "$config_file")"
fi
port="${port:-1314}"
case "$port" in
  ''|*[!0-9]*) exit 1 ;;
esac

wget -q -T 3 -O /dev/null "http://127.0.0.1:$port/api/health/ready"
