package setup

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"

	"github.com/samber/do/v2"
	"graphics.gd/product"
	"runtime.link/api/xray"
)

// NewBuildEnv constructs the canonical BuildEnv for the current host and target, reflecting resolved
// GOOS/GOARCH/GOLINK/LibC back into the environment.
func NewBuildEnv(di do.Injector) (product.BuildEnv, error) {
	buildEnv, err := product.FindBuildEnv(
		os.Getenv(product.EnvGOOS),
		os.Getenv(product.EnvGOARCH),
		os.Getenv(product.EnvLibGodotLibC),
		os.Getenv(product.EnvGOLink),
	)
	if err != nil {
		return buildEnv, xray.New(err)
	}
	// Only write GOOS/GOARCH back when FindBuildEnv canonicalised an
	// alias the user passed in (e.g. macos -> darwin, web -> js).
	// Skipping the no-op write keeps "was this user-set?" introspection
	// honest for any downstream tool that cares.
	if os.Getenv(product.EnvGOOS) != buildEnv.Target.GOOS {
		if err := os.Setenv(product.EnvGOOS, buildEnv.Target.GOOS); err != nil {
			return buildEnv, xray.New(err)
		}
	}
	if os.Getenv(product.EnvGOARCH) != buildEnv.Target.GOARCH {
		if err := os.Setenv(product.EnvGOARCH, buildEnv.Target.GOARCH); err != nil {
			return buildEnv, xray.New(err)
		}
	}
	if linkStr := buildEnv.Target.LinkMode.String(); os.Getenv(product.EnvGOLink) != linkStr {
		if err := os.Setenv(product.EnvGOLink, linkStr); err != nil {
			return buildEnv, xray.New(err)
		}
	}
	if buildEnv.Target.LibC != "" && os.Getenv(product.EnvLibGodotLibC) != buildEnv.Target.LibC {
		if err := os.Setenv(product.EnvLibGodotLibC, buildEnv.Target.LibC); err != nil {
			return buildEnv, xray.New(err)
		}
	}
	home, err := homeDir()
	if err != nil {
		return buildEnv, xray.New(err)
	}
	buildEnv.Host.UserHomeRoot = home
	appdata, err := appdataRoot(buildEnv.Host.GOOS, home)
	if err != nil {
		return buildEnv, xray.New(err)
	}
	buildEnv.Host.UserAppdataRoot = appdata
	root := os.Getenv(product.EnvGDPath)
	if root == "" {
		root = filepath.Join(home, "gd")
	}
	buildEnv.Host.GDRootPath = root
	buildEnv.Host.GDBinPath = filepath.Join(root, "bin")
	buildEnv.Host.GDLibPath = filepath.Join(root, "lib")
	// Reflect the canonical GDPATH back into the environment so any
	// spawned subprocesses that still read it see the value the typed
	// BuildEnv carries. New code should consume BuildEnv.Host.GD*Path
	// instead.
	if os.Getenv(product.EnvGDPath) != root {
		if err := os.Setenv(product.EnvGDPath, root); err != nil {
			return buildEnv, xray.New(err)
		}
	}
	if err := buildEnv.Validate(); err != nil {
		return buildEnv, xray.New(err)
	}
	return buildEnv, nil
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
		if xdg := os.Getenv(product.EnvXDGDataHome); xdg != "" {
			return xdg, nil
		}
		return filepath.Join(home, ".local", "share"), nil
	case product.GOOSWindows:
		if appdata := os.Getenv(product.EnvAppData); appdata != "" {
			return appdata, nil
		}
		return filepath.Join(home, "AppData", "Roaming"), nil
	case product.GOOSDarwin:
		return filepath.Join(home, "Library", "Application Support"), nil
	default:
		return "", fmt.Errorf("appdataRoot: no per-user app data anchor for goos %q", goos)
	}
}
