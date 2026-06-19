package main

import (
	"context"

	"github.com/urfave/cli/v3"
)

// shortFlagRewriteCmd ports cmd/gdnext-ci/short-flag-rewrite.sh.
func shortFlagRewriteCmd() *cli.Command {
	return &cli.Command{
		Name:  "short-flag-rewrite",
		Usage: "verify `-goos` (single-dash, multi-char) is rewritten to `--goos`",
		Description: "`gdnext -goos linux build --help` should reach the build verb's\n" +
			"help page; if the shortflags preprocessor regresses, the leading\n" +
			"`-goos` token would be parsed as `-g -o -o -s` and the build\n" +
			"verb's help would never render.",
		Action: func(_ context.Context, _ *cli.Command) error {
			out, err := output("gdnext", "-goos", "linux", "build", "--help")
			if err != nil {
				return err
			}
			return mustContain("gdnext -goos linux build --help", out, "gdnext build")
		},
	}
}
