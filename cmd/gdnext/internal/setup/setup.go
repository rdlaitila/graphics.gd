// Package setup runs the "Phase 2" pipeline shared by every build / run /
// test invocation: it propagates global flags into the env vars the legacy
// builder/* code expects, validates GOOS/GOARCH, detects musl on Linux,
// chooses the right C compiler (zig/clang) when CC is unset, and runs
// project.Setup + docgen.Process. Ported from cmd/gd/main.go:gd().
package setup

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"

	"graphics.gd/cmd/gdnext/internal/builder"
	"graphics.gd/cmd/gdnext/internal/platform"
	"graphics.gd/cmd/gdnext/internal/project"
	"graphics.gd/cmd/gdnext/internal/tooling"
	"graphics.gd/internal/docgen"

	"github.com/urfave/cli/v3"
	"runtime.link/api/xray"
)

// ForBuild runs the full setup pipeline and returns the platform-specific
// Builder ready for Build/Run/BuildMain/Test. testing should be true only
// for the "test" subcommand (it switches musl auto-detect into Test mode).
// extraArgs are the user-supplied positional args after "gdnext <verb>";
// they're passed into the musl test closure so it can honour -bench.
//
// The flag→env bridge (--goos / --goarch / --cc / --cgo / --gdpath) is
// handled by cli.PromoteFlagsToEnv on the root command's Before hook, so
// by the time ForBuild runs every legacy os.Getenv read sees the right
// values.
func ForBuild(cmd *cli.Command, testing bool, extraArgs []string) (platform.Builder, error) {
	GOOS := runtime.GOOS
	GOARCH := runtime.GOARCH
	if v := os.Getenv("GOOS"); v != "" {
		GOOS = v
	}
	if v := os.Getenv("GOARCH"); v != "" {
		GOARCH = v
	}
	if GOARCH != "amd64" && GOARCH != "arm64" && GOARCH != "wasm" {
		return nil, errors.New("gdnext requires an amd64, wasm, or arm64 GOARCH")
	}
	build_godot := func() error { return nil }
	if runtime.GOOS == "linux" {
		version, err := tooling.ListDynamicDependencies.CombinedOutput("--version")
		if strings.HasPrefix(version, "musl") {
			if testing {
				build_godot = muslTestClosure(GOOS, extraArgs)
			} else {
				build_godot = muslBuildClosure()
			}
			if os.Getenv("GOOS") == "" {
				GOOS = "musl"
			}
		} else if err != nil {
			return nil, xray.New(err)
		}
	}
	plat := platform.For(GOOS)
	if v := os.Getenv("GOOS"); v != "" {
		GOOS = v
	}
	if v := os.Getenv("GOARCH"); v != "" {
		GOARCH = v
	}
	if GOOS != "js" {
		if err := os.Setenv("CGO_ENABLED", "1"); err != nil {
			return nil, xray.New(err)
		}
	}
	if GOOS == "windows" && os.Getenv("CC") == "" {
		zig, err := tooling.Zig.Lookup()
		if err != nil {
			return nil, xray.New(err)
		}
		if err := os.Setenv("CC", zig+" cc"); err != nil {
			return nil, xray.New(err)
		}
	} else if zig, _ := exec.LookPath("zig"); zig != "" && os.Getenv("CC") == "" {
		if runtime.GOOS == "darwin" {
			if err := os.Setenv("CC", "clang"); err != nil {
				return nil, xray.New(err)
			}
		} else {
			if err := os.Setenv("CC", "zig cc"); err != nil {
				return nil, xray.New(err)
			}
		}
	}
	if err := project.Setup(build_godot); err != nil {
		return nil, err
	}
	if project.IncludesGo {
		if err := docgen.Process(project.Directory); err != nil {
			return nil, xray.New(err)
		}
	}
	_ = GOARCH
	return plat, nil
}

func muslBuildClosure() func() error {
	return func() error {
		GOARCH := os.Getenv("GOARCH")
		os.Setenv("GOARCH", runtime.GOARCH)
		defer os.Setenv("GOARCH", GOARCH)
		current, err := os.Getwd()
		if err != nil {
			return xray.New(err)
		}
		os.Chdir(project.Directory)
		defer os.Chdir(current)
		return builder.Musl{}.Build("-gcflags=graphics.gd/classdb/...=-N -l")
	}
}

func muslTestClosure(GOOS string, testArgsAll []string) func() error {
	return func() error {
		current, err := os.Getwd()
		if err != nil {
			return xray.New(err)
		}
		os.Chdir(project.Directory)
		defer os.Chdir(current)
		var faster_compile = []string{"-gcflags=graphics.gd/classdb/...=-N -l"}
		if slices.Contains(testArgsAll, "-bench") {
			faster_compile = nil
		}
		args := testArgsAll
		if GOOS != "musl" && GOOS != "" {
			args = []string{"-test.skip", "."}
		}
		return builder.Musl{}.Test(append(faster_compile, TestArgs(args)...)...)
	}
}
