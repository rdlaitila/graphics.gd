#!/usr/bin/env bash
# Reproduce the CI libgodot play-cell locally end-to-end. Useful for
# the GDNEXT_PLAY_RESULT env-loss diagnostic — drop probe / dump
# changes in playenv.go, re-run, read stdout. Not committed CI glue;
# scratch is recreated every run.
#
# scratch MUST live outside the example source tree: gdnext ci
# build-stage-example recursively copies the source dir, so scratch
# inside it produces infinite nesting (file name too long).
set -euo pipefail
cd "$(dirname "$0")"

TMP="${TMP:-${TMPDIR:-/tmp}/canarybird-libgodot}"
EXAMPLE=canarybird
TARGET=linux/amd64
LINK=libgodot
SHOT="$TMP/play-shot.png"

rm -rf "$TMP"
mkdir -p "$TMP"

go install ../../cmd/gdnext

echo "==> stage -> $TMP"
gdnext ci build-stage-example --example "$EXAMPLE" --scratch "$TMP"
echo "==> build $TARGET [$LINK]"
gdnext ci build-target --goos linux --goarch amd64 --link "$LINK" --scratch "$TMP"
echo "==> play"
PLAY_ARGS=(--scratch "$TMP" --example "$EXAMPLE" --target "$TARGET" --link "$LINK" --build-host local --screenshot "$SHOT")
if [[ -n "${NO_XVFB:-}" ]]; then
  PLAY_ARGS+=(--no-xvfb)
else
  unset DISPLAY  # force withXvfb wrapper in play-cell on Wayland hosts
fi
gdnext ci play-cell "${PLAY_ARGS[@]}"
