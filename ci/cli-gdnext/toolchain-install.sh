#!/usr/bin/env bash
# Install the two toolchains every build target needs (zig + godot)
# and verify both `toolchain path` and `toolchain doctor` recognise
# them once they're on disk.
set -euo pipefail

gdnext toolchain install zig
gdnext toolchain install godot

# `toolchain path` must resolve to a real executable on disk.
zigpath=$(gdnext toolchain path zig)
test -x "$zigpath" || { echo "toolchain path zig: $zigpath not executable"; exit 1; }
"$zigpath" version

godotpath=$(gdnext toolchain path godot)
test -e "$godotpath" || { echo "toolchain path godot: $godotpath missing"; exit 1; }

# Doctor (still no --fix) must now report zig + godot OK since they're
# cached. This is the post-install diagnostic.
doctor=$(gdnext toolchain doctor 2>&1 || true)
echo "$doctor"
echo "$doctor" | grep -qE "^[[:space:]]*zig[[:space:]]+OK" \
  || { echo "doctor: zig not OK after install"; exit 1; }
echo "$doctor" | grep -qE "^[[:space:]]*godot[[:space:]]+OK" \
  || { echo "doctor: godot not OK after install"; exit 1; }
