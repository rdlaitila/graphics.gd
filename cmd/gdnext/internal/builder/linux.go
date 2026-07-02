package builder

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"graphics.gd/cmd/gdnext/internal/project"
	"graphics.gd/cmd/gdnext/internal/tooling"
	"graphics.gd/product"

	"github.com/samber/do/v2"
	"runtime.link/api/xray"
)

//go:embed bundled/musl
var musl_sdk embed.FS

// built_musl guards against double-runs of the libgodot linux path
// within a single process (BuildMain calls Build; Test may re-enter).
var built_musl bool

// Linux drives every linux/* target: gdextension c-shared builds and
// the single-file libgodot variant (routed here on LinkMode.LibGodot).
// Handles both glibc (via Godot's buildroot SDK) and musl (via the
// dlopen shim) libc variants; the choice is threaded through
// GDNEXT_LIBGODOT_LIBC. The mallocng patch, gdextension-scrub, and
// runtime overlay helpers below are shared across both.
type Linux struct {
	BuildEnv    product.BuildEnv `do:""`
	ToolCatalog tooling.Catalog  `do:""`
	lib         string
	out         string
	godot       *tooling.Tool
}

// NewLinux constructs the Linux builder via DI.
func NewLinux(di do.Injector) (*Linux, error) {
	return do.InvokeStruct[*Linux](di)
}

// godotTool returns the *Tool subsequent godot invocations should
// use: the freshly-built single-file editor set by libgodotBuild /
// libgodotTest, otherwise the catalog default.
func (t *Linux) godotTool() *tooling.Tool {
	if t.godot != nil {
		return t.godot
	}
	return t.ToolCatalog.Godot
}

// useGodotAt records path as the godot binary every later call should
// run. Stores a shallow copy of the catalog's *Tool so the catalog
// itself stays unmodified.
func (t *Linux) useGodotAt(path string) {
	godot := *t.ToolCatalog.Godot
	godot.Path = path
	t.godot = &godot
}

func (t *Linux) Build(args ...string) error {
	if t.BuildEnv.Target.LinkMode.Has(product.LibGodot) {
		return t.libgodotBuild(args...)
	}
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
	if t.BuildEnv.Target.LinkMode.Has(product.LibGodot) {
		return t.libgodotBuildMain(args...)
	}
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
	return t.godotTool().Exec(args...)
}

func (t *Linux) Test(args ...string) error {
	if t.BuildEnv.Target.LinkMode.Has(product.LibGodot) {
		return t.libgodotTest(args...)
	}
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

// libgodotBuild produces the single-file linux binary that statically
// links libgodot + the c-archive Go build together. Dispatches to the
// glibc (Godot buildroot SDK) or musl (zig + dlopen shim) helper based
// on GDNEXT_LIBGODOT_LIBC (default: glibc). The glibc path emits a
// normal dynamic-libc binary that runs on any distro with glibc >= 2.28;
// musl keeps the older opt-in static-musl + dlopen-shim path.
func (t *Linux) libgodotBuild(args ...string) (err error) {
	switch libgodotLibC() {
	case product.LibCMusl:
		return t.libgodotBuildMusl(args...)
	default:
		return t.libgodotBuildGlibc(args...)
	}
}

// libgodotLibC returns the caller-requested libc variant for the
// libgodot linker path. Reads GDNEXT_LIBGODOT_LIBC; empty / unknown
// values fall through to glibc (the default).
func libgodotLibC() string {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(product.EnvLibGodotLibC)))
	if v == product.LibCMusl {
		return product.LibCMusl
	}
	return product.LibCGlibc
}

// libgodotArtefactPath resolves the installed libgodot .a for the
// given (goarch, editor) under the caller-requested libc. Returns
// `<host.GDLibPath>/<recipe.InstallName>` — the same location
// `gdnext libgodot install` writes to. Errors when no recipe matches
// the tuple or the file isn't on disk (run `gdnext libgodot install`).
func (t *Linux) libgodotArtefactPath(goarch string, editor bool) (string, error) {
	recipe, ok := product.FindLibGodotRecipeLibC(product.GOOSLinux, goarch, editor, libgodotLibC())
	if !ok {
		return "", fmt.Errorf("libgodot: no recipe for linux/%s editor=%v libc=%s", goarch, editor, libgodotLibC())
	}
	path := filepath.Join(t.BuildEnv.Host.GDLibPath, recipe.InstallName)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("libgodot: %s not installed (run `gdnext libgodot install --libc=%s%s`): %w",
			recipe.InstallName, recipe.LibC, editorFlag(editor), err)
	}
	return path, nil
}

func editorFlag(editor bool) string {
	if editor {
		return " --editor"
	}
	return ""
}

// libgodotBuildMusl produces the single-file linux binary that statically
// links libgodot + the c-archive Go build together. Uses zig cc with
// -target x86_64-linux-musl -static under the hood; that's a build
// implementation detail, not a target-audience distinction (the
// resulting binary is meant to run on any Linux with a display server
// socket, not musl-only distros).
func (t *Linux) libgodotBuildMusl(args ...string) (err error) {
	env := t.BuildEnv
	tools := t.ToolCatalog
	os.Remove(filepath.Join(project.GraphicsDirectory, "library.gdextension"))
	goos := os.Getenv(product.EnvGOOS)
	os.Setenv(product.EnvGOOS, product.GOOSLinux)
	defer os.Setenv(product.EnvGOOS, goos)
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
		libgodot, err := t.libgodotArtefactPath(GOARCH, true)
		if err != nil {
			return xray.New(err)
		}
		t.lib = libgodot
	}
	if t.out == "" {
		t.out = filepath.Join(project.GraphicsDirectory, "linux_"+GOARCH+".libgodot.editor")
		if env.Host.GOOS == product.GOOSLinux {
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
	if err := patchMuslMalloc(env); err != nil {
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
	case product.GOARCHAmd64:
		target = "x86_64-linux-musl"
		if err := os.Setenv(product.EnvCC, zig+" cc -target x86_64-linux-musl -static"); err != nil {
			return xray.New(err)
		}
	case product.GOARCHArm64:
		target = "aarch64-linux-musl"
		if err := os.Setenv(product.EnvCC, zig+" cc -target aarch64-linux-musl -static"); err != nil {
			return xray.New(err)
		}
	default:
		return fmt.Errorf("gd build: cannot cross-compile linux/libgodot %v on %s", GOARCH, env.Host.Tuple())
	}
	libgo := filepath.Join(project.GraphicsDirectory, fmt.Sprintf("linux_%v.libgodot.a", GOARCH))
	if err := tools.Go.Action("build", args, "-tags", "archive", "-buildmode=c-archive", "-overlay="+overlay, "-o", libgo); err != nil {
		return xray.New(err)
	}
	pckStub := filepath.Join(project.GraphicsDirectory, "pck_section.c")
	if err := os.WriteFile(pckStub, []byte(`static const char pck_dummy[8] __attribute__((section("pck"), used)) = {0};
`), 0o644); err != nil {
		return xray.New(err)
	}
	zigArgs := []string{"cc", "-target", target, pckStub, "-Wl,--start-group", t.lib, libgo}
	cgoLDFLAGS, err := tools.Go.Output("list", "-tags", "archive", "-deps", "-f", "{{range .CgoLDFLAGS}}{{println .}}{{end}}", ".")
	if err != nil {
		return xray.New(err)
	}
	for _, flag := range strings.Split(cgoLDFLAGS, "\n") {
		if flag = strings.TrimSpace(flag); flag != "" {
			zigArgs = append(zigArgs, flag)
		}
	}
	zigArgs = append(zigArgs, "-Wl,--end-group", "-lc++", "-o", t.out)
	if err := tools.Zig.Exec(zigArgs...); err != nil {
		return xray.New(err)
	}
	return nil
}

// libgodotBuildGlibc produces the single-file linux binary using
// Godot's buildroot SDK (pinned gcc 13.2.0 + glibc 2.28), links
// libgodot.a + the Go c-archive with -static-libstdc++ so the
// resulting binary depends only on glibc/pthread/dl/m/rt — the same
// baseline every mainstream distro ships since 2018. Skips the musl
// overlay, mallocng patch, and dlopen shim entirely; those are
// musl-only concerns.
func (t *Linux) libgodotBuildGlibc(args ...string) (err error) {
	env := t.BuildEnv
	tools := t.ToolCatalog
	os.Remove(filepath.Join(project.GraphicsDirectory, "library.gdextension"))
	if built_musl {
		return nil
	}
	defer func() { built_musl = true }()
	if !project.IncludesGo {
		return nil
	}
	GOARCH := env.Target.GOARCH
	sdk, err := tools.GodotBuildroot.Lookup()
	if err != nil {
		return xray.New(err)
	}
	if err := ensureBuildrootRelocated(sdk); err != nil {
		return xray.New(err)
	}
	var triple string
	switch GOARCH {
	case product.GOARCHAmd64:
		triple = "x86_64-godot-linux-gnu"
	case product.GOARCHArm64:
		triple = "aarch64-godot-linux-gnu"
	default:
		return fmt.Errorf("gd build: cannot cross-compile linux/libgodot %v on %s", GOARCH, env.Host.Tuple())
	}
	gcc := filepath.Join(sdk, "bin", triple+"-gcc")
	if err := os.Setenv(product.EnvCC, gcc); err != nil {
		return xray.New(err)
	}
	if t.lib == "" {
		libgodot, err := t.libgodotArtefactPath(GOARCH, true)
		if err != nil {
			return xray.New(err)
		}
		t.lib = libgodot
	}
	if t.out == "" {
		t.out = filepath.Join(project.GraphicsDirectory, "linux_"+GOARCH+".libgodot.editor")
		defer func() {
			if err == nil {
				t.useGodotAt(t.out)
			}
		}()
	}
	libgo := filepath.Join(project.GraphicsDirectory, fmt.Sprintf("linux_%v.libgodot.a", GOARCH))
	if err := tools.Go.Action("build", args, "-tags", "archive", "-buildmode=c-archive", "-o", libgo); err != nil {
		return xray.New(err)
	}
	pckStub := filepath.Join(project.GraphicsDirectory, "pck_section.c")
	if err := os.WriteFile(pckStub, []byte(`static const char pck_dummy[8] __attribute__((section("pck"), used)) = {0};
`), 0o644); err != nil {
		return xray.New(err)
	}
	// -static-libstdc++ / -static-libgcc so the binary doesn't need
	// libstdc++.so.<X>+ or libgcc_s.so.1 at runtime. libc/libpthread/
	// libdl/libm/librt stay dynamic (every glibc distro ships them).
	//
	// -l:libstdc++.a / -l:libgcc_eh.a live INSIDE --start-group /
	// --end-group so ld's multi-pass scan can resolve the tangle
	// between libgodot's per-module .a files and libstdc++ (Godot
	// pulls hundreds of `_M_create` / vtable refs out of embree
	// which need backtracking). `-l:libX.a` forces the static
	// archive by exact filename; plain `-lstdc++` would resolve to
	// the .so and defeat -static-libstdc++.
	linkArgs := []string{
		"-o", t.out,
		pckStub,
		"-static-libstdc++", "-static-libgcc",
		"-Wl,--start-group", t.lib, libgo, "-l:libstdc++.a", "-l:libgcc_eh.a",
	}
	cgoLDFLAGS, err := tools.Go.Output("list", "-tags", "archive", "-deps", "-f", "{{range .CgoLDFLAGS}}{{println .}}{{end}}", ".")
	if err != nil {
		return xray.New(err)
	}
	for _, flag := range strings.Split(cgoLDFLAGS, "\n") {
		if flag = strings.TrimSpace(flag); flag != "" {
			linkArgs = append(linkArgs, flag)
		}
	}
	linkArgs = append(linkArgs, "-Wl,--end-group",
		"-lpthread", "-ldl", "-lm", "-lrt")
	cmd := exec.Command(gcc, linkArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("libgodot glibc link: %w", err)
	}
	return nil
}

func (t *Linux) libgodotBuildMain(args ...string) error {
	env := t.BuildEnv
	os.Remove(filepath.Join(project.GraphicsDirectory, "library.gdextension"))
	GOARCH := env.Target.GOARCH
	var err error
	// Template filename mirrors godot's native `<platform>.<target>.<arch>`
	// convention plus the libc segment we're distinguishing on. The
	// canarybird export preset (and any downstream preset) picks the
	// binary by this exact name, so glibc and musl variants can coexist
	// in .godot/ without clobbering each other.
	godotArch := "x86_64"
	if GOARCH == product.GOARCHArm64 {
		godotArch = "arm64"
	}
	t.out = filepath.Join(project.GraphicsDirectory, ".godot",
		"godot.linux."+libgodotLibC()+".template_release."+godotArch)
	t.lib, err = t.libgodotArtefactPath(GOARCH, false)
	if err != nil {
		return xray.New(err)
	}
	built_musl = false
	if err := t.libgodotBuild(args...); err != nil {
		return xray.New(err)
	}
	var export []string
	var releaseDir string
	switch GOARCH {
	case product.GOARCHAmd64:
		export = []string{"--headless", "--export-release", "Linux x86_64 (libgodot)"}
		releaseDir = filepath.Join(project.ReleasesDirectory, "linux", "amd64")
	case product.GOARCHArm64:
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
	restoreExtensions, err := ignoreEnabledExtensions()
	if err != nil {
		return xray.New(err)
	}
	defer restoreExtensions()
	if err := t.godotTool().Exec(export...); err != nil {
		return xray.New(err)
	}
	return nil
}

func (t *Linux) libgodotTest(args ...string) error {
	env := t.BuildEnv
	tools := t.ToolCatalog
	if built_musl {
		return nil
	}
	defer func() {
		built_musl = true
	}()
	os.Remove(filepath.Join(project.GraphicsDirectory, "library.gdextension"))
	goos := os.Getenv(product.EnvGOOS)
	os.Setenv(product.EnvGOOS, product.GOOSLinux)
	defer os.Setenv(product.EnvGOOS, goos)
	GOARCH := env.Target.GOARCH
	if env.Host.GOOS != product.GOOSLinux || env.Host.GOARCH != GOARCH {
		return fmt.Errorf("gd test: cannot run linux/libgodot %v tests on %s", GOARCH, env.Host.Tuple())
	}
	zig, err := tools.Zig.Lookup()
	if err != nil {
		return xray.New(err)
	}
	muslLibPath := filepath.Join(env.Host.GDLibPath, "musl")
	if err := project.SetupFiles(musl_sdk, "bundled/musl", muslLibPath); err != nil {
		return xray.New(err)
	}
	if err := patchMuslMalloc(env); err != nil {
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
	case product.GOARCHAmd64:
		target = "x86_64-linux-musl"
		if err := os.Setenv(product.EnvCC, zig+" cc -target x86_64-linux-musl"); err != nil {
			return xray.New(err)
		}
	case product.GOARCHArm64:
		target = "aarch64-linux-musl"
		if err := os.Setenv(product.EnvCC, zig+" cc -target aarch64-linux-musl"); err != nil {
			return xray.New(err)
		}
	default:
		return fmt.Errorf("gd build: cannot cross-compile linux/libgodot %v on %s", GOARCH, env.Host.Tuple())
	}
	libgo := filepath.Join(project.GraphicsDirectory, fmt.Sprintf("linux_%v.libgodot.a", GOARCH))
	if err := tools.Go.Action("test", args, "-c", "-tags", "archive", "-buildmode=c-archive", "-overlay="+overlay, "-o", libgo); err != nil {
		return xray.New(err)
	}
	libgodot, err := t.libgodotArtefactPath(GOARCH, true)
	if err != nil {
		return xray.New(err)
	}
	editor := filepath.Join(project.GraphicsDirectory, "linux_"+GOARCH+".libgodot.editor")
	if err := tools.Zig.Exec("c++", "-target", target, libgo, libgodot, "-o", editor); err != nil {
		return xray.New(err)
	}
	t.useGodotAt(editor)
	if err := os.Chdir(project.GraphicsDirectory); err != nil {
		return xray.New(err)
	}
	args = append(args, "--headless")
	return t.godotTool().Exec(args...)
}

// patchMuslMalloc adjusts the mallocng initial-context struct so Go's
// runtime + musl-static play nicely under c-archive linking.
func patchMuslMalloc(env product.BuildEnv) error {
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

// ignoreEnabledExtensions disables every GDExtension for the duration of a musl export.
// A static musl binary can't dlopen a shared library (graphics.gd's loader borrows the
// host's dynamic loader), so any extension required at runtime must instead be statically
// linked and self-registered. Rename project *.gdextension files aside so Godot's import
// scan finds none; it then drops extension_list.cfg and the export bundles no extension
// loaders. The returned func restores everything for the editor and other-platform builds,
// so callers MUST defer it.
func ignoreEnabledExtensions() (restore func(), err error) {
	dir := project.GraphicsDirectory
	var found []string
	if err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if d.Name() == ".godot" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".gdextension") {
			found = append(found, path)
		}
		return nil
	}); err != nil {
		return func() {}, xray.New(err)
	}
	var disabled []string
	restore = func() {
		for _, path := range disabled {
			os.Rename(path+".disabled", path)
		}
	}
	for _, path := range found {
		if err := os.Rename(path, path+".disabled"); err != nil {
			restore()
			return func() {}, xray.New(err)
		}
		disabled = append(disabled, path)
	}
	extList := filepath.Join(dir, ".godot", "extension_list.cfg")
	if saved, e := os.ReadFile(extList); e == nil {
		os.Remove(extList)
		inner := restore
		restore = func() {
			inner()
			os.WriteFile(extList, saved, 0644)
		}
	}
	return restore, nil
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
