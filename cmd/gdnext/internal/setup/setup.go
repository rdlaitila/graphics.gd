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
	"graphics.gd/cmd/gdnext/internal/project"
	"graphics.gd/cmd/gdnext/internal/tooling"
	"graphics.gd/internal/docgen"
	"graphics.gd/product"

	"runtime.link/api/xray"
)

// ForBuild runs the full setup pipeline and returns the platform-specific
// Builder + the resolved (host, target) BuildEnv ready for
// Build/Run/BuildMain/Test. testing should be true only for the "test"
// subcommand (it switches musl auto-detect into Test mode). extraArgs are
// the user-supplied positional args after "gdnext <verb>"; they're passed
// into the musl test closure so it can honour -bench.
//
// The flag→env bridge (--goos / --goarch / --cc / --cgo / --gdpath) is
// handled by cli.PromoteFlagsToEnv on the root command's Before hook, so
// by the time ForBuild runs every legacy os.Getenv read sees the right
// values.
func ForBuild(env product.BuildEnv, testing bool, extraArgs []string) (builder.Builder, error) {
	if env.Target.GOARCH != product.GOARCHAmd64 &&
		env.Target.GOARCH != product.GOARCHArm64 &&
		env.Target.GOARCH != product.GOARCHWasm {
		return nil, errors.New("gdnext requires an amd64, wasm, or arm64 GOARCH")
	}
	build_godot := func() error { return nil }
	if env.Host.GOOS == product.GOOSLinux {
		version, err := tooling.ListDynamicDependencies.CombinedOutput("--version")
		if strings.HasPrefix(version, product.GOOSMusl) {
			if testing {
				build_godot = muslTestClosure(env, extraArgs)
			} else {
				build_godot = muslBuildClosure(env)
			}
			// Musl host = we're statically linking against libgodot
			// regardless of what the user asked for, so flip the
			// LinkMode bit unless the user explicitly chose one.
			if os.Getenv("GOLINK") == "" {
				env.Target.LinkMode |= product.LibGodot
			}
		} else if err != nil {
			return nil, xray.New(err)
		}
	}
	build, err := builder.For(env)
	if err != nil {
		return nil, xray.New(err)
	}
	if env.Target.GOOS != product.GOOSJS {
		if err := os.Setenv("CGO_ENABLED", "1"); err != nil {
			return nil, xray.New(err)
		}
	}
	if env.Target.GOOS == product.GOOSWindows && os.Getenv("CC") == "" {
		zig, err := tooling.Zig.Lookup()
		if err != nil {
			return nil, xray.New(err)
		}
		if err := os.Setenv("CC", zig+" cc"); err != nil {
			return nil, xray.New(err)
		}
	} else if zig, _ := exec.LookPath("zig"); zig != "" && os.Getenv("CC") == "" {
		if env.Host.GOOS == product.GOOSDarwin {
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
		return nil, xray.New(err)
	}
	if project.IncludesGo {
		if err := docgen.Process(project.Directory); err != nil {
			return nil, xray.New(err)
		}
	}
	return build, nil
}

func muslBuildClosure(env product.BuildEnv) func() error {
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
		return builder.Musl{}.Build(env, "-gcflags=graphics.gd/classdb/...=-N -l")
	}
}

func muslTestClosure(env product.BuildEnv, testArgsAll []string) func() error {
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
		if !env.Target.LinkMode.Has(product.LibGodot) {
			args = []string{"-test.skip", "."}
		}
		return builder.Musl{}.Test(env, append(faster_compile, TestArgs(args)...)...)
	}
}
