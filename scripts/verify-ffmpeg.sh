#!/usr/bin/env sh
set -eu

IMAGE_TAG=${1:-echo-noise:ffmpeg-verify}
TARGET=${TARGET:-final-ffmpeg}
INSTALL_FFMPEG=${INSTALL_FFMPEG:-1}
FFMPEG_MODE=${FFMPEG_MODE:-custom}

if ! command -v docker >/dev/null 2>&1; then
  echo "docker not found" >&2
  exit 127
fi

if docker buildx version >/dev/null 2>&1; then
  docker buildx build --load -t "$IMAGE_TAG" \
    --target "$TARGET" \
    --build-arg INSTALL_FFMPEG="$INSTALL_FFMPEG" \
    --build-arg FFMPEG_MODE="$FFMPEG_MODE" \
    .
else
  docker build -t "$IMAGE_TAG" \
    --target "$TARGET" \
    --build-arg INSTALL_FFMPEG="$INSTALL_FFMPEG" \
    --build-arg FFMPEG_MODE="$FFMPEG_MODE" \
    .
fi

docker run --rm "$IMAGE_TAG" sh -lc '
set -eu
ffmpeg -version >/dev/null
ffmpeg -encoders | grep -E "libx264" >/dev/null
ffmpeg -encoders | grep -E "\baac\b" >/dev/null
ffmpeg -hide_banner -y \
  -f lavfi -i testsrc=size=1280x720:rate=30 \
  -f lavfi -i sine=frequency=1000:sample_rate=44100 \
  -t 2 \
  -vcodec libx264 -crf 28 -preset fast \
  -acodec aac -b:a 128k \
  -movflags +faststart \
  /tmp/out.mp4 >/dev/null 2>&1
test -s /tmp/out.mp4
'

echo "ffmpeg verify OK: $IMAGE_TAG"
