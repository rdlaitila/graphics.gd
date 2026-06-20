package internal

import (
	"context"

	"github.com/urfave/cli/v3"
)

// ShortFlagRewriteCmd ports cmd/gdnext-ci/short-flag-rewrite.sh.
func ShortFlagRewriteCmd() *cli.Command {
	return &cli.Command{
		Name:  "short-flag-rewrite",
		Usage: "verify `-goos` (single-dash, multi-char) is rewritten to `--goos`",
		Action: func(_ context.Context, _ *cli.Command) error {
			out, err := output("gdnext", "-goos", "linux", "build", "--help")
			if err != nil {
				return err
			}
			return mustContain("gdnext -goos linux build --help", out, "gdnext build")
		},
	}
}
