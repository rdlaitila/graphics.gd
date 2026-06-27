package builder

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"graphics.gd/cmd/gdnext/internal/project"
	"graphics.gd/cmd/gdnext/internal/tooling"
	"graphics.gd/product"

	"github.com/samber/do/v2"
	"runtime.link/api/xray"
)

// Linux drives gdextension builds (and runs/tests) for the linux
// target GOOSes. The musl-static linker mode is in [Musl] instead.
type Linux struct {
	BuildEnv    product.BuildEnv `do:""`
	ToolCatalog tooling.Catalog  `do:""`
}

// NewLinux constructs the Linux builder via DI.
func NewLinux(di do.Injector) (*Linux, error) {
	return do.InvokeStruct[*Linux](di)
}

func (t *Linux) Build(args ...string) error {
	if !project.IncludesGo {
		return nil
	}
	var glibc bool
	if t.BuildEnv.Host.GOOS != product.GOOSLinux || t.BuildEnv.Host.GOARCH != t.BuildEnv.Target.GOARCH {
		zig, err := t.ToolCatalog.Zig.Lookup()
		if err != nil {
			return xray.New(err)
		}
		switch t.BuildEnv.Target.GOARCH {
		case product.GOARCHAmd64:
			if err := os.Setenv(product.EnvCC, zig+" cc -target x86_64-linux-gnu"); err != nil {
				return xray.New(err)
			}
		case product.GOARCHArm64:
			if err := os.Setenv(product.EnvCC, zig+" cc -target aarch64-linux-gnu"); err != nil {
				return xray.New(err)
			}
		default:
			return fmt.Errorf("gd build: cannot cross-compile linux %v on %v", t.BuildEnv.Target.GOARCH, t.BuildEnv.Host.GOOS)
		}
		glibc = true
	} else {
		version, _ := t.ToolCatalog.ListDynamicDependencies.CombinedOutput("--version")
		glibc = !strings.HasPrefix(strings.TrimSpace(version), "musl")
	}
	if glibc {
		// Force-link libgcc_s.so.1 so the c-shared extension's _Unwind_* refs
		// resolve at dlopen time on glibc >= 2.34 (Fedora/Nobara), where the
		// loader stopped pre-loading libgcc eagerly. --no-as-needed defeats
		// the linker's pruning of libraries with no pending undef ref, then
		// --as-needed restores the default for following libs. Skipped on
		// musl (statically links its own unwinder); harmless under zig (its
		// toolchain links unwinder too) but the as-needed pair is preferred
		// over --push-state since zig rejects the latter.
		const forceUnwinder = "-Wl,--no-as-needed -lgcc_s -Wl,--as-needed"
		ldflags := strings.TrimSpace(os.Getenv("CGO_LDFLAGS") + " " + forceUnwinder)
		if err := os.Setenv("CGO_LDFLAGS", ldflags); err != nil {
			return xray.New(err)
		}
	}
	return t.ToolCatalog.Go.Action("build", args, "-buildmode=c-shared", "-o", filepath.Join(project.GraphicsDirectory, fmt.Sprintf("linux_%v.so", t.BuildEnv.Target.GOARCH)))
}

func (t *Linux) BuildMain(args ...string) error {
	if err := t.Build(args...); err != nil {
		return xray.New(err)
	}
	var export []string
	switch t.BuildEnv.Target.GOARCH {
	case product.GOARCHAmd64:
		export = []string{"--headless", "--export-release", "Linux x86_64"}
	case product.GOARCHArm64:
		export = []string{"--headless", "--export-release", "Linux arm64"}
	default:
		return fmt.Errorf("gd export: cannot export linux %v", t.BuildEnv.Target.GOARCH)
	}
	if err := os.Chdir(project.GraphicsDirectory); err != nil {
		return xray.New(err)
	}
	if err := t.ToolCatalog.Godot.Exec(export...); err != nil {
		return xray.New(err)
	}
	return nil
}

func (t *Linux) Run(args ...string) error {
	if t.BuildEnv.Host.GOOS != product.GOOSLinux || t.BuildEnv.Host.GOARCH != t.BuildEnv.Target.GOARCH {
		return fmt.Errorf("gd run: cannot run linux/%v executable on %s", t.BuildEnv.Target.GOARCH, t.BuildEnv.Host.Tuple())
	}
	if err := t.Build(args...); err != nil {
		return xray.New(err)
	}
	if err := os.Chdir(project.GraphicsDirectory); err != nil {
		return xray.New(err)
	}
	return t.ToolCatalog.Godot.Exec(args...)
}

func (t *Linux) Test(args ...string) error {
	if t.BuildEnv.Host.GOOS != product.GOOSLinux || t.BuildEnv.Host.GOARCH != t.BuildEnv.Target.GOARCH {
		return fmt.Errorf("gd test: cannot run linux/%v tests on %s", t.BuildEnv.Target.GOARCH, t.BuildEnv.Host.Tuple())
	}
	if err := t.ToolCatalog.Go.Action("test", args, "-c", "-buildmode=c-shared", "-o", filepath.Join(project.GraphicsDirectory, fmt.Sprintf("linux_%v.so", t.BuildEnv.Target.GOARCH))); err != nil {
		return xray.New(err)
	}
	if err := os.Chdir(project.GraphicsDirectory); err != nil {
		return xray.New(err)
	}
	args = append(args, "--headless")
	return t.ToolCatalog.Godot.Exec(args...)
}
