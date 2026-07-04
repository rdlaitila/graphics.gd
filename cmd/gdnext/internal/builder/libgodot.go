package builder

import (
	"embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"graphics.gd/cmd/gdnext/internal/shared"
	"graphics.gd/cmd/gdnext/internal/tooling"
	"graphics.gd/product"

	"github.com/samber/do/v2"
	"runtime.link/api/xray"
)

//go:embed bundled/libgodot
var libgodotShims embed.FS

// LibGodot drives the SCons build that produces the libgodot.<...>.a
// static library the LibGodot link mode links against. Unlike the
// other builders in this package it does not build the user's project
// — its output is a toolchain artefact that later Musl / iOS / etc.
// builds consume. Wired into the CLI via `gdnext libgodot build`.
type LibGodot struct {
	BuildEnv    product.BuildEnv `do:""`
	ToolCatalog tooling.Catalog  `do:""`
}

func NewLibGodot(di do.Injector) (*LibGodot, error) {
	return do.InvokeStruct[*LibGodot](di)
}

// Build resolves the pinned godot source via the catalog (fetched
// as the release-tag zip, cached under $GDPATH/godot-src/<ref>),
// runs SCons with the recipe's args, and returns the absolute path
// to the produced bin/<ArtefactName>.
func (t *LibGodot) Build(recipe product.LibGodotRecipe) (string, error) {
	if err := t.hostCanBuild(recipe); err != nil {
		return "", err
	}
	src, err := t.ToolCatalog.GodotSrc.Lookup()
	if err != nil {
		return "", xray.New(err)
	}
	scons, err := t.ToolCatalog.SCons.Lookup(tooling.ModeFind)
	if err != nil {
		return "", fmt.Errorf("libgodot build: %w", err)
	}
	shimDir, err := t.plantToolchainShims(recipe)
	if err != nil {
		return "", xray.New(err)
	}
	includeDir, err := t.plantMuslExecinfoInclude(recipe)
	if err != nil {
		return "", xray.New(err)
	}
	env, err := t.sconsEnv(recipe, shimDir, includeDir)
	if err != nil {
		return "", xray.New(err)
	}
	args := recipe.SconsArgs()
	shared.Announce("", nil, "libgodot build", []string{
		fmt.Sprintf("%s/%s", recipe.GOOS, recipe.GOARCH),
		fmt.Sprintf("editor=%v", recipe.Editor),
		fmt.Sprintf("src=%s", src),
	})
	if err := shared.RunInEnvStdin(src, env, true, nil, scons, args...); err != nil {
		return "", fmt.Errorf("libgodot build: scons: %w", err)
	}
	artefact, err := findArtefact(filepath.Join(src, "bin"), recipe)
	if err != nil {
		return "", fmt.Errorf("libgodot build: scons finished but %w", err)
	}
	// Musl targets get a dlopen shim baked in: godot itself dlopens
	// X11/wayland/pulse/etc at runtime via so_wrap, and musl's own
	// dlopen is a stub. The shim is a no-op under glibc (guarded by
	// #ifndef __GLIBC__), so it is safe to compile it into every
	// libgodot flavour but there is no reason to when the target is
	// glibc.
	shimArchive, err := t.compileMuslShim(recipe)
	if err != nil {
		return "", fmt.Errorf("libgodot build: %w", err)
	}
	// Godot's SCons only bundles the platform-driver objects into
	// bin/libgodot.<...>.a; every module, driver, servers/scene tree,
	// and core/main archive ships as a separate .a alongside its
	// source dir. A downstream link against just the top-level file
	// would miss ~99% of the engine. Merge everything into one fat
	// archive here so consumers see the artefact upstream advertises.
	var extras []string
	if shimArchive != "" {
		extras = append(extras, shimArchive)
	}
	fatArchive, err := t.mergeArchives(src, artefact, recipe, extras...)
	if err != nil {
		return "", fmt.Errorf("libgodot build: %w", err)
	}
	fmt.Printf("==> libgodot artefact: %s\n", fatArchive)
	return fatArchive, nil
}

// mergeArchives combines every static archive SCons produced for
// this recipe (top-level bin/libgodot.<...>.a plus per-module,
// per-driver, per-servers .a files under the source tree) into a
// single fat archive. Uses ar's MRI script mode (open + addlib +
// save) so we don't have to extract/repack objects — MRI copies
// members directly between archives.
func (t *LibGodot) mergeArchives(src, topLevel string, recipe product.LibGodotRecipe, extras ...string) (string, error) {
	// Base scons suffix — <.platform.target.arch>. Godot's detect.py
	// may insert `.llvm` (use_llvm=yes) or `.san` between arch and
	// our own extra_suffix, so we match on this stable prefix and
	// further filter by the extra_suffix token below.
	baseSuffix := fmt.Sprintf(".%s.%s.%s", recipe.GodotPlatform, recipe.SconsTarget(), recipe.GodotArch)
	extraSuffix := recipe.SconsExtraSuffix()
	// Snapshot the pristine SCons-produced top-level archive (which
	// contains only the linuxbsd platform-driver objects) to a
	// `.scons` sidecar the first time we see it, so repeated merges
	// don't compound: on each call we overwrite topLevel with a fat
	// archive, and without a stable snapshot the NEXT merge would
	// re-include that fat archive via the addlib step, doubling the
	// output size every run (see mergeArchives history). The sidecar
	// is refreshed whenever topLevel shrinks below it (SCons has
	// relinked from scratch, so we grab the fresh platform objects).
	snapshot := topLevel + ".scons"
	if info, err := os.Stat(topLevel); err == nil {
		snapInfo, snapErr := os.Stat(snapshot)
		if snapErr != nil || info.Size() < snapInfo.Size() {
			if err := copyRegularFile(topLevel, snapshot); err != nil {
				return "", xray.New(err)
			}
		}
	} else if _, snapErr := os.Stat(snapshot); snapErr != nil {
		return "", fmt.Errorf("mergeArchives: neither %s nor its .scons snapshot exists (run scons first)", topLevel)
	}
	// Collect every .a whose name matches `*<suffix>*.a` under the
	// source tree. use_llvm=yes appends `.llvm` after arch; sanitizer
	// builds append `.san`; custom extra_suffix appends whatever the
	// user asked for. Match on the recipe's stable suffix and let
	// anything after it (including `.a`) come along. Skip the
	// pristine snapshot and the (about-to-be-overwritten) topLevel
	// itself so we don't feed either back into the merge.
	var members []string
	if err := filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		name := info.Name()
		if !strings.HasSuffix(name, ".a") || !strings.Contains(name, baseSuffix) {
			return nil
		}
		if extraSuffix != "" && !strings.Contains(name, "."+extraSuffix+".") && !strings.HasSuffix(name, "."+extraSuffix+".a") {
			return nil
		}
		if p == topLevel || p == snapshot {
			return nil
		}
		members = append(members, p)
		return nil
	}); err != nil {
		return "", xray.New(err)
	}
	members = append(members, extras...)
	// Snapshot last so platform-driver objects override any stale
	// copies of the same names pulled in from module archives.
	members = append(members, snapshot)
	if len(members) == 0 {
		desc := baseSuffix
		if extraSuffix != "" {
			desc += "*" + extraSuffix
		}
		return "", fmt.Errorf("mergeArchives: found no .a files matching *%s*.a under %s", desc, src)
	}
	// Repoint at the tool-provided ar. Zig ships one that
	// understands MRI scripts and matches whatever CC/AR we told
	// SCons to use, so the merged archive's format lines up with
	// the objects inside it.
	zig, err := t.ToolCatalog.Zig.Lookup()
	if err != nil {
		return "", xray.New(err)
	}
	// MRI script: create a fresh archive at topLevel, addlib each
	// member (copies its objects in), save, end. Overwrites the
	// small top-level libgodot.<...>.a in place.
	var script strings.Builder
	fmt.Fprintf(&script, "create %s\n", topLevel)
	for _, m := range members {
		fmt.Fprintf(&script, "addlib %s\n", m)
	}
	fmt.Fprintln(&script, "save")
	fmt.Fprintln(&script, "end")
	if err := shared.RunStdin(strings.NewReader(script.String()), zig, "ar", "-M"); err != nil {
		return "", fmt.Errorf("ar -M merge: %w", err)
	}
	fmt.Printf("==> merged %d archives into %s\n", len(members), topLevel)
	return topLevel, nil
}

// copyRegularFile snapshots src to dst with 0644 perms, overwriting
// dst if it exists. Small helper used by mergeArchives to preserve
// SCons's pristine bin/libgodot.<...>.a before we overwrite it with
// the fat merged archive.
func copyRegularFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

// compileMuslShim builds the graphics.gd dlopen shim (bundled copy of
// startup/internal/dlopen/dlopen.c + foreign_tramp.S, plus the glibc
// helper and the execinfo stub) into a small libdlopen archive that
// gets merged into the final libgodot. Only fires when ZigTarget
// contains "musl": the shim's whole body is
// gated by `#ifndef __GLIBC__` upstream, so it is a no-op under glibc
// and we skip the extra work.
//
// Baking the shim into libgodot lets consumers drop cgo's dlopen
// package from their build (switch `-tags musl` → `-tags archive`)
// and get the shim symbols exclusively from libgodot.a — no
// duplicate-symbol risk at final link.
func (t *LibGodot) compileMuslShim(recipe product.LibGodotRecipe) (string, error) {
	if recipe.LibC != product.LibCMusl {
		return "", nil
	}
	zig, err := t.ToolCatalog.Zig.Lookup()
	if err != nil {
		return "", xray.New(err)
	}
	dlopenC, err := libgodotShims.ReadFile("bundled/libgodot/musl_dlopen.c")
	if err != nil {
		return "", xray.New(err)
	}
	trampS, err := libgodotShims.ReadFile("bundled/libgodot/musl_foreign_tramp.S")
	if err != nil {
		return "", xray.New(err)
	}
	helperC, err := libgodotShims.ReadFile("bundled/libgodot/musl_helper.c")
	if err != nil {
		return "", xray.New(err)
	}
	dir := filepath.Join(t.BuildEnv.Host.GDRootPath, "libgodot-shim", shimSubdir(recipe))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", xray.New(err)
	}
	cPath := filepath.Join(dir, "musl_dlopen.c")
	sPath := filepath.Join(dir, "musl_foreign_tramp.S")
	helperCPath := filepath.Join(dir, "musl_helper.c")
	if err := os.WriteFile(cPath, dlopenC, 0o644); err != nil {
		return "", xray.New(err)
	}
	if err := os.WriteFile(sPath, trampS, 0o644); err != nil {
		return "", xray.New(err)
	}
	if err := os.WriteFile(helperCPath, helperC, 0o644); err != nil {
		return "", xray.New(err)
	}
	dlopenO := filepath.Join(dir, "musl_dlopen.o")
	trampO := filepath.Join(dir, "musl_foreign_tramp.o")
	// -fno-stack-protector: musl_dlopen.c switches %fs/tpidr_el0 in the
	// middle of functions, so a canary loaded from one TCB and
	// checked against another would trip __stack_chk_fail. Matches
	// the flag set on graphics.gd/startup/internal/dlopen/dlopen.go.
	if err := runShim(zig, "cc", "-target", recipe.ZigTarget, "-O2", "-g0", "-fno-stack-protector", "-c", cPath, "-o", dlopenO); err != nil {
		return "", fmt.Errorf("compile musl_dlopen.c: %w", err)
	}
	if err := runShim(zig, "cc", "-target", recipe.ZigTarget, "-O2", "-g0", "-c", sPath, "-o", trampO); err != nil {
		return "", fmt.Errorf("compile musl_foreign_tramp.S: %w", err)
	}
	// Compile musl_helper.c against glibc 2.28 (Godot's official baseline),
	// then embed the resulting ELF as a byte array so foreign_compile
	// can memfd_create + write + elf_exec it at runtime. Zero cc
	// dependency on the end-user's machine.
	helperBin := filepath.Join(dir, "musl_helper.bin")
	helperTarget := strings.Replace(recipe.ZigTarget, "musl", "gnu.2.28", 1)
	if err := runShim(zig, "cc", "-target", helperTarget, "-pie", "-fPIC", "-O2", "-g0",
		helperCPath, "-o", helperBin, "-ldl"); err != nil {
		return "", fmt.Errorf("compile musl_helper.c (glibc): %w", err)
	}
	embeddedC := filepath.Join(dir, "embedded_helper_generated.c")
	if err := writeEmbeddedHelperWrapper(embeddedC, helperBin); err != nil {
		return "", fmt.Errorf("generate embedded_helper wrapper: %w", err)
	}
	embeddedO := filepath.Join(dir, "embedded_helper_generated.o")
	if err := runShim(zig, "cc", "-target", recipe.ZigTarget, "-O2", "-g0",
		"-c", embeddedC, "-o", embeddedO); err != nil {
		return "", fmt.Errorf("compile embedded_helper_generated.c: %w", err)
	}
	// musl_execinfo.c: no-op backtrace/backtrace_symbols/
	// backtrace_symbols_fd. Godot's crash_handler_linuxbsd.cpp
	// unconditionally includes <execinfo.h> when the SCons host is
	// glibc (which it always is on ubuntu-latest), so we plant a
	// shim header + link stubs to satisfy the references. Effective
	// runtime behaviour matches upstream's own `execinfo=no`.
	execinfoC, err := libgodotShims.ReadFile("bundled/libgodot/musl_execinfo.c")
	if err != nil {
		return "", xray.New(err)
	}
	execinfoCPath := filepath.Join(dir, "musl_execinfo.c")
	if err := os.WriteFile(execinfoCPath, execinfoC, 0o644); err != nil {
		return "", xray.New(err)
	}
	execinfoO := filepath.Join(dir, "musl_execinfo.o")
	if err := runShim(zig, "cc", "-target", recipe.ZigTarget, "-O2", "-g0",
		"-c", execinfoCPath, "-o", execinfoO); err != nil {
		return "", fmt.Errorf("compile musl_execinfo.c: %w", err)
	}
	archive := filepath.Join(dir, "libdlopen.a")
	if err := os.Remove(archive); err != nil && !os.IsNotExist(err) {
		return "", xray.New(err)
	}
	if err := runShim(zig, "ar", "rcs", archive, dlopenO, trampO, embeddedO, execinfoO); err != nil {
		return "", fmt.Errorf("archive libdlopen: %w", err)
	}
	fmt.Printf("==> compiled dlopen shim into %s (helper %d bytes embedded)\n", archive, fileSize(helperBin))
	return archive, nil
}

// writeEmbeddedHelperWrapper emits a small C source declaring
// embedded_helper_bytes[] + embedded_helper_size referenced weakly by
// musl_dlopen.c. The bytes are the pre-compiled glibc helper binary;
// musl_dlopen.c writes them to a memfd at runtime and hands the
// /proc/self/fd path to elf_exec, avoiding any cc/filesystem
// dependency on the end user's host.
func writeEmbeddedHelperWrapper(dst, helperBin string) error {
	bytes, err := os.ReadFile(helperBin)
	if err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "/* Auto-generated by gdnext compileMuslShim; do not edit. */\n")
	fmt.Fprintf(&b, "const unsigned char embedded_helper_bytes[] = {")
	for i, x := range bytes {
		if i%16 == 0 {
			b.WriteString("\n  ")
		}
		fmt.Fprintf(&b, "0x%02x,", x)
	}
	fmt.Fprintf(&b, "\n};\n")
	fmt.Fprintf(&b, "const unsigned int embedded_helper_size = %d;\n", len(bytes))
	return os.WriteFile(dst, []byte(b.String()), 0o644)
}

func fileSize(path string) int64 {
	if info, err := os.Stat(path); err == nil {
		return info.Size()
	}
	return 0
}

func runShim(prog string, args ...string) error {
	return shared.Run(prog, args...)
}

// findArtefact locates the .a SCons dropped for this recipe. Godot's
// per-platform detect.py can inject extra_suffix segments the recipe
// can't predict statically (`.llvm` on use_llvm=yes, `.san` under
// sanitizers, custom extra_suffix values), so we first try the
// recipe's exact ArtefactName and fall back to a glob keyed on the
// stable prefix `libgodot.<platform>.<target>.<arch>*.a`. When the
// recipe carries an extra_suffix (e.g. `glibc`, `musl`) we then
// filter the glob matches to those whose basename contains that
// token, since detect.py may insert `.llvm` between `<arch>` and
// `<extra_suffix>` (which our prefix would otherwise miss).
func findArtefact(binDir string, recipe product.LibGodotRecipe) (string, error) {
	exact := filepath.Join(binDir, recipe.ArtefactName)
	if _, err := os.Stat(exact); err == nil {
		return exact, nil
	}
	prefix := fmt.Sprintf("libgodot.%s.%s.%s", recipe.GodotPlatform, recipe.SconsTarget(), recipe.GodotArch)
	matches, _ := filepath.Glob(filepath.Join(binDir, prefix+"*.a"))
	if es := recipe.SconsExtraSuffix(); es != "" {
		filtered := matches[:0]
		token := "." + es + "."
		altSuffix := "." + es + ".a"
		for _, m := range matches {
			name := filepath.Base(m)
			if strings.Contains(name, token) || strings.HasSuffix(name, altSuffix) {
				filtered = append(filtered, m)
			}
		}
		matches = filtered
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("expected artefact missing at %s, but %d candidates match %s*.a — set the recipe's ArtefactName explicitly: %v", exact, len(matches), prefix, matches)
	}
	return "", fmt.Errorf("expected artefact missing at %s and no candidates match %s*.a", exact, prefix)
}

// LocateArtefact returns the path Build would have produced for
// this recipe, or an error if the source tree / artefact aren't
// on disk. Uses ModeFind on the GodotSrc toolchain so it never
// triggers a download — install is meant to be a cheap post-build
// step, not a covert build trigger.
func (t *LibGodot) LocateArtefact(recipe product.LibGodotRecipe) (string, error) {
	src, err := t.ToolCatalog.GodotSrc.Lookup(tooling.ModeFind)
	if err != nil {
		return "", fmt.Errorf("libgodot: godot source not on disk (run `gdnext libgodot build` first): %w", err)
	}
	return findArtefact(filepath.Join(src, "bin"), recipe)
}

// Install copies a built artefact into the host GDLibPath under the
// recipe's InstallName and writes the sidecar so `gdnext toolchain
// doctor` and subsequent lookups see the hash. artefact may be the
// path returned by [LibGodot.Build] or any pre-built .a the caller
// wants to plug into the toolchain (e.g. downloaded from a release).
func (t *LibGodot) Install(recipe product.LibGodotRecipe, artefact string) (string, string, error) {
	if err := sanityCheckArchive(artefact); err != nil {
		return "", "", fmt.Errorf("libgodot install: %w", err)
	}
	installed, sum, err := tooling.InstallLibraryFromFile(
		t.BuildEnv.Host,
		recipe.InstallSlug,
		recipe.GOOS,
		recipe.GOARCH,
		recipe.LibC,
		artefact,
		recipe.InstallName,
	)
	if err != nil {
		return "", "", xray.New(err)
	}
	fmt.Printf("==> libgodot installed: %s\n    %s\n", installed, sum)
	return installed, sum, nil
}

// CleanOptions selects how much of the libgodot build state to remove.
type CleanOptions struct {
	AllRecipes bool // ignore recipe filter, wipe SCons output for every target
	SourceTree bool // delete the whole godot-src/<ref>/ cache directory
	Shims      bool // also wipe $GDPATH/libgodot-shim/
	Installed  bool // also delete the installed .a + sidecar
	DryRun     bool // list what would be removed without touching anything
}

// Clean removes the build artefacts a prior `libgodot build` produced.
// The default (no options set) is a recipe-scoped scrub of SCons's
// per-suffix .o and .a files under the source tree, forcing a full
// recompile on the next build without paying the cost of re-fetching
// the godot source.
func (t *LibGodot) Clean(recipe product.LibGodotRecipe, opts CleanOptions) error {
	host := t.BuildEnv.Host
	src, err := t.ToolCatalog.GodotSrc.Lookup(tooling.ModeFind)
	if err != nil {
		if !os.IsNotExist(err) {
			// Only surface non-not-found errors; a missing source
			// tree just means there's nothing to scrub under it.
			fmt.Printf("==> libgodot clean: godot source not on disk, skipping SCons scrub\n")
		}
		src = ""
	}
	if opts.SourceTree {
		if src != "" {
			if err := removeAll(src, opts.DryRun); err != nil {
				return xray.New(err)
			}
			fmt.Printf("==> removed godot source tree: %s\n", src)
		}
	} else if src != "" {
		if err := cleanSconsOutput(src, recipe, opts.AllRecipes, opts.DryRun); err != nil {
			return xray.New(err)
		}
	}
	if opts.Shims {
		shimDir := filepath.Join(host.GDRootPath, "libgodot-shim")
		if _, err := os.Stat(shimDir); err == nil {
			if err := removeAll(shimDir, opts.DryRun); err != nil {
				return xray.New(err)
			}
			fmt.Printf("==> removed shim dir: %s\n", shimDir)
		}
	}
	if opts.Installed {
		targets := []product.LibGodotRecipe{recipe}
		if opts.AllRecipes {
			targets = nil
			for _, r := range product.LibGodotMatrix {
				targets = append(targets, r)
			}
		}
		for _, r := range targets {
			installed := filepath.Join(host.GDLibPath, r.InstallName)
			sidecar := tooling.SidecarPath(host, r.InstallSlug, r.GOOS, r.GOARCH, r.LibC)
			for _, p := range []string{installed, sidecar} {
				if _, err := os.Stat(p); err == nil {
					if err := removePath(p, opts.DryRun); err != nil {
						return xray.New(err)
					}
					fmt.Printf("==> removed installed artefact: %s\n", p)
				}
			}
		}
	}
	return nil
}

// cleanSconsOutput removes every SCons-produced .o and .a under src
// matching this recipe's stable suffix (`.<platform>.<target>.<arch>`),
// plus any generated shader/theme headers under bin/. Everything else
// (source, third-party trees, config caches) stays. When allRecipes is
// true, all .o/.a files are removed regardless of suffix.
func cleanSconsOutput(src string, recipe product.LibGodotRecipe, allRecipes, dryRun bool) error {
	baseSuffix := fmt.Sprintf(".%s.%s.%s", recipe.GodotPlatform, recipe.SconsTarget(), recipe.GodotArch)
	extraSuffix := recipe.SconsExtraSuffix()
	var removed int
	err := filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		name := info.Name()
		if !strings.HasSuffix(name, ".o") && !strings.HasSuffix(name, ".a") {
			return nil
		}
		if !allRecipes {
			if !strings.Contains(name, baseSuffix) {
				return nil
			}
			if extraSuffix != "" && !strings.Contains(name, "."+extraSuffix+".") && !strings.HasSuffix(name, "."+extraSuffix+".a") && !strings.HasSuffix(name, "."+extraSuffix+".o") {
				return nil
			}
		}
		if err := removePath(p, dryRun); err != nil {
			return err
		}
		removed++
		return nil
	})
	if err != nil {
		return err
	}
	verb := "removed"
	if dryRun {
		verb = "would remove"
	}
	if allRecipes {
		fmt.Printf("==> %s %d SCons object/archive files (all recipes)\n", verb, removed)
	} else {
		desc := baseSuffix
		if extraSuffix != "" {
			desc += " + " + extraSuffix
		}
		fmt.Printf("==> %s %d SCons object/archive files matching *%s*\n", verb, removed, desc)
	}
	return nil
}

func removePath(p string, dryRun bool) error {
	if dryRun {
		fmt.Printf("would remove: %s\n", p)
		return nil
	}
	return os.Remove(p)
}

func removeAll(p string, dryRun bool) error {
	if dryRun {
		fmt.Printf("would remove tree: %s\n", p)
		return nil
	}
	return os.RemoveAll(p)
}

// sanityCheckArchive verifies the file exists, is non-empty, and
// starts with the Unix `ar` magic (`!<arch>\n`). Catches the common
// mistake of pointing --from at a script/text file, and refuses a
// suspiciously tiny archive that would indicate an incomplete
// upstream build (Godot's untouched top-level .a is ~2.5MB and
// missing 99% of the engine — see mergeArchives).
func sanityCheckArchive(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("artefact missing at %s: %w", path, err)
	}
	if fi.Size() < 8 {
		return fmt.Errorf("artefact %s is too small (%d bytes) to be an archive", path, fi.Size())
	}
	f, err := os.Open(path)
	if err != nil {
		return xray.New(err)
	}
	defer f.Close()
	magic := make([]byte, 8)
	if _, err := f.Read(magic); err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if string(magic) != "!<arch>\n" {
		return fmt.Errorf("artefact %s is not a Unix ar archive (bad magic %q)", path, magic)
	}
	// The unmerged top-level libgodot.<...>.a Godot's SCons produces
	// is ~2.5MB and contains only the platform-driver objects. A
	// fat archive with the full engine is ~200MB+. Warn if a value
	// under 50MB slips through — likely means Build's merge step
	// didn't run (e.g. `--from` pointing at a raw scons artefact).
	if fi.Size() < 50*1024*1024 {
		fmt.Printf("==> warning: %s is %s — expected ~200MB+ for a full libgodot; the file may be missing engine components\n", path, humanBytes(fi.Size()))
	}
	return nil
}

func humanBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1fGB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1fKB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%dB", n)
	}
}

// hostCanBuild rejects recipes whose target can't be cross-compiled
// from the current host, with a clear message pointing at the
// alternative. Rules mirror what upstream Godot's platform drivers
// support: linuxbsd cross-compiles from anywhere via zig-clang;
// windows via zig-mingw; android via the NDK (env-provided);
// macos/ios need a real darwin host for the SDK sysroots.
func (t *LibGodot) hostCanBuild(recipe product.LibGodotRecipe) error {
	host := t.BuildEnv.Host.GOOS
	switch recipe.GOOS {
	case product.GOOSLinux:
		// zig cc handles the linux-musl cross from any host; the
		// linuxbsd platform driver itself refuses to configure on
		// non-linux boxes though (can_build() checks os.name != posix
		// or darwin), so gate to linux until we teach it otherwise.
		if host != product.GOOSLinux {
			return fmt.Errorf("libgodot build: linuxbsd target requires a linux build host (got %s)", t.BuildEnv.Host.Tuple())
		}
		return nil
	case product.GOOSWindows:
		// zig-mingw cross-compile works from any host; scons on windows
		// wants a msvc-style shell but we route everything through zig
		// so linux/darwin runners can drive it. Windows-on-windows also
		// fine via the same zig path.
		return nil
	case product.GOOSDarwin:
		if host != product.GOOSDarwin {
			return fmt.Errorf("libgodot build: %s target requires a darwin host for the macOS SDK sysroot (got %s)", recipe.GOOS, t.BuildEnv.Host.Tuple())
		}
		return nil
	default:
		return fmt.Errorf("libgodot build: target %s/%s is declared in product.LibGodotMatrix but no host is known to be able to build it", recipe.GOOS, recipe.GOARCH)
	}
}

// sconsEnv builds the child env for the scons process: parent env
// with inherited compiler vars scrubbed (so upstream detect.py can't
// pick up a host clang/gcc that would leak wrong-LLVM .o files),
// SCONSFLAGS augmented with `-j$(nproc)`, PATH prefixed with a
// per-recipe shim dir when ZigTarget is set, and android's NDK vars
// when the recipe targets android.
func (t *LibGodot) sconsEnv(recipe product.LibGodotRecipe, shimDir, includeDir string) ([]string, error) {
	strip := map[string]bool{
		"CC": true, "CXX": true, "LINK": true, "AR": true, "RANLIB": true,
		"LD": true, "LDFLAGS": true, "CFLAGS": true, "CXXFLAGS": true, "CPPFLAGS": true,
	}
	// Collect prepends we want on PATH in first-wins order.
	var pathPrepends []string
	if shimDir != "" {
		pathPrepends = append(pathPrepends, shimDir)
	}
	var env []string
	for _, kv := range os.Environ() {
		eq := strings.IndexByte(kv, '=')
		if eq > 0 && strip[kv[:eq]] {
			continue
		}
		if eq > 0 && (kv[:eq] == "PATH" || kv[:eq] == "CPATH") {
			continue // handled below
		}
		env = append(env, kv)
	}
	if os.Getenv("SCONSFLAGS") == "" {
		env = append(env, "SCONSFLAGS=-j"+jobsForScons())
	}
	// Pin zig's cache under $GDPATH so SCons's subprocess env (which
	// scrubs HOME in some configurations) doesn't fail zig's cache
	// directory resolution with "AppDataDirUnavailable".
	zigCache := filepath.Join(t.BuildEnv.Host.GDRootPath, "zig-cache")
	if os.Getenv("ZIG_LOCAL_CACHE_DIR") == "" {
		env = append(env, "ZIG_LOCAL_CACHE_DIR="+zigCache)
	}
	if os.Getenv("ZIG_GLOBAL_CACHE_DIR") == "" {
		env = append(env, "ZIG_GLOBAL_CACHE_DIR="+zigCache)
	}
	// CPATH points gcc/clang at our shim <execinfo.h> when targeting
	// musl (see plantMuslExecinfoInclude). Merges with any inherited
	// CPATH so callers can layer additional include dirs.
	if includeDir != "" {
		parent := os.Getenv("CPATH")
		if parent != "" {
			env = append(env, "CPATH="+includeDir+string(os.PathListSeparator)+parent)
		} else {
			env = append(env, "CPATH="+includeDir)
		}
	} else if parent := os.Getenv("CPATH"); parent != "" {
		env = append(env, "CPATH="+parent)
	}
	// Assemble final PATH: shim dir first, then whatever the parent PATH was.
	parentPath := os.Getenv("PATH")
	joined := strings.Join(pathPrepends, string(os.PathListSeparator))
	switch {
	case joined != "" && parentPath != "":
		env = append(env, "PATH="+joined+string(os.PathListSeparator)+parentPath)
	case joined != "":
		env = append(env, "PATH="+joined)
	case parentPath != "":
		env = append(env, "PATH="+parentPath)
	}
	return env, nil
}

// plantMuslExecinfoInclude writes the bundled musl_execinfo.h shim
// as `execinfo.h` under a scratch include directory, and returns
// that directory for CPATH. Only fires for musl targets — glibc
// sysroots ship their own execinfo.h and don't need the shim.
//
// The stub declarations here are paired with musl_execinfo.o
// (compiled by compileMuslShim and merged into libgodot.a), so
// Godot's crash_handler links against no-op backtrace symbols
// instead of failing on missing <execinfo.h> at compile time.
func (t *LibGodot) plantMuslExecinfoInclude(recipe product.LibGodotRecipe) (string, error) {
	if recipe.LibC != product.LibCMusl {
		return "", nil
	}
	header, err := libgodotShims.ReadFile("bundled/libgodot/musl_execinfo.h")
	if err != nil {
		return "", xray.New(err)
	}
	dir := filepath.Join(t.BuildEnv.Host.GDRootPath, "libgodot-shim", shimSubdir(recipe), "include")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", xray.New(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "execinfo.h"), header, 0o644); err != nil {
		return "", xray.New(err)
	}
	return dir, nil
}

// plantZigShims writes cc/c++/ld/ld.lld/ar/ranlib scripts into a
// scratch dir that forward every call to `zig cc/c++/ar/ranlib
// -target <ZigTarget> -g0`. The scratch dir is prepended to PATH
// via sconsEnv so SCons's Tool auto-detection picks up our zig
// toolchain.
//
// Shim names deliberately avoid `clang` / `clang++` — Godot's
// linuxbsd detect.py auto-flips `use_llvm=True` the moment CXX
// basename contains "clang", which appends `.llvm` to every
// artefact filename and diverges the .o names from the maintainer's
// blackbox blob (which was byte-verified as also built with zig
// 0.15.2's clang but without triggering the use_llvm path).
func (t *LibGodot) plantZigShims(recipe product.LibGodotRecipe) (string, error) {
	if recipe.ZigTarget == "" {
		return "", nil
	}
	zig, err := t.ToolCatalog.Zig.Lookup()
	if err != nil {
		return "", xray.New(err)
	}
	dir := filepath.Join(t.BuildEnv.Host.GDRootPath, "libgodot-shim", shimSubdir(recipe))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", xray.New(err)
	}
	zigCache := filepath.Join(t.BuildEnv.Host.GDRootPath, "zig-cache")
	compileTmpl, err := libgodotShims.ReadFile("bundled/libgodot/compile.sh.tmpl")
	if err != nil {
		return "", xray.New(err)
	}
	toolTmpl, err := libgodotShims.ReadFile("bundled/libgodot/tool.sh.tmpl")
	if err != nil {
		return "", xray.New(err)
	}
	render := func(tmpl []byte, verb string) string {
		return strings.NewReplacer(
			"{{CACHE}}", strconv.Quote(zigCache),
			"{{ZIG}}", strconv.Quote(zig),
			"{{VERB}}", verb,
			"{{TARGET}}", recipe.ZigTarget,
		).Replace(string(tmpl))
	}
	shims := []struct {
		name, verb string
		tmpl       []byte
	}{
		// Only cc/c++ (plus ld variants + ar/ranlib): the recipe
		// passes CC=cc CXX=c++ to SCons via ARGUMENTS, so SCons's
		// default Tool respects those names and skips auto-discovery
		// of gcc/g++/clang on PATH. Naming them cc/c++ instead of
		// gcc/g++ or clang/clang++ keeps methods.py:using_gcc and
		// methods.py:using_clang both False, avoiding the GCC-16+
		// -Wno-sfinae-incomplete branch AND detect.py's
		// use_llvm=True auto-flip that would append `.llvm` to
		// every artefact filename.
		{"cc", "cc", compileTmpl},
		{"c++", "c++", compileTmpl},
		{"ld", "cc", compileTmpl},
		{"ld.lld", "cc", compileTmpl},
		{"ar", "ar", toolTmpl},
		{"ranlib", "ranlib", toolTmpl},
	}
	for _, s := range shims {
		p := filepath.Join(dir, s.name)
		if err := os.WriteFile(p, []byte(render(s.tmpl, s.verb)), 0o755); err != nil {
			return "", xray.New(err)
		}
	}
	return dir, nil
}

// plantToolchainShims plants zig-cc shims for any recipe carrying a
// non-empty ZigTarget. Recipes without a ZigTarget (windows/mingw,
// darwin, etc.) plant nothing here — those toolchains are supplied
// directly via CC= / CXX= in ExtraSconsArgs and their compilers are
// pre-installed on the CI runner.
func (t *LibGodot) plantToolchainShims(recipe product.LibGodotRecipe) (string, error) {
	if recipe.ZigTarget == "" {
		return "", nil
	}
	if _, err := t.ToolCatalog.Zig.Lookup(); err != nil {
		return "", xray.New(err)
	}
	return t.plantZigShims(recipe)
}

// shimSubdir names the per-recipe shim directory under $GDPATH/libgodot-shim/.
// Includes LibC when set so glibc-arm64 (zig-cc, gnu.2.28 target) and
// musl-arm64 (zig-cc, musl target) don't share a directory and clobber
// each other's cc/c++ shim templates.
func shimSubdir(r product.LibGodotRecipe) string {
	sub := r.GOOS + "-" + r.GOARCH
	if r.LibC != "" {
		sub += "-" + r.LibC
	}
	return sub
}

// jobsForScons returns the -j argument SCons should use. Reads
// GOMAXPROCS as a hint (set by the CI runner or the user) and falls
// back to "4" — enough to be fast on a modest runner without
// spinning up 32 parallel g++ processes on a beefy dev box that
// wanted to run other things.
func jobsForScons() string {
	if v := os.Getenv("GOMAXPROCS"); v != "" {
		return v
	}
	return "4"
}
