#!/usr/bin/env bash
# Confirm gdnext's CommandNotFound passthrough still forwards unknown
# verbs to the underlying `go` toolchain. Output is captured before any
# `head`/`grep -q` so partial reads don't SIGPIPE gdnext under
# set -o pipefail.
set -euo pipefail

gdnext env GOVERSION
std=$(gdnext list std)
head -3 <<<"$std"
mod=$(gdnext mod help)
grep -q "Go mod" <<<"$mod"
