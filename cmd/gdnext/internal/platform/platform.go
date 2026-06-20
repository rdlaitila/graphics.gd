// Package platform maps the user-facing GOOS aliases accepted by gdnext (and
// by the legacy gd command) onto the concrete builder.* implementations
// living under cmd/gdnext/internal/builder. The mapping is the single source
// of truth for which targets gdnext understands and which env-var defaults
// each target imposes (Android/iOS default to GOARCH=arm64, etc.).
package platform

import (
	"fmt"
	"os"

	"graphics.gd/cmd/gdnext/internal/builder"
	"graphics.gd/product"
)

// Builder is the union of every method any of the per-platform Builder types
// expose to gdnext. Each method takes a product.BuildEnv up front so the
// (host, target) pair is explicit at the API boundary — the methods do not
// re-read $GOOS / $GOARCH for their own decisions.
type Builder interface {
	Run(env product.BuildEnv, args ...string) error
	Build(env product.BuildEnv, args ...string) error
	BuildMain(env product.BuildEnv, args ...string) error
	Test(env product.BuildEnv, args ...string) error
}

// Compile-time assertions
var (
	_ Builder = builder.Linux{}
	_ Builder = builder.Windows{}
	_ Builder = builder.MacOS{}
	_ Builder = builder.IOS{}
	_ Builder = builder.Android{}
	_ Builder = builder.MetaQuest{}
	_ Builder = builder.Browser{}
	_ Builder = (*builder.Musl)(nil)
)

// For returns the Builder responsible for the supplied GOOS-or-alias string.
// As a side effect it canonicalises the GOOS / GOARCH environment variables
// because spawned subprocesses (go build, godot, etc.) read them directly.
// Returns a nil Builder and exits the process when goos is not recognised —
// this preserves the legacy behaviour of cmd/gd's builderFor.
func For(goos string) Builder {
	switch goos {
	case "linux", "ubuntu", "arch", "debian", "nix", "musl":
		if goos == "musl" {
			return &builder.Musl{}
		}
		return builder.Linux{}
	case "windows", "win":
		os.Setenv("GOOS", "windows")
		return builder.Windows{}
	case "darwin", "macos":
		os.Setenv("GOOS", "darwin")
		return builder.MacOS{}
	case "ios", "iphone":
		os.Setenv("GOOS", "ios")
		if os.Getenv("GOARCH") == "" {
			os.Setenv("GOARCH", "arm64")
		}
		return builder.IOS{}
	case "android":
		os.Setenv("GOOS", "android")
		if os.Getenv("GOARCH") == "" {
			os.Setenv("GOARCH", "arm64")
		}
		return builder.Android{}
	case "metaquest", "quest", "meta":
		os.Setenv("GOOS", "android")
		os.Setenv("GOARCH", "arm64")
		return builder.MetaQuest{}
	case "browser", "js", "web", "wasm":
		os.Setenv("GOOS", "js")
		return builder.Browser{}
	default:
		fmt.Fprint(os.Stderr, "gdnext: unsupported GOOS '"+goos+"'\n")
		os.Exit(1)
		return nil
	}
}
