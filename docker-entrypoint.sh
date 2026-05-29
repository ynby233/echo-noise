#!/bin/sh
set -eu

CONFIG_DIR="${ECHO_NOISE_CONFIG_DIR:-/app/config}"
DEFAULT_CONFIG_DIR="${ECHO_NOISE_DEFAULT_CONFIG_DIR:-/app/default-config}"

load_default_config() {
  if [ -f "$CONFIG_DIR/config.yaml" ]; then
    return 0
  fi
  if [ ! -d "$DEFAULT_CONFIG_DIR" ]; then
    return 0
  fi

  mkdir -p "$CONFIG_DIR"
  cp -a "$DEFAULT_CONFIG_DIR/." "$CONFIG_DIR/"
  echo "Initialized default config in $CONFIG_DIR"
}

load_runtime_env_example() {
  example_file="$CONFIG_DIR/runtime.env.example"
  default_example_file="$DEFAULT_CONFIG_DIR/runtime.env.example"
  if [ -f "$example_file" ]; then
    return 0
  fi
  if [ ! -f "$default_example_file" ]; then
    return 0
  fi

  mkdir -p "$CONFIG_DIR"
  cp "$default_example_file" "$example_file"
  echo "Initialized runtime env template in $example_file"
}

load_runtime_env() {
  env_file="${RUNTIME_ENV_FILE:-$CONFIG_DIR/runtime.env}"
  if [ ! -f "$env_file" ]; then
    return 0
  fi

  tmp_file="/tmp/echo-noise-runtime-env.$$"
  cleanup_runtime_env() {
    rm -f "$tmp_file"
  }
  trap cleanup_runtime_env EXIT HUP INT TERM

  rm -f "$tmp_file"
  tr -d '\r' < "$env_file" > "$tmp_file"

  set +u
  set -a
  # shellcheck disable=SC1090
  . "$tmp_file"
  set +a
  set -u

  rm -f "$tmp_file"
  trap - EXIT HUP INT TERM
  echo "Loaded runtime environment from $env_file"
}

load_default_config
load_runtime_env_example
load_runtime_env
exec "$@"
