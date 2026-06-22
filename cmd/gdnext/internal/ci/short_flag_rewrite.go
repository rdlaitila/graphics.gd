package ci

import (
	"context"

	"graphics.gd/cmd/gdnext/internal/shared"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
)

// ShortFlagRewriteCommand wires `gdnext ci short-flag-rewrite`.
// Runtime state lives on *ShortFlagRewriteActions.
type ShortFlagRewriteCommand struct {
	*cli.Command
	Injector do.Injector `do:""`
}

// ShortFlagRewriteActions carries the runtime state.
type ShortFlagRewriteActions struct{}

// NewShortFlagRewriteCommand constructs the short-flag-rewrite subcommand.
func NewShortFlagRewriteCommand(di do.Injector) (*ShortFlagRewriteCommand, error) {
	t := do.MustInvokeStruct[*ShortFlagRewriteCommand](di)
	t.Command = &cli.Command{
		Name:   "short-flag-rewrite",
		Usage:  "verify `-goos` (single-dash, multi-char) is rewritten to `--goos`",
		Action: shared.BindAction(t.Injector, (*ShortFlagRewriteActions).action),
	}
	return t, nil
}

// NewShortFlagRewriteActions resolves the runtime state.
func NewShortFlagRewriteActions(di do.Injector) (*ShortFlagRewriteActions, error) {
	return do.InvokeStruct[*ShortFlagRewriteActions](di)
}

func (t *ShortFlagRewriteActions) action(_ context.Context, _ *cli.Command) error {
	out, err := output("gdnext", "-goos", "linux", "build", "--help")
	if err != nil {
		return err
	}
	return mustContain("gdnext -goos linux build --help", out, "gdnext build")
}
