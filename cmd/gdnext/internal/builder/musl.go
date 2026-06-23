package builder

import (
	"bytes"
	"embed"
	"encoding/json"
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

var (
	//go:embed bundled/musl
	musl_sdk embed.FS
)

var built_musl bool

// Musl drives static-musl Linux builds (libgodot mode).
type Musl struct {
	BuildEnv    product.BuildEnv `do:""`
	ToolCatalog tooling.Catalog  `do:""`
	lib         string
	out         string
	godot       *tooling.Tool // override; nil means "use catalog default"
}

// NewMusl constructs the Musl builder via DI.
func NewMusl(di do.Injector) (*Musl, error) {
	return do.InvokeStruct[*Musl](di)
}

// godotTool returns the *Tool subsequent godot invocations should
// use: the freshly-built musl-static editor when [Musl.Build] /
// [Musl.Test] set it, otherwise the catalog default.
func (t *Musl) godotTool() *tooling.Tool {
	if t.godot != nil {
		return t.godot
	}
	return t.ToolCatalog.Godot
}

// useGodotAt records path as the godot binary every later call should
// run. Stores a shallow copy of the catalog's *Tool so the catalog
// itself stays unmodified.
func (t *Musl) useGodotAt(path string) {
	godot := *t.ToolCatalog.Godot
	godot.Path = path
	t.godot = &godot
}

func (t *Musl) Build(args ...string) (err error) {
	env := t.BuildEnv
	tools := t.ToolCatalog
	os.Remove(filepath.Join(project.GraphicsDirectory, "library.gdextension"))
	goos := os.Getenv("GOOS")
	os.Setenv("GOOS", "linux")
	defer os.Setenv("GOOS", goos)
	if built_musl {
		return nil
	}
	defer func() {
		built_musl = true
	}()
	if !project.IncludesGo {
		return nil
	}
	GOARCH := env.Target.GOARCH
	zig, err := tools.Zig.Lookup()
	if err != nil {
		return xray.New(err)
	}
	if t.lib == "" {
		libgodot, err := tools.LibGodotEditor.LookupPlatform("musl", GOARCH)
		if err != nil {
			return xray.New(err)
		}
		t.lib = libgodot
	}
	if t.out == "" {
		t.out = filepath.Join(project.GraphicsDirectory, "musl_"+GOARCH+".editor")
		if env.Host.GOOS == "linux" {
			version, _ := tools.ListDynamicDependencies.CombinedOutput("--version")
			if strings.HasPrefix(version, "musl") {
				defer func() {
					if err == nil {
						t.useGodotAt(t.out)
					}
				}()
			}
		}
	}
	muslLibPath := filepath.Join(env.Host.GDLibPath, "musl")
	if err := project.SetupFiles(musl_sdk, "bundled/musl", muslLibPath); err != nil {
		return xray.New(err)
	}
	if err := t.patch(env); err != nil {
		return xray.New(err)
	}
	GOROOT, err := tools.Go.Output("env", "GOROOT")
	if err != nil {
		return xray.New(err)
	}
	overlay, err := writeMuslOverlay(env, GOROOT)
	if err != nil {
		return xray.New(err)
	}
	var target string
	switch GOARCH {
	case "amd64":
		target = "x86_64-linux-musl"
		if err := os.Setenv("CC", zig+" cc -target x86_64-linux-musl -static"); err != nil {
			return xray.New(err)
		}
	case "arm64":
		target = "aarch64-linux-musl"
		if err := os.Setenv("CC", zig+" cc -target aarch64-linux-musl -static"); err != nil {
			return xray.New(err)
		}
	default:
		return fmt.Errorf("gd build: cannot cross-compile linux %v on %s", GOARCH, env.Host.Tuple())
	}
	libgo := filepath.Join(project.GraphicsDirectory, fmt.Sprintf("musl_%v.a", GOARCH))
	if err := tools.Go.Action("build", args, "-tags", "musl", "-buildmode=c-archive", "-overlay="+overlay, "-o", libgo); err != nil {
		return xray.New(err)
	}
	if err := tools.Zig.Exec("cc", "-target", target, "-lc++", t.lib, libgo, "-o", t.out); err != nil {
		return xray.New(err)
	}
	return nil
}

func (t *Musl) patch(env product.BuildEnv) error {
	musl_malloc := filepath.Join(env.Host.GDBinPath, "lib", "libc", "musl", "src", "malloc", "mallocng", "malloc.c")
	file, err := os.ReadFile(musl_malloc)
	if err != nil {
		return xray.New(err)
	}
	file = bytes.Replace(file,
		[]byte(`struct malloc_context ctx = { 0 };`),
		[]byte(`struct malloc_context ctx = { .brk = -1 };`), 1)
	if err := os.WriteFile(musl_malloc, file, 0644); err != nil {
		return xray.New(err)
	}
	return nil
}

func (t *Musl) BuildMain(args ...string) error {
	env := t.BuildEnv
	tools := t.ToolCatalog
	os.Remove(filepath.Join(project.GraphicsDirectory, "library.gdextension"))
	GOARCH := env.Target.GOARCH
	var err error
	t.out = filepath.Join(project.GraphicsDirectory, ".godot", "godot.musl.template_release.x86_64")
	t.lib, err = tools.LibGodot.LookupPlatform("musl", GOARCH)
	if err != nil {
		return xray.New(err)
	}
	built_musl = false
	if err := t.Build(args...); err != nil {
		return xray.New(err)
	}
	var export []string
	var releaseDir string
	switch GOARCH {
	case "amd64":
		export = []string{"--headless", "--export-release", "Linux x86_64 (libgodot)"}
		releaseDir = filepath.Join(project.ReleasesDirectory, "linux", "amd64")
	case "arm64":
		export = []string{"--headless", "--export-release", "Linux arm64 (libgodot)"}
		releaseDir = filepath.Join(project.ReleasesDirectory, "linux", "arm64")
	default:
		return fmt.Errorf("gd export: cannot export libgodot %v", GOARCH)
	}
	if err := os.MkdirAll(releaseDir, 0o755); err != nil {
		return xray.New(err)
	}
	if err := os.Chdir(project.GraphicsDirectory); err != nil {
		return xray.New(err)
	}
	if err := t.godotTool().Exec(export...); err != nil {
		return xray.New(err)
	}
	return nil
}

func (t *Musl) Run(args ...string) error {
	env := t.BuildEnv
	GOARCH := env.Target.GOARCH
	if env.Host.GOOS != "linux" || env.Host.GOARCH != GOARCH {
		return fmt.Errorf("gd run: cannot run linux/%v executable on %s", GOARCH, env.Host.Tuple())
	}
	if err := t.Build(args...); err != nil {
		return xray.New(err)
	}
	if err := os.Chdir(project.GraphicsDirectory); err != nil {
		return xray.New(err)
	}
	return t.godotTool().Exec(args...)
}

func (t *Musl) Test(args ...string) error {
	env := t.BuildEnv
	tools := t.ToolCatalog
	if built_musl {
		return nil
	}
	defer func() {
		built_musl = true
	}()
	os.Remove(filepath.Join(project.GraphicsDirectory, "library.gdextension"))
	goos := os.Getenv("GOOS")
	os.Setenv("GOOS", "linux")
	defer os.Setenv("GOOS", goos)
	GOARCH := env.Target.GOARCH
	if env.Host.GOOS != "linux" || env.Host.GOARCH != GOARCH {
		return fmt.Errorf("gd test: cannot run linux/%v tests on %s", GOARCH, env.Host.Tuple())
	}
	zig, err := tools.Zig.Lookup()
	if err != nil {
		return xray.New(err)
	}
	muslLibPath := filepath.Join(env.Host.GDLibPath, "musl")
	if err := project.SetupFiles(musl_sdk, "bundled/musl", muslLibPath); err != nil {
		return xray.New(err)
	}
	if err := t.patch(env); err != nil {
		return xray.New(err)
	}
	GOROOT, err := tools.Go.Output("env", "GOROOT")
	if err != nil {
		return xray.New(err)
	}
	overlay, err := writeMuslOverlay(env, GOROOT)
	if err != nil {
		return xray.New(err)
	}
	var target string
	switch GOARCH {
	case "amd64":
		target = "x86_64-linux-musl"
		if err := os.Setenv("CC", zig+" cc -target x86_64-linux-musl"); err != nil {
			return xray.New(err)
		}
	case "arm64":
		target = "aarch64-linux-musl"
		if err := os.Setenv("CC", zig+" cc -target aarch64-linux-musl"); err != nil {
			return xray.New(err)
		}
	default:
		return fmt.Errorf("gd build: cannot cross-compile linux %v on %s", GOARCH, env.Host.Tuple())
	}
	libgo := filepath.Join(project.GraphicsDirectory, fmt.Sprintf("musl_%v.a", GOARCH))
	if err := tools.Go.Action("test", args, "-c", "-tags", "musl", "-buildmode=c-archive", "-overlay="+overlay, "-o", libgo); err != nil {
		return xray.New(err)
	}
	libgodot, err := tools.LibGodotEditor.LookupPlatform("musl", GOARCH)
	if err != nil {
		return xray.New(err)
	}
	if err := tools.Zig.Exec("c++", "-target", target, libgo, libgodot, "-o", filepath.Join(project.GraphicsDirectory, "musl_"+GOARCH+".editor")); err != nil {
		return xray.New(err)
	}
	t.useGodotAt(filepath.Join(project.GraphicsDirectory, "musl_"+GOARCH+".editor"))

	if err := os.Chdir(project.GraphicsDirectory); err != nil {
		return xray.New(err)
	}
	args = append(args, "--headless")
	return t.godotTool().Exec(args...)
}

// writeMuslOverlay materialises the go-toolchain overlay JSON that
// remaps a couple of runtime source files to the musl-aware overlays
// shipped under GDLibPath/musl/. json.Marshal handles backslash-quoted
// paths on Windows; building the document by string concatenation
// would emit raw \h, \t, etc. inside Windows runtime paths and the go
// toolchain would reject the file with
//
//	parsing overlay JSON: invalid character ... in string escape code
func writeMuslOverlay(env product.BuildEnv, GOROOT string) (string, error) {
	overlay := filepath.Join(env.Host.GDLibPath, "musl.json")
	doc := struct {
		Replace map[string]string
	}{Replace: map[string]string{
		filepath.Join(GOROOT, "src", "runtime", "runtime1.go"): filepath.Join(env.Host.GDLibPath, "musl", "runtime1.go.overlay"),
		filepath.Join(GOROOT, "src", "runtime", "os_linux.go"): filepath.Join(env.Host.GDLibPath, "musl", "os_linux.go.overlay"),
	}}
	body, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(overlay, body, 0755); err != nil {
		return "", err
	}
	return overlay, nil
}
