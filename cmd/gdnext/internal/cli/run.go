package cli

import (
	"context"
	"os"

	"graphics.gd/cmd/gdnext/internal/project"
	"graphics.gd/cmd/gdnext/internal/setup"

	"github.com/urfave/cli/v3"
	"runtime.link/api/xray"
)

func runCmd() *cli.Command {
	return &cli.Command{
		Name:            "run",
		Usage:           "build the project as a shared library and launch it via Godot (or adb / web server)",
		ArgsUsage:       "[-- go-build-flags...]",
		SkipFlagParsing: true,
		Action:          runAction,
	}
}

func runAction(_ context.Context, cmd *cli.Command) error {
	if helpRequested(cmd) {
		return cli.ShowSubcommandHelp(cmd)
	}
	extra := cmd.Args().Slice()
	platform, err := setup.ForBuild(buildEnv, false, extra)
	if err != nil {
		return err
	}
	if err := os.Chdir(project.Directory); err != nil {
		return xray.New(err)
	}
	return platform.Run(buildEnv, append([]string{"-gcflags=graphics.gd/classdb/...=-N -l"}, extra...)...)
}
