package builder

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	lipo "github.com/konoui/lipo/cmd"

	"graphics.gd/cmd/gdnext/internal/project"
	"graphics.gd/cmd/gdnext/internal/tooling"
	"graphics.gd/product"

	"github.com/samber/do/v2"
	"runtime.link/api/xray"
)

var (
	// macos_sdk is a manually prepared "MacOS SDK" designed to support cross-compilation of Go code to arm64/amd64 targets using "zig cc".
	// the SDK was constructed by adding each undefined symbol / missing library observed from compilation errors on Linux GOOS=darwin as
	// .tbd files placed in the expected locations.
	//
	//go:embed bundled/macos
	macos_sdk embed.FS
)

// MacOS drives universal-binary builds for darwin targets.
type MacOS struct {
	BuildEnv    product.BuildEnv `do:""`
	ToolCatalog tooling.Catalog  `do:""`
}

func NewMacOS(di do.Injector) (*MacOS, error) {
	return do.InvokeStruct[*MacOS](di)
}

func (t *MacOS) Build(args ...string) error {
	if err := os.MkdirAll(filepath.Join(project.ReleasesDirectory, "darwin", "universal"), 0755); err != nil {
		return xray.New(err)
	}
	if !project.IncludesGo {
		return nil
	}
	if err := os.Setenv(product.EnvCGOEnabled, "1"); err != nil {
		return xray.New(err)
	}
	if t.BuildEnv.Host.GOOS != product.GOOSDarwin {
		zig, err := t.ToolCatalog.Zig.Lookup()
		if err != nil {
			return xray.New(err)
		}
		project.SetupFiles(macos_sdk, "bundled/macos", filepath.Join(project.ReleasesDirectory, "darwin", "sdk"))
		DARWIN_SDK, err := filepath.Abs(filepath.Join(project.ReleasesDirectory, "darwin", "sdk"))
		if err != nil {
			return xray.New(err)
		}
		if err := os.Setenv(product.EnvCC, zig+" cc -target aarch64-macos -F "+DARWIN_SDK+"/Frameworks -L"+DARWIN_SDK+"/lib -I"+DARWIN_SDK+"/include"); err != nil {
			return xray.New(err)
		}
	}
	if err := os.Setenv(product.EnvGOARCH, product.GOARCHArm64); err != nil {
		return xray.New(err)
	}
	if err := t.ToolCatalog.Go.Action("build", args, "-buildmode=c-shared", "-o", filepath.Join(project.GraphicsDirectory, "darwin_arm64.dylib")); err != nil {
		return xray.New(err)
	}
	if t.BuildEnv.Host.GOOS != product.GOOSDarwin {
		zig, err := t.ToolCatalog.Zig.Lookup()
		if err != nil {
			return xray.New(err)
		}
		DARWIN_SDK, err := filepath.Abs(filepath.Join(project.ReleasesDirectory, "darwin", "sdk"))
		if err != nil {
			return xray.New(err)
		}
		if err := os.Setenv(product.EnvCC, zig+" cc -target x86_64-macos -F "+DARWIN_SDK+"/Frameworks -L"+DARWIN_SDK+"/lib -I"+DARWIN_SDK+"/include"); err != nil {
			return xray.New(err)
		}
	}
	if err := os.Setenv(product.EnvGOARCH, product.GOARCHAmd64); err != nil {
		return xray.New(err)
	}
	if err := t.ToolCatalog.Go.Action("build", args, "-buildmode=c-shared", "-o", filepath.Join(project.GraphicsDirectory, "darwin_amd64.dylib")); err != nil {
		return xray.New(err)
	}
	err := lipo.Execute(os.Stdout, os.Stderr,
		[]string{
			"-create",
			filepath.Join(project.GraphicsDirectory, "darwin_amd64.dylib"),
			filepath.Join(project.GraphicsDirectory, "darwin_arm64.dylib"),
			"-output",
			filepath.Join(project.GraphicsDirectory, "darwin_universal.dylib"),
		},
	)
	if err != 0 {
		return errors.New("lipo execution failed")
	}
	return nil
}

func (t *MacOS) BuildMain(_ ...string) error {
	if err := t.Build(); err != nil {
		return xray.New(err)
	}
	if err := os.Chdir(project.GraphicsDirectory); err != nil {
		return xray.New(err)
	}
	if err := t.ToolCatalog.Godot.Exec("--headless", "--export-release", "macOS"); err != nil {
		return xray.New(err)
	}
	return nil
}

func (t *MacOS) Run(args ...string) error {
	if t.BuildEnv.Host.GOOS != product.GOOSDarwin {
		return fmt.Errorf("gd run: cannot run darwin/universal executable on %s", t.BuildEnv.Host.Tuple())
	}
	if err := t.ToolCatalog.Go.Action("build", args, "-buildmode=c-shared", "-o", filepath.Join(project.GraphicsDirectory, fmt.Sprintf("darwin_%v.dylib", t.BuildEnv.Host.GOARCH))); err != nil {
		return xray.New(err)
	}
	err := lipo.Execute(os.Stdout, os.Stderr,
		[]string{
			"-create",
			filepath.Join(project.GraphicsDirectory, "darwin_"+t.BuildEnv.Host.GOARCH+".dylib"),
			"-output",
			filepath.Join(project.GraphicsDirectory, "darwin_universal.dylib"),
		},
	)
	if err != 0 {
		return errors.New("lipo execution failed")
	}
	if err := os.Chdir(project.GraphicsDirectory); err != nil {
		return xray.New(err)
	}
	return t.ToolCatalog.Godot.Exec(args...)
}

func (t *MacOS) Test(args ...string) error {
	if t.BuildEnv.Host.GOOS != product.GOOSDarwin {
		return fmt.Errorf("gd test: cannot run darwin/universal tests on %s", t.BuildEnv.Host.Tuple())
	}
	if err := t.ToolCatalog.Go.Action("test", args, "-c", "-buildmode=c-shared", "-o", filepath.Join(project.GraphicsDirectory, fmt.Sprintf("darwin_%v.dylib", t.BuildEnv.Host.GOARCH))); err != nil {
		return xray.New(err)
	}
	err := lipo.Execute(os.Stdout, os.Stderr,
		[]string{
			"-create",
			filepath.Join(project.GraphicsDirectory, "darwin_"+t.BuildEnv.Host.GOARCH+".dylib"),
			"-output",
			filepath.Join(project.GraphicsDirectory, "darwin_universal.dylib"),
		},
	)
	if err != 0 {
		return errors.New("lipo execution failed")
	}
	if err := os.Chdir(project.GraphicsDirectory); err != nil {
		return xray.New(err)
	}
	args = append(args, "--headless")
	return t.ToolCatalog.Godot.Exec(args...)
}
