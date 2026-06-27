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

	"github.com/samber/do/v2"
	"graphics.gd/cmd/gdnext/internal/builder"
	"graphics.gd/cmd/gdnext/internal/project"
	"graphics.gd/cmd/gdnext/internal/tooling"
	"graphics.gd/internal/docgen"
	"graphics.gd/product"

	"runtime.link/api/xray"
)

// Provides is the package-level provider set for the setup package
var Provides = do.Package(
	do.Lazy(NewBuildEnv),
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
func ForBuild(di do.Injector, testing bool, extraArgs []string) (builder.Builder, error) {
	env := do.MustInvoke[product.BuildEnv](di)
	tools := do.MustInvoke[tooling.Catalog](di)
	if env.Target.GOARCH != product.GOARCHAmd64 &&
		env.Target.GOARCH != product.GOARCHArm64 &&
		env.Target.GOARCH != product.GOARCHWasm {
		return nil, errors.New("gdnext requires an amd64, wasm, or arm64 GOARCH")
	}
	build_godot := func() error { return nil }
	if env.Host.GOOS == product.GOOSLinux {
		version, err := tools.ListDynamicDependencies.CombinedOutput("--version")
		if strings.HasPrefix(version, product.GOOSMusl) {
			if testing {
				build_godot = muslTestClosure(di, env, extraArgs)
			} else {
				build_godot = muslBuildClosure(di)
			}
			// Musl host always statically links libgodot; honour the
			// user's --link only if they set it. The local env mutation
			// only affects which builder builder.For picks below; the
			// builder reads its own DI-injected env (which is unchanged)
			// for everything else.
			if os.Getenv(product.EnvGOLink) == "" {
				env.Target.LinkMode |= product.LibGodot
			}
		} else if err != nil {
			return nil, xray.New(err)
		}
	}
	build, err := builder.For(di, env)
	if err != nil {
		return nil, xray.New(err)
	}
	if env.Target.GOOS != product.GOOSJS {
		if err := os.Setenv(product.EnvCGOEnabled, "1"); err != nil {
			return nil, xray.New(err)
		}
	}
	if env.Target.GOOS == product.GOOSWindows && os.Getenv(product.EnvCC) == "" {
		zig, err := tools.Zig.Lookup()
		if err != nil {
			return nil, xray.New(err)
		}
		if err := os.Setenv(product.EnvCC, zig+" cc"); err != nil {
			return nil, xray.New(err)
		}
	} else if zig, _ := exec.LookPath("zig"); zig != "" && os.Getenv(product.EnvCC) == "" {
		if env.Host.GOOS == product.GOOSDarwin {
			if err := os.Setenv(product.EnvCC, "clang"); err != nil {
				return nil, xray.New(err)
			}
		} else {
			if err := os.Setenv(product.EnvCC, "zig cc"); err != nil {
				return nil, xray.New(err)
			}
		}
	}
	if err := project.Setup(tools, build_godot); err != nil {
		return nil, xray.New(err)
	}
	if project.IncludesGo {
		if err := docgen.Process(project.Directory); err != nil {
			return nil, xray.New(err)
		}
	}
	return build, nil
}

func muslBuildClosure(di do.Injector) func() error {
	return func() error {
		GOARCH := os.Getenv(product.EnvGOARCH)
		os.Setenv(product.EnvGOARCH, runtime.GOARCH)
		defer os.Setenv(product.EnvGOARCH, GOARCH)
		current, err := os.Getwd()
		if err != nil {
			return xray.New(err)
		}
		os.Chdir(project.Directory)
		defer os.Chdir(current)
		musl, err := do.Invoke[*builder.Musl](di)
		if err != nil {
			return xray.New(err)
		}
		return musl.Build("-gcflags=graphics.gd/classdb/...=-N -l")
	}
}

func muslTestClosure(di do.Injector, env product.BuildEnv, testArgsAll []string) func() error {
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
		musl, err := do.Invoke[*builder.Musl](di)
		if err != nil {
			return xray.New(err)
		}
		return musl.Test(append(faster_compile, TestArgs(args)...)...)
	}
}
