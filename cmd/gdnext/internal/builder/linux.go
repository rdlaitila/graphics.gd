package builder

import (
	"fmt"
	"os"
	"path/filepath"

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
	if t.BuildEnv.Host.GOOS != product.GOOSLinux || t.BuildEnv.Host.GOARCH != t.BuildEnv.Target.GOARCH {
		zig, err := t.ToolCatalog.Zig.Lookup()
		if err != nil {
			return xray.New(err)
		}
		switch t.BuildEnv.Target.GOARCH {
		case product.GOARCHAmd64:
			if err := os.Setenv("CC", zig+" cc -target x86_64-linux-gnu"); err != nil {
				return xray.New(err)
			}
		case product.GOARCHArm64:
			if err := os.Setenv("CC", zig+" cc -target aarch64-linux-gnu"); err != nil {
				return xray.New(err)
			}
		default:
			return fmt.Errorf("gd build: cannot cross-compile linux %v on %v", t.BuildEnv.Target.GOARCH, t.BuildEnv.Host.GOOS)
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
