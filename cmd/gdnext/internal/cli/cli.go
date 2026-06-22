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
	"graphics.gd/cmd/gdnext/internal/tooling"
	"graphics.gd/product"
	"runtime.link/api/xray"
)

// RootCommand wraps the urfave Command with any additional context or helpers needed by gdnext.
type RootCommand struct {
	*cli.Command
	Injector    do.Injector      `do:""`
	BuildEnv    product.BuildEnv `do:""`
	ToolCatalog tooling.Catalog  `do:""`
}

// Provides is the package-level provider set for the gdnext CLI
var Provides = do.Package(
	do.Lazy(NewRootCommand),
	do.Lazy(NewAndroidCommand),
	do.Lazy(NewBuildCommand),
	do.Lazy(NewExportCommand),
	do.Lazy(NewRunCommand),
	do.Lazy(NewTestCommand),
	do.Lazy(NewDocCommand),
	do.Lazy(NewFixCommand),
	do.Lazy(NewVersionCommand),
	do.Lazy(NewProjectCommand),
	do.Lazy(NewToolchainCommand),
	do.Lazy(NewPlatformCommand),
	do.Lazy(NewIosCommand),
	do.Lazy(NewMacosCommand),
	do.Lazy(NewWebCommand),
	do.Lazy(NewMuslCommand),
)

// NewRootCommand constructs the root command for gdnext
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
		Action:                t.launchEditor,
		CommandNotFound:       t.passthroughToGo(),
		Commands:              commands(di),
	}
	return t, nil
}

// before is the urfave Before hook for the root command.
func (t *RootCommand) before(ctx context.Context, c *cli.Command) (context.Context, error) {
	if _, err := promoteFlagsToEnv(ctx, c); err != nil {
		return ctx, xray.New(err)
	}
	return ctx, nil
}

// launchEditor is the root no-subcommand handler: build the project as a shared
// library so the editor sees fresh code, then launch Godot in editor mode.
// RUNNING_INSIDE_GODOT short-circuits the launch to avoid an infinite spawn
// loop when the editor is the parent process.
func (t *RootCommand) launchEditor(_ context.Context, cmd *cli.Command) error {
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
	if cmd.Bool("inside-godot") || os.Getenv("RUNNING_INSIDE_GODOT") != "" {
		return nil
	}
	return t.ToolCatalog.Godot.Exec("-e")
}

// passthroughToGo remains the urfave CommandNotFound handler for the rare
// case where execution reaches urfave with a verb we didn't catch in
// goPassthrough — defensive backup, not the primary path.
func (t *RootCommand) passthroughToGo() cli.CommandNotFoundFunc {
	return func(_ context.Context, cmd *cli.Command, name string) {
		if name == "" {
			return
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
		do.MustInvoke[*AndroidCommand](di).Command,
		do.MustInvoke[*IosCommand](di).Command,
		do.MustInvoke[*MacosCommand](di).Command,
		do.MustInvoke[*WebCommand](di).Command,
		do.MustInvoke[*MuslCommand](di).Command,
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
