package cli

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime/debug"

	"github.com/urfave/cli/v3"
	"graphics.gd/cmd/gdnext/internal/project"
	"graphics.gd/cmd/gdnext/internal/setup"
	"graphics.gd/cmd/gdnext/internal/tooling"
	"graphics.gd/product"
	"runtime.link/api/xray"
)

var buildEnv product.BuildEnv

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
		platformCmd(),
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

// Before is the urfave Before hook for the root command. It promotes
// global flags to environment variables so they are visible to all
// subcommands and the editor launch. It also determines the build
// environment based on the current GOOS and GOARCH and stores it in
// the package-level buildEnv variable.
func Before(ctx context.Context, c *cli.Command) (context.Context, error) {
	if _, err := promoteFlagsToEnv(ctx, c); err != nil {
		return ctx, xray.New(err)
	}
	if err := prepareBuildEnv(); err != nil {
		return ctx, xray.New(err)
	}
	return ctx, nil
}

// LaunchEditor is the root no-subcommand handler: build the project as a shared
// library so the editor sees fresh code, then launch Godot in editor mode.
// RUNNING_INSIDE_GODOT short-circuits the launch to avoid an infinite spawn
// loop when the editor is the parent process.
func LaunchEditor(_ context.Context, cmd *cli.Command) error {
	platform, err := setup.ForBuild(buildEnv, false, nil)
	if err != nil {
		return err
	}
	if err := os.Chdir(project.Directory); err != nil {
		return xray.New(err)
	}
	if err := platform.Build(buildEnv, "-gcflags=graphics.gd/classdb/...=-N -l"); err != nil {
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

func prepareBuildEnv() error {
	env, err := product.FindBuildEnv(os.Getenv("GOOS"), os.Getenv("GOARCH"), os.Getenv("GOLINK"))
	if err != nil {
		return xray.New(err)
	}
	buildEnv = env
	// Only write GOOS/GOARCH back when FindBuildEnv canonicalised an
	// alias the user passed in (e.g. macos -> darwin, web -> js).
	// Skipping the no-op write keeps "was this user-set?" introspection
	// honest for any downstream tool that cares.
	if os.Getenv("GOOS") != buildEnv.Target.GOOS {
		if err := os.Setenv("GOOS", buildEnv.Target.GOOS); err != nil {
			return xray.New(err)
		}
	}
	if os.Getenv("GOARCH") != buildEnv.Target.GOARCH {
		if err := os.Setenv("GOARCH", buildEnv.Target.GOARCH); err != nil {
			return xray.New(err)
		}
	}
	if linkStr := buildEnv.Target.LinkMode.String(); os.Getenv("GOLINK") != linkStr {
		if err := os.Setenv("GOLINK", linkStr); err != nil {
			return xray.New(err)
		}
	}
	home, err := homeDir()
	if err != nil {
		return xray.New(err)
	}
	buildEnv.Host.UserHomeRoot = home
	appdata, err := appdataRoot(buildEnv.Host.GOOS, home)
	if err != nil {
		return xray.New(err)
	}
	buildEnv.Host.UserAppdataRoot = appdata
	root := os.Getenv("GDPATH")
	if root == "" {
		root = filepath.Join(home, "gd")
	}
	buildEnv.Host.GDRootPath = root
	buildEnv.Host.GDBinPath = filepath.Join(root, "bin")
	buildEnv.Host.GDLibPath = filepath.Join(root, "lib")
	// Reflect the canonical GDPATH back into the environment so any
	// downstream code that still reads it (tooling.Tool.LookupPlatform,
	// spawned subprocesses) sees the same value the typed BuildEnv
	// carries. New code should consume BuildEnv.Host.GD*Path instead.
	if os.Getenv("GDPATH") != root {
		if err := os.Setenv("GDPATH", root); err != nil {
			return xray.New(err)
		}
	}
	if err := buildEnv.Validate(); err != nil {
		return xray.New(err)
	}
	return nil
}

// homeDir resolves the host user's home directory. It tries
// os.UserHomeDir first (which respects $HOME / %USERPROFILE%) and falls
// back to user.Current() for environments where neither env var is set
// but the OS still knows the user (some sandboxed or daemonised shells).
func homeDir() (string, error) {
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		return h, nil
	}
	whoami, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("could not resolve user home: %w", err)
	}
	if whoami.HomeDir == "" {
		return "", fmt.Errorf("could not resolve user home: empty HomeDir for %s", whoami.Username)
	}
	return whoami.HomeDir, nil
}

// appdataRoot returns the per-user application-data anchor for goos:
//
//	linux   $XDG_DATA_HOME or ~/.local/share        (XDG Base Directory)
//	windows %APPDATA% or ~\AppData\Roaming          (Roaming AppData)
//	darwin  ~/Library/Application Support           (Apple HIG)
//
// Deep callers compose this with the application's own conventional
// subdir (Godot uses "godot" on linux, "Godot" elsewhere) instead of
// re-branching on goos themselves. An unknown goos is a hard error;
// silently returning home would mis-place files on hosts gdnext hasn't
// been taught about.
func appdataRoot(goos, home string) (string, error) {
	switch goos {
	case product.GOOSLinux:
		if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
			return xdg, nil
		}
		return filepath.Join(home, ".local", "share"), nil
	case product.GOOSWindows:
		if appdata := os.Getenv("APPDATA"); appdata != "" {
			return appdata, nil
		}
		return filepath.Join(home, "AppData", "Roaming"), nil
	case product.GOOSDarwin:
		return filepath.Join(home, "Library", "Application Support"), nil
	default:
		return "", fmt.Errorf("appdataRoot: no per-user app data anchor for goos %q", goos)
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
