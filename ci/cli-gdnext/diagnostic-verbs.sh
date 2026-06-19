#!/usr/bin/env bash
# Verify the diagnostic verbs that must work without any toolchain
# installed: `version`, `toolchain list` (catalog completeness), and
# `toolchain doctor` without --fix (no-download invariant).
#
# Run before `toolchain-install.sh` so the no-download check is meaningful.
set -euo pipefail

gdnext version

# `toolchain list` must print every entry in the catalog. If you add or
# rename one in cmd/gdnext/internal/cli/toolchain.go, update this list.
listing=$(gdnext toolchain list)
echo "$listing"
for tool in godot go zig llvm adb apksigner aapt2 apktool \
            bundletool android.jar upx vpk libgodot \
            libgodot-editor ldd; do
  echo "$listing" | grep -qE "^[[:space:]]*$tool([[:space:]]|$)" \
    || { echo "toolchain list missing entry: $tool"; exit 1; }
done

# `doctor` without --fix uses tooling.ModeFind and must not download.
# Capture $GDPATH size before/after to prove it.
gdpath="${GDPATH:-$HOME/gd}"
before=$(du -s "$gdpath" 2>/dev/null | awk '{print $1}' || echo 0)
gdnext toolchain doctor || true   # nonzero is expected when tools are missing
after=$(du -s "$gdpath" 2>/dev/null | awk '{print $1}' || echo 0)
test "$before" = "$after" || { echo "doctor mutated $gdpath"; exit 1; }
