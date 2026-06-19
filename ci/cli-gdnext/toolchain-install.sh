#!/usr/bin/env bash
# Smoke-test `gdnext toolchain install` itself:
#
#   1. The argless walk (`gdnext toolchain install`) logs a result for
#      every entry and exits 0 even when individual entries are
#      unreachable (ldd on macos/windows, libgodot 404s, etc.).
#   2. `gdnext toolchain install <name>` followed by
#      `gdnext toolchain path <name>` round-trips to an on-disk file for
#      every installable executable.
#
# Per-target REQUIRED/OPTIONAL checks happen later inside
# build-target.sh via `GOOS=... GOARCH=... gdnext toolchain doctor
# --fix`, so this script intentionally does NOT call doctor — it stays
# focused on the install verb's own contract.
#
# Tools intentionally skipped:
#   ldd      — system tool, never downloaded
#   go       — gdnext is already running on it
#   libgodot / libgodot-editor / android.jar — IsLibrary, no executable
#   upx, vpk — optional packagers with flaky upstream URLs (best-effort
#              attempted but failure is tolerated)
set -euo pipefail

echo "=== gdnext toolchain install (full walk) ==="
gdnext toolchain install

tools=(
  godot
  zig
  llvm
  adb
  apksigner
  aapt2
  apktool
  bundletool
)
optional_tools=(upx vpk)

for name in "${tools[@]}"; do
  echo
  echo "=== $name ==="
  gdnext toolchain install "$name"
  path=$(gdnext toolchain path "$name")
  test -e "$path" || { echo "$name: path $path does not exist"; exit 1; }
done

for name in "${optional_tools[@]}"; do
  echo
  echo "=== $name (best-effort) ==="
  if ! gdnext toolchain install "$name" 2>&1; then
    echo "(optional install for $name failed; continuing)"
  fi
done
