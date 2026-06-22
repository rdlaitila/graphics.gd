package cli

import (
	"context"
	"os"
	"path/filepath"

	"graphics.gd/cmd/gdnext/internal/project"
	"graphics.gd/cmd/gdnext/internal/setup"
	"graphics.gd/cmd/gdnext/internal/tooling"
	"graphics.gd/product"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
	"runtime.link/api/xray"
)

// BuildCommand exposes both `gdnext build` and the `export` alias.
// Both verbs invoke the same Action — the alias is wired through
// ExportCommand so it shows up in `gdnext help` under its preferred
// name without forcing callers to construct a second BuildCommand.
type BuildCommand struct {
	*cli.Command
	Injector    do.Injector      `do:""`
	BuildEnv    product.BuildEnv `do:""`
	ToolCatalog tooling.Catalog  `do:""`
}

// ExportCommand is the alias-shaped sibling of BuildCommand. They share
// the same Action so the behaviour stays identical.
type ExportCommand struct {
	*cli.Command
	Injector    do.Injector      `do:""`
	BuildEnv    product.BuildEnv `do:""`
	ToolCatalog tooling.Catalog  `do:""`
}

// NewBuildCommand constructs the `gdnext build` subcommand
func NewBuildCommand(di do.Injector) (*BuildCommand, error) {
	t := do.MustInvokeStruct[*BuildCommand](di)
	t.Command = &cli.Command{
		Name:            "build",
		Usage:           "cross-compile and produce a distributable binary (Godot --export-release)",
		ArgsUsage:       "[-- go-build-flags...]",
		SkipFlagParsing: true,
		Action:          t.build,
	}
	return t, nil
}

// NewExportCommand constructs the `gdnext export` alias subcommand
func NewExportCommand(di do.Injector) (*ExportCommand, error) {
	t := do.MustInvokeStruct[*ExportCommand](di)
	t.Command = &cli.Command{
		Name:            "export",
		Usage:           "alias for 'build' — produce a distributable binary via Godot export",
		ArgsUsage:       "[-- go-build-flags...]",
		SkipFlagParsing: true,
		Action:          t.build,
	}
	return t, nil
}

func (t *BuildCommand) build(_ context.Context, cmd *cli.Command) error {
	return buildAction(t.Injector, t.BuildEnv, cmd)
}

func (t *ExportCommand) build(_ context.Context, cmd *cli.Command) error {
	return buildAction(t.Injector, t.BuildEnv, cmd)
}

func buildAction(di do.Injector, env product.BuildEnv, cmd *cli.Command) error {
	if helpRequested(cmd) {
		return cli.ShowSubcommandHelp(cmd)
	}
	extra := cmd.Args().Slice()
	platform, err := setup.ForBuild(di, false, extra)
	if err != nil {
		return err
	}
	if err := os.Chdir(project.Directory); err != nil {
		return xray.New(err)
	}
	tools := do.MustInvoke[tooling.Catalog](di)
	if err := setup.AssertTemplate(env, tools.Godot.Version); err != nil {
		return xray.New(err)
	}
	if err := os.MkdirAll(filepath.Join(project.ReleasesDirectory, env.Target.GOOS, env.Target.GOARCH), 0755); err != nil {
		return xray.New(err)
	}
	return platform.BuildMain(append([]string{"-ldflags=-s -w"}, extra...)...)
}
