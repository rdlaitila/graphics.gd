#!/usr/bin/env bash
# `-goos` (single dash, multi-char) must be rewritten to `--goos` by
# the shortflags preprocessor before urfave/cli sees it.
set -euo pipefail

out=$(gdnext -goos linux build --help)
grep -q "gdnext build" <<<"$out"
