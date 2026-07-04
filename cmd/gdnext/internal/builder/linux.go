package builder

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"graphics.gd/cmd/gdnext/internal/project"
	"graphics.gd/cmd/gdnext/internal/shared"
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
// Both libc variants (glibc and musl) use zig-cc for cross-compilation;
// musl adds -static, the mallocng patch, the musl runtime overlay,
// and bakes in the dlopen shim so the resulting binary borrows the
// system ld.so at runtime. The choice is threaded through
// BuildEnv.Target.LibC (default glibc; --libc=musl / GDNEXT_LIBGODOT_LIBC
// opts in).
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
		if err := setGoCrossEnv(product.GOOSLinux, t.BuildEnv.Target.GOARCH); err != nil {
			return xray.New(err)
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

// libgodotBuild produces the single-file linux binary that statically links libgodot + the c-archive Go build together.
// Dispatches to the glibc (zig-cc pinned to glibc 2.28) or musl (zig + dlopen shim) helper based on BuildEnv.Target.LibC.
func (t *Linux) libgodotBuild(args ...string) (err error) {
	switch t.BuildEnv.Target.LibC {
	case product.LibCMusl:
		return t.libgodotBuildMusl(args...)
	default:
		return t.libgodotBuildGlibc(args...)
	}
}

// libgodotArtefactPath resolves the installed libgodot .a for the given (goarch, editor) under BuildEnv.Target.LibC.
// Downloads the published artefact via ToolCatalog when it isn't already under $GDPATH/lib.
func (t *Linux) libgodotArtefactPath(goarch string, editor bool) (string, error) {
	libc := t.BuildEnv.Target.LibC
	if libc == "" {
		libc = product.LibCGlibc
	}
	tool := t.ToolCatalog.LibGodot
	if editor {
		tool = t.ToolCatalog.LibGodotEditor
	}
	return tool.LookupPlatform(product.GOOSLinux, goarch, libc)
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
	if err := setGoCrossEnv(product.GOOSLinux, env.Target.GOARCH); err != nil {
		return xray.New(err)
	}
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
	if err := tools.Go.Action("build", args, "-tags", "archive", "-buildmode=c-archive", "-overlay="+overlay, "-a", "-x", "-o", libgo); err != nil {
		return xray.New(err)
	}
	if err := normalizeGoArchive(libgo, zig); err != nil {
		return xray.New(err)
	}
	if err := diagLibgodotArchive(libgo); err != nil {
		return xray.New(err)
	}
	pckStub := filepath.Join(project.GraphicsDirectory, "pck_section.c")
	if err := os.WriteFile(pckStub, []byte(`static const char pck_dummy[8] __attribute__((section("pck"), used)) = {0};
`), 0o644); err != nil {
		return xray.New(err)
	}
	zigArgs := []string{"cc", "-target", target, "-Wl,-u,main", pckStub, "-Wl,--start-group", t.lib, libgo}
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

// libgodotBuildGlibc produces the single-file linux binary via zig-cc
// pinned to `<arch>-linux-gnu.2.28`, statically links libgodot.a + the
// Go c-archive together plus zig's libc++, and leaves glibc/pthread/
// dl/m/rt dynamic. Skips the musl overlay, mallocng patch, and dlopen
// shim entirely; those are musl-only concerns.
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
	zig, err := tools.Zig.Lookup()
	if err != nil {
		return xray.New(err)
	}
	target, err := glibcZigTarget(GOARCH)
	if err != nil {
		return fmt.Errorf("gd build: cannot cross-compile linux/libgodot %v on %s: %w", GOARCH, env.Host.Tuple(), err)
	}
	if err := os.Setenv(product.EnvCC, zig+" cc -target "+target); err != nil {
		return xray.New(err)
	}
	if err := setGoCrossEnv(product.GOOSLinux, GOARCH); err != nil {
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
		if env.Host.GOOS == product.GOOSLinux && env.Host.GOARCH == GOARCH {
			defer func() {
				if err == nil {
					t.useGodotAt(t.out)
				}
			}()
		}
	}
	libgo := filepath.Join(project.GraphicsDirectory, fmt.Sprintf("linux_%v.libgodot.a", GOARCH))
	if err := tools.Go.Action("build", args, "-tags", "archive", "-buildmode=c-archive", "-a", "-x", "-o", libgo); err != nil {
		return xray.New(err)
	}
	if err := normalizeGoArchive(libgo, zig); err != nil {
		return xray.New(err)
	}
	if err := diagLibgodotArchive(libgo); err != nil {
		return xray.New(err)
	}
	pckStub := filepath.Join(project.GraphicsDirectory, "pck_section.c")
	if err := os.WriteFile(pckStub, []byte(`static const char pck_dummy[8] __attribute__((section("pck"), used)) = {0};
`), 0o644); err != nil {
		return xray.New(err)
	}
	// -Wl,-u,main forces the linker to treat `main` as an initial
	// undefined reference so the archive scan pulls in the cgo-emitted
	// main() wrapper even when crt1.o's implicit reference to main
	// gets lost across --start-group boundaries.
	zigArgs := []string{"cc", "-target", target, "-Wl,-u,main", pckStub, "-Wl,--start-group", t.lib, libgo}
	cgoLDFLAGS, err := tools.Go.Output("list", "-tags", "archive", "-deps", "-f", "{{range .CgoLDFLAGS}}{{println .}}{{end}}", ".")
	if err != nil {
		return xray.New(err)
	}
	for _, flag := range strings.Split(cgoLDFLAGS, "\n") {
		if flag = strings.TrimSpace(flag); flag != "" {
			zigArgs = append(zigArgs, flag)
		}
	}
	zigArgs = append(zigArgs, "-Wl,--end-group", "-lc++", "-lpthread", "-ldl", "-lm", "-lrt", "-o", t.out)
	if err := tools.Zig.Exec(zigArgs...); err != nil {
		return fmt.Errorf("libgodot glibc link: %w", err)
	}
	return nil
}

// glibcZigTarget returns the zig `-target` triple for a linux glibc
// libgodot build. Pinned to glibc 2.28 (Ubuntu 20.04+, Debian 11+,
// Fedora 30+, Arch, SteamOS, NixOS, Bazzite). Alpine users must opt
// into `--libc=musl`.
func glibcZigTarget(goarch string) (string, error) {
	switch goarch {
	case product.GOARCHAmd64:
		return "x86_64-linux-gnu.2.28", nil
	case product.GOARCHArm64:
		return "aarch64-linux-gnu.2.28", nil
	}
	return "", fmt.Errorf("no glibc zig target for GOARCH=%s", goarch)
}

// setGoCrossEnv points subsequent go invocations at the given cross
// target. Without GOOS/GOARCH the host toolchain produces Mach-O / PE
// archives on mac / windows runners and the zig-cc link fails on
// undefined `main`.
func setGoCrossEnv(goos, goarch string) error {
	if err := os.Setenv(product.EnvGOOS, goos); err != nil {
		return err
	}
	if err := os.Setenv(product.EnvGOARCH, goarch); err != nil {
		return err
	}
	if err := os.Setenv("CGO_ENABLED", "1"); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "==> go cross env: GOOS=%s GOARCH=%s CGO_ENABLED=%s CC=%q\n",
		os.Getenv(product.EnvGOOS), os.Getenv(product.EnvGOARCH),
		os.Getenv("CGO_ENABLED"), os.Getenv(product.EnvCC))
	return nil
}

// normalizeGoArchive extracts every member of libgo through zig ar
// (llvm-ar under the hood, cross-aware, understands every archive
// variant) and repacks them into a fresh SysV-format archive. Go's
// -buildmode=c-archive on darwin emits a BSD-format archive with
// __.SYMDEF SORTED whose member metadata lld's --start-group scan
// fails to enumerate — the ELF objects (including cgo's main()
// wrapper) are inside but invisible to the linker, hence
// 'undefined symbol: main'. Repacking through zig ar normalises the
// on-disk layout so any lld build sees the members.
func normalizeGoArchive(libgo, zig string) error {
	tmp, err := os.MkdirTemp("", "goarchive-*")
	if err != nil {
		return fmt.Errorf("normalize: mkdir tmp: %w", err)
	}
	defer os.RemoveAll(tmp)
	if err := shared.RunIn(tmp, zig, "ar", "x", libgo); err != nil {
		return fmt.Errorf("normalize: extract: %w", err)
	}
	entries, err := os.ReadDir(tmp)
	if err != nil {
		return fmt.Errorf("normalize: readdir: %w", err)
	}
	var members []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, "__.SYMDEF") {
			continue
		}
		members = append(members, filepath.Join(tmp, name))
	}
	sort.Strings(members)
	if len(members) == 0 {
		if st, statErr := os.Stat(libgo); statErr == nil {
			fmt.Fprintf(os.Stderr, "==> archive %s is %d bytes\n", libgo, st.Size())
		}
		_ = shared.Run(zig, "ar", "t", libgo)
		_ = shared.Run(zig, "ar", "tv", libgo)
		return fmt.Errorf("normalize: archive %s has no extractable members (see zig ar tv above)", libgo)
	}
	if err := os.Remove(libgo); err != nil {
		return fmt.Errorf("normalize: remove old: %w", err)
	}
	repackArgs := append([]string{"ar", "rcs", libgo}, members...)
	if err := shared.Run(zig, repackArgs...); err != nil {
		return fmt.Errorf("normalize: repack: %w", err)
	}
	return nil
}

// diagLibgodotArchive dumps what the Go c-archive actually contains
// so a subsequent 'undefined symbol: main' link error is diagnosable
// from the CI log alone: the file header + ar member list confirm the
// archive is ELF-for-target (not host Mach-O / PE), and the nm scan
// tells us whether the cgo-emitted main() wrapper is actually inside.
func diagLibgodotArchive(archive string) error {
	fmt.Fprintf(os.Stderr, "==> diag: %s\n", archive)
	if err := shared.Run("file", archive); err != nil {
		return fmt.Errorf("diag file: %w", err)
	}
	if err := shared.Run("ar", "t", archive); err != nil {
		return fmt.Errorf("diag ar t: %w", err)
	}
	nm, err := shared.OutputBytes("nm", "--defined-only", archive)
	if err != nil {
		return fmt.Errorf("diag nm: %w", err)
	}
	mainFound := false
	for _, line := range strings.Split(string(nm), "\n") {
		if strings.HasSuffix(line, " T main") || strings.HasSuffix(line, " W main") {
			fmt.Fprintf(os.Stderr, "    main provider: %s\n", strings.TrimSpace(line))
			mainFound = true
		}
	}
	if !mainFound {
		fmt.Fprintln(os.Stderr, "    !! no defined `main` symbol in the archive")
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
	libc := t.BuildEnv.Target.LibC
	if libc == "" {
		libc = product.LibCGlibc
	}
	t.out = filepath.Join(project.GraphicsDirectory, ".godot",
		"godot.linux."+libc+".template_release."+godotArch)
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
	switch t.BuildEnv.Target.LibC {
	case product.LibCMusl:
		return t.libgodotTestMusl(args...)
	default:
		return t.libgodotTestGlibc(args...)
	}
}

// libgodotTestGlibc builds the headless test binary via zig-cc pinned
// to `<arch>-linux-gnu.2.28`, linking libgodot + the Go c-archive test
// binary together with zig's libc++. Mirrors libgodotBuildGlibc; the
// only differences are the Go verb (`test -c` instead of `build`) and
// the omission of the pck stub (tests don't embed a pack).
func (t *Linux) libgodotTestGlibc(args ...string) error {
	env := t.BuildEnv
	tools := t.ToolCatalog
	if built_musl {
		return nil
	}
	defer func() { built_musl = true }()
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
	target, err := glibcZigTarget(GOARCH)
	if err != nil {
		return fmt.Errorf("gd test: %w", err)
	}
	if err := os.Setenv(product.EnvCC, zig+" cc -target "+target); err != nil {
		return xray.New(err)
	}
	libgo := filepath.Join(project.GraphicsDirectory, fmt.Sprintf("linux_%v.libgodot.a", GOARCH))
	if err := tools.Go.Action("test", args, "-c", "-tags", "archive", "-buildmode=c-archive", "-o", libgo); err != nil {
		return xray.New(err)
	}
	libgodot, err := t.libgodotArtefactPath(GOARCH, true)
	if err != nil {
		return xray.New(err)
	}
	editor := filepath.Join(project.GraphicsDirectory, "linux_"+GOARCH+".libgodot.editor")
	zigArgs := []string{"c++", "-target", target, "-Wl,--start-group", libgodot, libgo, "-Wl,--end-group",
		"-lc++", "-lpthread", "-ldl", "-lm", "-lrt", "-o", editor}
	if err := tools.Zig.Exec(zigArgs...); err != nil {
		return fmt.Errorf("libgodot glibc test link: %w", err)
	}
	t.useGodotAt(editor)
	if err := os.Chdir(project.GraphicsDirectory); err != nil {
		return xray.New(err)
	}
	args = append(args, "--headless")
	return t.godotTool().Exec(args...)
}

func (t *Linux) libgodotTestMusl(args ...string) error {
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
