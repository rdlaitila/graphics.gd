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
	"graphics.gd/cmd/gdnext/internal/shared"
	"github.com/urfave/cli/v3"
	"runtime.link/api/xray"
)

// BuildCommand wires `gdnext build`. It holds only the urfave Command
// + Injector at startup; runtime state (BuildEnv, Catalog) lives on
// *BuildActions, lazily resolved by bindAction once Before has
// finalised env-dependent state.
type BuildCommand struct {
	*cli.Command
	Injector do.Injector `do:""`
}

// ExportCommand is the alias-shaped sibling of BuildCommand. They
// share the same Action so the behaviour stays identical.
type ExportCommand struct {
	*cli.Command
	Injector do.Injector `do:""`
}

// BuildActions carries the runtime state both verbs need.
type BuildActions struct {
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
		Action:          shared.BindAction(t.Injector, (*BuildActions).build),
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
		Action:          shared.BindAction(t.Injector, (*BuildActions).build),
	}
	return t, nil
}

// NewBuildActions resolves the runtime state for build/export.
func NewBuildActions(di do.Injector) (*BuildActions, error) {
	return do.InvokeStruct[*BuildActions](di)
}

func (t *BuildActions) build(_ context.Context, cmd *cli.Command) error {
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
	if err := setup.AssertTemplate(t.BuildEnv, t.ToolCatalog.Godot.Version); err != nil {
		return xray.New(err)
	}
	if err := os.MkdirAll(filepath.Join(project.ReleasesDirectory, t.BuildEnv.Target.GOOS, t.BuildEnv.Target.GOARCH), 0755); err != nil {
		return xray.New(err)
	}
	return platform.BuildMain(append([]string{"-ldflags=-s -w"}, extra...)...)
}
