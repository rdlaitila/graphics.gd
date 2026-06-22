package cli

import (
	"context"
	"errors"

	"graphics.gd/cmd/gdnext/internal/project"
	"graphics.gd/cmd/gdnext/internal/setup"
	"graphics.gd/cmd/gdnext/internal/tooling"
	"graphics.gd/product"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
)

// TestCommand exposes `gdnext test`: cross-compile and run go tests
// inside the Godot runtime.
type TestCommand struct {
	*cli.Command
	Injector    do.Injector      `do:""`
	BuildEnv    product.BuildEnv `do:""`
	ToolCatalog tooling.Catalog  `do:""`
}

// NewTestCommand constructs the `gdnext test` subcommand
func NewTestCommand(di do.Injector) (*TestCommand, error) {
	t := do.MustInvokeStruct[*TestCommand](di)
	t.Command = &cli.Command{
		Name:            "test",
		Usage:           "cross-compile and run go tests inside the Godot runtime",
		ArgsUsage:       "[-- go-test-flags...]",
		SkipFlagParsing: true,
		Action:          t.test,
	}
	return t, nil
}

func (t *TestCommand) test(_ context.Context, cmd *cli.Command) error {
	if helpRequested(cmd) {
		return cli.ShowSubcommandHelp(cmd)
	}
	extra := cmd.Args().Slice()
	platform, err := setup.ForBuild(t.Injector, true, extra)
	if err != nil {
		return err
	}
	if !project.IncludesGo {
		return errors.New("cannot run 'gdnext test' on a project that does not include Go code")
	}
	return platform.Test(setup.TestArgs(extra)...)
}
