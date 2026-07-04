// Package cli hosts the urfave/cli/v3 command tree wired into the
// gdnext binary. Every top-level verb (`build`, `run`, `test`,
// `toolchain`, ...) lives in its own `<verb>.go` file and follows
// the same two-struct DI shape:
//
//   - `XCommand` holds the *cli.Command built at startup (flags,
//     subcommands, Action bindings). Only startup-time state.
//   - `XActions` holds the per-invocation state (BuildEnv, catalog,
//     ...) resolved lazily via `shared.BindAction` when an Action
//     fires — after the Before hook has finalised the BuildEnv.
//
// `NewXCommand` + `NewXActions` are DI constructors registered in
// `Provides` and consumed by samber/do. They exist purely as wiring;
// no caller invokes them directly.
package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
	"graphics.gd/cmd/gdnext/internal/ci"
	"graphics.gd/cmd/gdnext/internal/project"
	"graphics.gd/cmd/gdnext/internal/setup"
	"graphics.gd/cmd/gdnext/internal/shared"
	"graphics.gd/cmd/gdnext/internal/tooling"
	"graphics.gd/product"
	"runtime.link/api/xray"
)

// RootCommand wires the root urfave command tree.
type RootCommand struct {
	*cli.Command
	Injector do.Injector `do:""`
}

// RootActions carries the runtime state the root verb (no-arg editor
// launch) and the go-passthrough fallback need to resolve once the
// Before hook has finalised the BuildEnv.
type RootActions struct {
	Injector    do.Injector      `do:""`
	BuildEnv    product.BuildEnv `do:""`
	ToolCatalog tooling.Catalog  `do:""`
}

// Provides is the package-level provider set for the gdnext CLI
var Provides = do.Package(
	do.Lazy(NewRootCommand),
	do.Lazy(NewRootActions),
	do.Lazy(NewAndroidCommand),
	do.Lazy(NewAndroidActions),
	do.Lazy(NewBuildCommand),
	do.Lazy(NewBuildActions),
	do.Lazy(NewExportCommand),
	do.Lazy(NewRunCommand),
	do.Lazy(NewRunActions),
	do.Lazy(NewTestCommand),
	do.Lazy(NewTestActions),
	do.Lazy(NewDocCommand),
	do.Lazy(NewDocActions),
	do.Lazy(NewFixCommand),
	do.Lazy(NewFixActions),
	do.Lazy(NewVersionCommand),
	do.Lazy(NewVersionActions),
	do.Lazy(NewProjectCommand),
	do.Lazy(NewProjectActions),
	do.Lazy(NewToolchainCommand),
	do.Lazy(NewToolchainActions),
	do.Lazy(NewPlatformCommand),
	do.Lazy(NewPlatformActions),
	do.Lazy(NewQuirksCommand),
	do.Lazy(NewQuirksActions),
	do.Lazy(NewIosCommand),
	do.Lazy(NewIosActions),
	do.Lazy(NewMacosCommand),
	do.Lazy(NewMacosActions),
	do.Lazy(NewWebCommand),
	do.Lazy(NewWebActions),
	do.Lazy(NewLibGodotCommand),
	do.Lazy(NewLibGodotActions),
)

func NewRootCommand(di do.Injector) (*RootCommand, error) {
	t := do.MustInvokeStruct[*RootCommand](di)
	t.Command = &cli.Command{
		Name:                  "gdnext",
		Usage:                 "Drop-in replacement for the go command for Godot-based projects",
		Version:               version(),
		Suggest:               true,
		EnableShellCompletion: true,
		Flags:                 flags(),
		Before:                t.before,
		Action:                shared.BindAction(t.Injector, (*RootActions).launchEditor),
		CommandNotFound:       passthroughToGo(t.Injector),
		Commands:              commands(di),
	}
	return t, nil
}

func NewRootActions(di do.Injector) (*RootActions, error) {
	return do.InvokeStruct[*RootActions](di)
}

// before is the urfave Before hook for the root command.
func (t *RootCommand) before(ctx context.Context, c *cli.Command) (context.Context, error) {
	if _, err := promoteFlagsToEnv(ctx, c); err != nil {
		return ctx, xray.New(err)
	}
	// Replace the cached BuildEnv first, then the Catalog (which
	// resolves BuildEnv internally). do.Override clears the cached
	// instance, so the next Invoke re-runs the constructor against
	// the now-correct env.
	do.Override(t.Injector, setup.NewBuildEnv)
	do.Override(t.Injector, tooling.NewCatalog)
	return ctx, nil
}

// launchEditor is the root no-subcommand handler: build the project as a shared
// library so the editor sees fresh code, then launch Godot in editor mode.
// RUNNING_INSIDE_GODOT short-circuits the launch to avoid an infinite spawn
// loop when the editor is the parent process.
func (t *RootActions) launchEditor(_ context.Context, cmd *cli.Command) error {
	platform, err := setup.ForBuild(t.Injector, false, nil)
	if err != nil {
		return err
	}
	if err := os.Chdir(project.Directory); err != nil {
		return xray.New(err)
	}
	if err := platform.Build("-gcflags=graphics.gd/classdb/...=-N -l"); err != nil {
		return xray.New(err)
	}
	if err := os.Chdir(project.GraphicsDirectory); err != nil {
		return xray.New(err)
	}
	if cmd.Bool("inside-godot") || os.Getenv(product.EnvRunningInsideGodot) != "" {
		return nil
	}
	return t.ToolCatalog.Godot.Exec("-e")
}

// passthroughToGo is the urfave CommandNotFound handler. It captures
// the injector at construction time so the closure can resolve a fresh
// RootActions per invocation.
func passthroughToGo(di do.Injector) cli.CommandNotFoundFunc {
	return func(_ context.Context, cmd *cli.Command, name string) {
		if name == "" {
			return
		}
		t, err := do.Invoke[*RootActions](di)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		args := append([]string{name}, cmd.Args().Slice()...)
		if err := t.ToolCatalog.Go.Exec(args...); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}

// commands returns the full subcommand list attached to the root command.
// Each entry is defined in its own <name>.go file.
func commands(di do.Injector) []*cli.Command {
	return []*cli.Command{
		do.MustInvoke[*BuildCommand](di).Command,
		do.MustInvoke[*RunCommand](di).Command,
		do.MustInvoke[*TestCommand](di).Command,
		do.MustInvoke[*ExportCommand](di).Command,
		do.MustInvoke[*DocCommand](di).Command,
		do.MustInvoke[*FixCommand](di).Command,
		do.MustInvoke[*VersionCommand](di).Command,
		do.MustInvoke[*ProjectCommand](di).Command,
		do.MustInvoke[*ToolchainCommand](di).Command,
		do.MustInvoke[*PlatformCommand](di).Command,
		do.MustInvoke[*QuirksCommand](di).Command,
		do.MustInvoke[*AndroidCommand](di).Command,
		do.MustInvoke[*IosCommand](di).Command,
		do.MustInvoke[*MacosCommand](di).Command,
		do.MustInvoke[*WebCommand](di).Command,
		do.MustInvoke[*LibGodotCommand](di).Command,
		do.MustInvoke[*ci.CICommand](di).Command,
	}
}

// helpRequested reports whether the user typed `--help` / `-h` anywhere
// in the positional args of a SkipFlagParsing subcommand. urfave normally
// intercepts the help flag before the Action runs, but SkipFlagParsing
// disables that, so verbs that forward all flags to a downstream tool
// (build → go build, doc → go doc, etc.) need to opt back in by calling
// this at the top of their Action.
func helpRequested(cmd *cli.Command) bool {
	for _, a := range cmd.Args().Slice() {
		if a == "--help" || a == "-h" {
			return true
		}
		if a == "--" {
			return false
		}
	}
	return false
}
