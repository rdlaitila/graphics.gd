#!/usr/bin/env bash
# Stage examples/<name>/ into a tempdir so a fresh `gdnext build` runs against
# a self-contained copy with `graphics.gd` redirected to the current checkout.
# Replaces scratch-project.sh — every CI build verb operates on the staged dir.
#
# Usage: stage-example.sh <name> <empty_target_dir>
#
# Inputs:
#   GRAPHICS_GD_ROOT (preferred) or GITHUB_WORKSPACE — path to the graphics.gd
#   checkout that should be substituted for the `require graphics.gd` line in
#   the example's go.mod.
set -euo pipefail

name="${1:?usage: stage-example.sh <name> <empty_target_dir>}"
target="${2:?usage: stage-example.sh <name> <empty_target_dir>}"
graphics_gd_root="${GRAPHICS_GD_ROOT:-${GITHUB_WORKSPACE:-}}"

if [ -z "$graphics_gd_root" ]; then
  echo "set GRAPHICS_GD_ROOT or GITHUB_WORKSPACE to the graphics.gd checkout" >&2
  exit 1
fi
src="$graphics_gd_root/examples/$name"
if [ ! -d "$src" ]; then
  echo "no such example: $src" >&2
  exit 1
fi
if [ ! -f "$src/go.mod" ] || [ ! -f "$src/graphics/project.godot" ]; then
  echo "$src is not a valid example (missing go.mod or graphics/project.godot)" >&2
  exit 1
fi

# Copy the example into the tempdir. Trailing slash on $src ensures we get
# its *contents* rather than the directory itself.
cp -R "$src"/. "$target"/

cd "$target"
# Overwrite any committed `replace graphics.gd => ../..` so the staged copy
# resolves against the actual checkout root, not its old relative path.
go mod edit -replace=graphics.gd="$graphics_gd_root"
go mod tidy

# Sanity-check that the canonical fixtures landed; the build steps assume
# they're there. If a future example forgets one, fail loud.
test -f graphics/project.godot
test -f graphics/main.tscn
