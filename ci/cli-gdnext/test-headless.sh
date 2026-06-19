#!/usr/bin/env bash
# Run `gdnext test` against the scratch project. It has no Go tests, so
# the run is a no-op success; the value is exercising the full setup-
# and-dispatch path of the test verb under godot --headless.
set -euo pipefail

scratch="${1:?usage: test-headless.sh <scratch_dir>}"
cd "$scratch"
gdnext test
