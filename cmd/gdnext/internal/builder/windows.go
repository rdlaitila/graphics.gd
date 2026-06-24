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

// Windows drives gdextension builds (and runs/tests) for windows.
type Windows struct {
	BuildEnv    product.BuildEnv `do:""`
	ToolCatalog tooling.Catalog  `do:""`
}

// NewWindows constructs the Windows builder via DI.
func NewWindows(di do.Injector) (*Windows, error) {
	return do.InvokeStruct[*Windows](di)
}

func (t *Windows) Build(args ...string) error {
	if !project.IncludesGo {
		return nil
	}
	if t.BuildEnv.Host.GOOS != product.GOOSWindows || t.BuildEnv.Host.GOARCH != t.BuildEnv.Target.GOARCH {
		zig, err := t.ToolCatalog.Zig.Lookup()
		if err != nil {
			return xray.New(err)
		}
		switch t.BuildEnv.Target.GOARCH {
		case product.GOARCHAmd64:
			if err := os.Setenv("CC", zig+" cc -target x86_64-windows-gnu"); err != nil {
				return xray.New(err)
			}
		case product.GOARCHArm64:
			if err := os.Setenv("CC", zig+" cc -target aarch64-windows-gnu"); err != nil {
				return xray.New(err)
			}
		default:
			return fmt.Errorf("gd build: cannot cross-compile windows %v on %v", t.BuildEnv.Target.GOARCH, t.BuildEnv.Host.GOOS)
		}
	}
	return t.ToolCatalog.Go.Action("build", args, "-ldflags=-w -s", "-buildmode=c-shared", "-o", filepath.Join(project.GraphicsDirectory, fmt.Sprintf("windows_%v.dll", t.BuildEnv.Target.GOARCH)))
}

func (t *Windows) BuildMain(args ...string) error {
	if err := t.Build(args...); err != nil {
		return xray.New(err)
	}
	var export []string
	switch t.BuildEnv.Target.GOARCH {
	case product.GOARCHAmd64:
		export = []string{"--headless", "--export-release", "Windows x86_64"}
	case product.GOARCHArm64:
		export = []string{"--headless", "--export-release", "Windows arm64"}
	default:
		return fmt.Errorf("gd export: cannot export windows %v", t.BuildEnv.Target.GOARCH)
	}
	if err := os.Chdir(project.GraphicsDirectory); err != nil {
		return xray.New(err)
	}
	if err := t.ToolCatalog.Godot.Exec(export...); err != nil {
		return xray.New(err)
	}
	return nil
}

func (t *Windows) Run(args ...string) error {
	if t.BuildEnv.Host.GOOS != product.GOOSWindows || t.BuildEnv.Host.GOARCH != t.BuildEnv.Target.GOARCH {
		return fmt.Errorf("gd run: cannot run windows/%v executable on %s", t.BuildEnv.Target.GOARCH, t.BuildEnv.Host.Tuple())
	}
	if err := t.Build(args...); err != nil {
		return xray.New(err)
	}
	if err := os.Chdir(project.GraphicsDirectory); err != nil {
		return xray.New(err)
	}
	return t.ToolCatalog.Godot.Exec(args...)
}

func (t *Windows) Test(args ...string) error {
	if t.BuildEnv.Host.GOOS != product.GOOSWindows || t.BuildEnv.Host.GOARCH != t.BuildEnv.Target.GOARCH {
		return fmt.Errorf("gd test: cannot run windows/%v tests on %s", t.BuildEnv.Target.GOARCH, t.BuildEnv.Host.Tuple())
	}
	if err := t.ToolCatalog.Go.Action("test", args, "-c", "-buildmode=c-shared", "-o", filepath.Join(project.GraphicsDirectory, fmt.Sprintf("windows_%v.dll", t.BuildEnv.Target.GOARCH))); err != nil {
		return xray.New(err)
	}
	if err := os.Chdir(project.GraphicsDirectory); err != nil {
		return xray.New(err)
	}
	args = append(args, "--headless")
	return t.ToolCatalog.Godot.Exec(args...)
}
