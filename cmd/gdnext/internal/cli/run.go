package cli

import (
	"context"
	"os"

	"graphics.gd/cmd/gdnext/internal/project"
	"graphics.gd/cmd/gdnext/internal/setup"

	"github.com/samber/do/v2"
	"graphics.gd/cmd/gdnext/internal/shared"
	"github.com/urfave/cli/v3"
	"runtime.link/api/xray"
)

// RunCommand wires `gdnext run`. Runtime state lives on *RunActions.
type RunCommand struct {
	*cli.Command
	Injector do.Injector `do:""`
}

// RunActions carries the runtime state.
type RunActions struct {
	Injector do.Injector `do:""`
}

// NewRunCommand constructs the `gdnext run` subcommand
func NewRunCommand(di do.Injector) (*RunCommand, error) {
	t := do.MustInvokeStruct[*RunCommand](di)
	t.Command = &cli.Command{
		Name:            "run",
		Usage:           "build the project as a shared library and launch it via Godot (or adb / web server)",
		ArgsUsage:       "[-- go-build-flags...]",
		SkipFlagParsing: true,
		Action:          shared.BindAction(t.Injector, (*RunActions).run),
	}
	return t, nil
}

// NewRunActions resolves the runtime state for run.
func NewRunActions(di do.Injector) (*RunActions, error) {
	return do.InvokeStruct[*RunActions](di)
}

func (t *RunActions) run(_ context.Context, cmd *cli.Command) error {
	if helpRequested(cmd) {
		return cli.ShowSubcommandHelp(cmd)
	}
	extra := cmd.Args().Slice()
	platform, err := setup.ForBuild(t.Injector, false, extra)
	if err != nil {
		return err
	}
	if err := os.Chdir(project.Directory); err != nil {
		return xray.New(err)
	}
	return platform.Run(append([]string{"-gcflags=graphics.gd/classdb/...=-N -l"}, extra...)...)
}
