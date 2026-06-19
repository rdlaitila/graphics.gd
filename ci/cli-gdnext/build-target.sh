#!/usr/bin/env bash
# Run `gdnext build` for one GOOS/GOARCH pair against a scratch project
# and assert the produced shared library + distributable land on disk.
# The android branch additionally exercises apksigner via the toolchain
# wrapper, which transitively proves the cryptic keystore + adb +
# apksigner + aapt2 + apktool + bundletool + android.jar pipeline.
#
# Usage: build-target.sh <goos> <goarch> <scratch_dir>
set -euo pipefail

goos="${1:?usage: build-target.sh <goos> <goarch> <scratch_dir>}"
goarch="${2:?usage: build-target.sh <goos> <goarch> <scratch_dir>}"
scratch="${3:?usage: build-target.sh <goos> <goarch> <scratch_dir>}"

cd "$scratch"
gdnext -goos "$goos" -goarch "$goarch" build

# Shared library next to the project files. macOS BuildMain produces both
# the arch-specific dylib and a universal lipo'd one — assert both so a
# regression in the lipo step is caught.
case "$goos" in
  linux|musl)
    test -f "graphics/linux_${goarch}.so"
    ;;
  windows)
    test -f "graphics/windows_${goarch}.dll"
    ;;
  darwin)
    test -f "graphics/darwin_${goarch}.dylib"
    test -f "graphics/darwin_universal.dylib"
    ;;
  js)
    test -f "releases/js/wasm/library.wasm"
    ;;
  android)
    : # apk + signature checked below
    ;;
  *)
    echo "unsupported goos: $goos" >&2
    exit 1
    ;;
esac

# Distributable bundle. Each builder writes to a fixed location driven by
# the Godot export preset; assert it landed on disk. wasm has no separate
# distributable (everything lives under releases/js/wasm/ already), so
# the wasm shared-library check above is enough.
case "$goos" in
  linux|windows|android)
    ls "releases/$goos/$goarch/"
    ;;
  darwin)
    # macOS exports a universal .app regardless of -goarch.
    ls releases/darwin/universal/
    ;;
  musl)
    ls releases/musl/$goarch/
    ;;
esac

# Android-specific: `gdnext build` exports an unsigned apk (it bypasses
# Godot's jarsigner by writing a stub `java` binary). Sign it with the
# debug keystore that `gdnext build` already provisioned via the cryptic
# generator, then verify. This proves apksigner + the keystore + the
# toolchain path resolver all line up end-to-end.
if [ "$goos" = "android" ]; then
  apk=$(ls "releases/android/${goarch}/"*.apk 2>/dev/null | head -1)
  test -n "$apk" || { echo "android build produced no apk" >&2; exit 1; }
  keystore=$(gdnext android keystore show)
  test -f "$keystore" || { echo "expected keystore at $keystore but it's missing"; exit 1; }
  gdnext android apk sign "$apk" \
    --ks "$keystore" --ks-pass pass:android --ks-key-alias androiddebugkey
  gdnext android apk verify "$apk"
fi
