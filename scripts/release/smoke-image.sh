#!/bin/sh
set -eu

ref=${1:?image reference is required}
revision=${2:?expected revision is required}
version=${3:?expected version is required}
expected_time=${4:-}
work=$(mktemp -d)
name="echo-noise-smoke-${GITHUB_RUN_ID:-local}-${GITHUB_RUN_ATTEMPT:-1}-$$"
cleanup() { docker rm -f "$name" "$name-identity" >/dev/null 2>&1 || true; rm -rf "$work"; }
trap cleanup EXIT

if [ "${SMOKE_LOCAL_IMAGE:-0}" != 1 ]; then docker pull "$ref"; fi
image_id=$(docker image inspect "$ref" --format '{{.Id}}')
docker image inspect "$image_id" --format '{{json .Config.Labels}}' > "$work/labels.json"
# Bypass entrypoint/runtime.env: only executable-embedded identity is accepted.
timeout 20s docker run --rm --name "$name-identity" --entrypoint /app/noise "$image_id" --build-info > "$work/runtime.json"
node -e '
  const fs = require("node:fs")
  const [directory, revision, version, expectedTime] = process.argv.slice(1)
  const labels = JSON.parse(fs.readFileSync(directory + "/labels.json", "utf8"))
  const runtime = JSON.parse(fs.readFileSync(directory + "/runtime.json", "utf8"))
  const builtAt = labels["org.opencontainers.image.created"]
  if (!/^[0-9a-f]{40}$/.test(revision) || !builtAt || Number.isNaN(Date.parse(builtAt)) ||
      (expectedTime && builtAt !== expectedTime) ||
      labels["org.opencontainers.image.revision"] !== revision ||
      labels["org.opencontainers.image.version"] !== version ||
      runtime.revision !== revision || runtime.version !== version ||
      runtime.identity !== version || runtime.built_at !== builtAt) {
    throw Error("image labels and executable build identity do not match expected target")
  }
  console.log("Verified executable identity: " + JSON.stringify(runtime))
' "$work" "$revision" "$version" "$expected_time"

docker run -d --name "$name" -e ACCESS_LOG=false "$image_id"
for _ in $(seq 1 60); do
  status=$(docker inspect "$name" --format '{{.State.Health.Status}}')
  [ "$status" = healthy ] && exit 0
  [ "$status" = unhealthy ] && break
  sleep 2
done
docker logs "$name"
exit 1
