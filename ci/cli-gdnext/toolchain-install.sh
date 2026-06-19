#!/usr/bin/env bash
# Exercise `gdnext toolchain install` end-to-end:
#
#   1. The argless walk (`gdnext toolchain install`) must succeed and
#      log a result for every entry in the catalog, even if individual
#      entries are unreachable on this OS (ldd on macos/windows, etc.).
#   2. Each installable executable in the catalog must, after
#      `gdnext toolchain install <name>`, be:
#        - resolvable via `gdnext toolchain path <name>`,
#        - present on disk,
#        - reported `OK` by `gdnext toolchain doctor`.
#
# Tools intentionally skipped:
#   ldd               — system tool, no download
#   go                — gdnext is already running on it
#   libgodot / libgodot-editor / android.jar — IsLibrary, no executable
#
# This uses the runner's default $GDPATH (~/gd) so the populated cache
# carries through to the subsequent gdnext build steps (and is preserved
# week-over-week by the workflow's actions/cache step).
set -euo pipefail

# 1) Full walk must exit 0. The install verb logs "failed: ..." per
#    unreachable entry but should keep going and exit 0 itself.
echo "=== gdnext toolchain install (full walk) ==="
gdnext toolchain install

# 2) Per-tool assertions. Order matches the catalog in
#    cmd/gdnext/internal/cli/toolchain.go.
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

# upx and vpk are optional packagers that aren't required for any
# `gdnext build` step we run. Their download URLs occasionally drift
# upstream (e.g. upx ships .tar.xz on linux but the catalog template
# expects .zip). Treat them as best-effort: install attempts may
# fail, but a failure here doesn't fail the whole job.
optional_tools=(upx vpk)

for name in "${tools[@]}"; do
  echo
  echo "=== $name ==="
  gdnext toolchain install "$name"

  path=$(gdnext toolchain path "$name")
  test -e "$path" || { echo "$name: path $path does not exist"; exit 1; }

  # Doctor must report this entry OK now. This is the load-bearing
  # assertion — doctor invokes the toolchain's configured VersionFlags
  # under the hood, so an OK status proves the binary on disk actually
  # runs and reports a parseable version.
  doctor=$(gdnext toolchain doctor 2>&1 || true)
  echo "$doctor" | grep -qE "^[[:space:]]*${name}[[:space:]]+OK" \
    || { echo "doctor: $name not OK after install"; echo "$doctor"; exit 1; }
done

for name in "${optional_tools[@]}"; do
  echo
  echo "=== $name (best-effort) ==="
  if ! gdnext toolchain install "$name" 2>&1; then
    echo "(optional install for $name failed; continuing)"
  fi
done
