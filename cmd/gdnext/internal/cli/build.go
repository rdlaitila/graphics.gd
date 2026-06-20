package cli

import (
	"context"
	"os"
	"path/filepath"

	"graphics.gd/cmd/gdnext/internal/project"
	"graphics.gd/cmd/gdnext/internal/setup"
	"graphics.gd/cmd/gdnext/internal/templates"
	"graphics.gd/cmd/gdnext/internal/tooling"

	"github.com/urfave/cli/v3"
	"runtime.link/api/xray"
)

func buildCmd() *cli.Command {
	return &cli.Command{
		Name:            "build",
		Usage:           "cross-compile and produce a distributable binary (Godot --export-release)",
		ArgsUsage:       "[-- go-build-flags...]",
		SkipFlagParsing: true,
		Action:          buildAction,
	}
}

// exportCmd is "gdnext export": a clearer-named alias for "build". Both
// verbs invoke buildAction so the behaviour stays identical.
func exportCmd() *cli.Command {
	cmd := buildCmd()
	cmd.Name = "export"
	cmd.Usage = "alias for 'build' — produce a distributable binary via Godot export"
	return cmd
}

func buildAction(_ context.Context, cmd *cli.Command) error {
	if helpRequested(cmd) {
		return cli.ShowSubcommandHelp(cmd)
	}
	extra := cmd.Args().Slice()
	platform, env, err := setup.ForBuild(cmd, false, extra)
	if err != nil {
		return err
	}
	if err := os.Chdir(project.Directory); err != nil {
		return xray.New(err)
	}
	if err := templates.Assert(tooling.Godot.Version); err != nil {
		return xray.New(err)
	}
	if err := os.MkdirAll(filepath.Join(project.ReleasesDirectory, env.TargetGOOS, env.TargetGOARCH), 0755); err != nil {
		return xray.New(err)
	}
	return platform.BuildMain(env, append([]string{"-ldflags=-s -w"}, extra...)...)
}
