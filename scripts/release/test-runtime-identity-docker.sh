#!/bin/sh
set -eu
base=${1:?candidate image is required}
revision=${2:?revision is required}
version=${3:?version is required}
name="echo-noise-u1-bad-${GITHUB_RUN_ID:-local}-$$"
image="echo-noise-u1-identity-mismatch:local"
cleanup() { docker rm -f "$name" >/dev/null 2>&1 || true; docker image rm "$image" >/dev/null 2>&1 || true; }
trap cleanup EXIT
docker build --build-arg "BASE=$base" -f scripts/release/fixtures/identity-mismatch.Dockerfile -t "$image" scripts/release/fixtures
test "$(docker image inspect "$image" --format '{{ index .Config.Labels "org.opencontainers.image.revision" }}')" = "$revision"
docker run -d --name "$name" "$image"
for _ in $(seq 1 30); do
  [ "$(docker inspect "$name" --format '{{.State.Health.Status}}')" = healthy ] && break
  sleep 1
done
test "$(docker inspect "$name" --format '{{.State.Health.Status}}')" = healthy
echo 'Reproduced legacy acceptance: correct labels and healthy container, wrong executable identity.'
if SMOKE_LOCAL_IMAGE=1 sh scripts/release/smoke-image.sh "$image" "$revision" "$version"; then
  echo 'ERROR: mismatched executable identity was accepted' >&2
  exit 1
fi
echo 'Mismatched executable identity rejected before promotion.'
