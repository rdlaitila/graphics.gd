#!/usr/bin/env bash
# Probe whether GDNEXT_PLAY_* env vars survive when passed through the
# exact env-construction code path play-cell uses. Bypasses any
# graphics.gd / libgodot init by exec'ing /usr/bin/env, which just
# prints its environ and exits.
set -euo pipefail

# Build the same c.Env shape play_cell.go does, then exec the probe
# under it. If env shows all four, the env hand-off is fine and the
# loss is happening inside libgodot init (not in Go's exec / xvfb-run).
TMP="${TMPDIR:-/tmp}"
export GDNEXT_PLAY_RESULT='"/tmp/r.json"'
export GDNEXT_PLAY_SCREENSHOT='"/tmp/s.png"'
export GDNEXT_PLAY_HUD='"[{\"name\":\"x\"}]"'
export GDNEXT_PLAY='"active"'

echo "==> direct /usr/bin/env"
/usr/bin/env | grep ^GDNEXT_ || echo "(none)"

echo "==> via xvfb-run"
xvfb-run -a --server-args=-screen\ 0\ 1280x720x24 /usr/bin/env | grep ^GDNEXT_ || echo "(none)"
