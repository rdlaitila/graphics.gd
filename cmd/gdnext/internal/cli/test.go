package cli

import (
	"context"
	"errors"

	"graphics.gd/cmd/gdnext/internal/project"
	"graphics.gd/cmd/gdnext/internal/setup"

	"github.com/samber/do/v2"
	"graphics.gd/cmd/gdnext/internal/shared"
	"github.com/urfave/cli/v3"
)

// TestCommand wires `gdnext test`. Runtime state lives on *TestActions.
type TestCommand struct {
	*cli.Command
	Injector do.Injector `do:""`
}

// TestActions carries the runtime state.
type TestActions struct {
	Injector do.Injector `do:""`
}

// NewTestCommand constructs the `gdnext test` subcommand
func NewTestCommand(di do.Injector) (*TestCommand, error) {
	t := do.MustInvokeStruct[*TestCommand](di)
	t.Command = &cli.Command{
		Name:            "test",
		Usage:           "cross-compile and run go tests inside the Godot runtime",
		ArgsUsage:       "[-- go-test-flags...]",
		SkipFlagParsing: true,
		Action:          shared.BindAction(t.Injector, (*TestActions).test),
	}
	return t, nil
}

// NewTestActions resolves the runtime state for test.
func NewTestActions(di do.Injector) (*TestActions, error) {
	return do.InvokeStruct[*TestActions](di)
}

func (t *TestActions) test(_ context.Context, cmd *cli.Command) error {
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
