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
# Redirect stdin away from the parent shell so the optional AAB-signing
# `Provide passphrase:` prompt in `gdnext build` reads EOF immediately
# and the build returns cleanly. Without this, term.ReadPassword can
# consume the next bash script line as the passphrase.
gdnext -goos "$goos" -goarch "$goarch" build < /dev/null

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

# Android-specific: `gdnext build` exports an unsigned apk (it stubs out
# Godot's jarsigner step). Sign it with the debug keystore that gdnext
# already provisioned via the cryptic generator, then verify. This proves
# apksigner + the keystore + the toolchain path resolver all line up
# end-to-end.
#
# apksigner is positional-last: pass --ks/--ks-pass/--ks-key-alias first,
# then the apk path. The opposite order trips apksigner's parser into
# emitting "At least one signer must be specified".
if [ "$goos" = "android" ]; then
  apk=$(ls "releases/android/${goarch}/"*.apk 2>/dev/null | head -1)
  test -n "$apk" || { echo "android build produced no apk" >&2; exit 1; }
  keystore=$(gdnext android keystore show)
  test -f "$keystore" || { echo "expected keystore at $keystore but it's missing"; exit 1; }
  gdnext android apk sign \
    --ks "$keystore" --ks-pass pass:android --ks-key-alias androiddebugkey \
    "$apk"
  gdnext android apk verify "$apk"
fi
