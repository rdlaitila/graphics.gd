package cli

import (
	"context"
	"os"

	"graphics.gd/cmd/gdnext/internal/project"
	"graphics.gd/cmd/gdnext/internal/setup"
	"graphics.gd/cmd/gdnext/internal/tooling"
	"graphics.gd/product"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
	"runtime.link/api/xray"
)

// RunCommand exposes `gdnext run`: build the project as a shared
// library and launch it via Godot (or adb / web server).
type RunCommand struct {
	*cli.Command
	Injector    do.Injector      `do:""`
	BuildEnv    product.BuildEnv `do:""`
	ToolCatalog tooling.Catalog  `do:""`
}

// NewRunCommand constructs the `gdnext run` subcommand
func NewRunCommand(di do.Injector) (*RunCommand, error) {
	t := do.MustInvokeStruct[*RunCommand](di)
	t.Command = &cli.Command{
		Name:            "run",
		Usage:           "build the project as a shared library and launch it via Godot (or adb / web server)",
		ArgsUsage:       "[-- go-build-flags...]",
		SkipFlagParsing: true,
		Action:          t.run,
	}
	return t, nil
}

func (t *RunCommand) run(_ context.Context, cmd *cli.Command) error {
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
