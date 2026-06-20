package cli

import (
	"context"
	"errors"

	"graphics.gd/cmd/gdnext/internal/project"
	"graphics.gd/cmd/gdnext/internal/setup"

	"github.com/urfave/cli/v3"
)

func testCmd() *cli.Command {
	return &cli.Command{
		Name:            "test",
		Usage:           "cross-compile and run go tests inside the Godot runtime",
		ArgsUsage:       "[-- go-test-flags...]",
		SkipFlagParsing: true,
		Action: func(_ context.Context, cmd *cli.Command) error {
			if helpRequested(cmd) {
				return cli.ShowSubcommandHelp(cmd)
			}
			extra := cmd.Args().Slice()
			platform, err := setup.ForBuild(buildEnv, true, extra)
			if err != nil {
				return err
			}
			if !project.IncludesGo {
				return errors.New("cannot run 'gdnext test' on a project that does not include Go code")
			}
			return platform.Test(buildEnv, setup.TestArgs(extra)...)
		},
	}
}
