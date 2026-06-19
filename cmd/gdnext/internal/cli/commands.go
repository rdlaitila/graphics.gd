// Package cli assembles every gdnext subcommand and the global flag set
// into a single urfave/cli/v3 command tree. Each verb lives in its own
// <name>.go file; commands.go only contains the Commands() / Action() /
// Version() entry points and shared bits that don't belong to any single
// verb.
package cli

import (
	"context"
	"os"
	"runtime/debug"

	"graphics.gd/cmd/gdnext/internal/project"
	"graphics.gd/cmd/gdnext/internal/setup"
	"graphics.gd/cmd/gdnext/internal/tooling"

	"github.com/urfave/cli/v3"
	"runtime.link/api/xray"
)

// Commands returns the full subcommand list attached to the root command.
// Each entry is defined in its own <name>.go file.
func Commands() []*cli.Command {
	return []*cli.Command{
		buildCmd(),
		runCmd(),
		testCmd(),
		exportCmd(),
		docCmd(),
		fixCmd(),
		versionCmd(),
		projectCmd(),
		toolchainCmd(),
		platformsCmd(),
		androidCmd(),
		iosCmd(),
		macosCmd(),
		webCmd(),
		muslCmd(),
	}
}

// Version returns the gdnext binary's own module version, falling back to
// "(devel)" when invoked from a non-vendored build.
func Version() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "(devel)"
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

// LaunchEditor is the root no-subcommand handler: build the project as a shared
// library so the editor sees fresh code, then launch Godot in editor mode.
// RUNNING_INSIDE_GODOT short-circuits the launch to avoid an infinite spawn
// loop when the editor is the parent process.
func LaunchEditor(_ context.Context, cmd *cli.Command) error {
	platform, err := setup.ForBuild(cmd, false, nil)
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
	return tooling.Godot.Exec("-e")
}
