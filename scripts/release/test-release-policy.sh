#!/bin/sh
set -eu

root="$(cd "$(dirname "$0")/../.." && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

git -C "$work" init -q -b test-root
git -C "$work" config user.name test
git -C "$work" config user.email test@example.com
echo one > "$work/file"
git -C "$work" add file
git -C "$work" commit -qm one
first="$(git -C "$work" rev-parse HEAD)"
git -C "$work" tag -a v1.2.3 -m release "$first"
echo two >> "$work/file"
git -C "$work" commit -qam two
second="$(git -C "$work" rev-parse HEAD)"
git -C "$work" branch main
git -C "$work" switch -qc divergent "$first"
echo other > "$work/other"
git -C "$work" add other
git -C "$work" commit -qm other
other="$(git -C "$work" rev-parse HEAD)"

test "$(git -C "$work" rev-parse 'v1.2.3^{commit}')" = "$first"
resolved="$(cd "$work" && sh "$root/scripts/release/resolve-release.sh" v1.2.3 main)"
printf '%s\n' "$resolved" | grep -qx "VERSION=v1.2.3"
printf '%s\n' "$resolved" | grep -qx "REVISION=$first"

test "$(cd "$work" && sh "$root/scripts/release/channel-policy.sh" '' "$second")" = advance
test "$(cd "$work" && sh "$root/scripts/release/channel-policy.sh" "$first" "$second")" = advance
test "$(cd "$work" && sh "$root/scripts/release/channel-policy.sh" "$first" "$second" "$(printf %.12s "$first")" "$(printf %.12s "$second")")" = advance
test "$(cd "$work" && sh "$root/scripts/release/channel-policy.sh" "$first" "$second" v1.2.3 v1.3.0)" = advance
test "$(cd "$work" && sh "$root/scripts/release/channel-policy.sh" "$first" "$second" v2.0.0 v1.9.0)" = keep
test "$(cd "$work" && sh "$root/scripts/release/channel-policy.sh" "$second" "$first")" = keep
test "$(cd "$work" && sh "$root/scripts/release/channel-policy.sh" "$second" "$second")" = keep
test "$(cd "$work" && sh "$root/scripts/release/channel-policy.sh" "$second" "$second" v1.2.3 v1.2.4)" = advance
test "$(cd "$work" && sh "$root/scripts/release/channel-policy.sh" "$second" "$second" v1.2.4 v1.2.3)" = keep
if (cd "$work" && sh "$root/scripts/release/channel-policy.sh" "$second" "$other"); then
  echo "diverged histories must be rejected" >&2
  exit 1
fi

mkdir "$work/bin"
printf '%s\n' '#!/bin/sh' 'case "$FAKE_MODE" in' \
  'exists) printf '\''%s\n'\'' '\''{"annotations":{"org.opencontainers.image.revision":"1111111111111111111111111111111111111111","org.opencontainers.image.version":"v1.2.3"}}'\'' ;;' \
  'missing) echo "manifest unknown" >&2; exit 1 ;;' \
  '*) echo "network timeout" >&2; exit 1 ;;' \
  'esac' > "$work/bin/docker"
metadata="$(FAKE_MODE=exists PATH="$work/bin:$PATH" sh "$root/scripts/release/image-metadata.sh" example)"
printf '%s\n' "$metadata" | grep -qx 'STATE=exists'
printf '%s\n' "$metadata" | grep -qx 'REVISION=1111111111111111111111111111111111111111'
test "$(FAKE_MODE=missing PATH="$work/bin:$PATH" sh "$root/scripts/release/image-metadata.sh" example)" = 'STATE=missing'
if FAKE_MODE=failure PATH="$work/bin:$PATH" sh "$root/scripts/release/image-metadata.sh" example >/dev/null 2>&1; then
  echo "registry failures must not be treated as missing tags" >&2
  exit 1
fi

echo "release policy tests passed"
