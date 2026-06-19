#!/usr/bin/env bash
# Smoke-test that every registered gdnext verb resolves --help to a
# non-empty page mentioning its own name. Output is captured before
# grepping so `grep -q` closing stdin early doesn't SIGPIPE gdnext
# (which set -o pipefail would otherwise propagate as exit 141).
set -euo pipefail

out=$(gdnext --help)
grep -q gdnext <<<"$out"
for v in build run test export doc fix version project toolchain android ios macos web musl; do
  out=$(gdnext "$v" --help)
  grep -q "$v" <<<"$out"
done
