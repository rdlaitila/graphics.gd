package ci

import (
	"context"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
)

// ShortFlagRewriteCommand exposes `gdnext ci short-flag-rewrite`:
// verify `-goos` (single-dash, multi-char) is rewritten to `--goos`.
type ShortFlagRewriteCommand struct {
	*cli.Command
}

// NewShortFlagRewriteCommand constructs the short-flag-rewrite subcommand.
func NewShortFlagRewriteCommand(di do.Injector) (*ShortFlagRewriteCommand, error) {
	t := do.MustInvokeStruct[*ShortFlagRewriteCommand](di)
	t.Command = &cli.Command{
		Name:   "short-flag-rewrite",
		Usage:  "verify `-goos` (single-dash, multi-char) is rewritten to `--goos`",
		Action: t.action,
	}
	return t, nil
}

func (t *ShortFlagRewriteCommand) action(_ context.Context, _ *cli.Command) error {
	out, err := output("gdnext", "-goos", "linux", "build", "--help")
	if err != nil {
		return err
	}
	return mustContain("gdnext -goos linux build --help", out, "gdnext build")
}
