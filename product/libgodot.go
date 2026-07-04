package product

// LibGodotRef is the pinned Godot ref libgodot builds check out. Bumping
// this here bumps every downstream libgodot artefact in lockstep.
const LibGodotRef = "4.7-stable"

// LibGodotRecipe is one row of the libgodot build matrix: a
// (target GOOS, target GOARCH, editor) tuple plus the SCons + naming
// bits the CLI needs to drive Godot's build for it.
//
// GodotPlatform / GodotArch translate the graphics.gd tuple into the
// tokens Godot's `platform=` / `arch=` scons variables want (Godot
// calls linux "linuxbsd" and amd64 "x86_64", etc.). ExtraSconsArgs
// carries per-target flags that don't fit anywhere else (musl CC,
// mingw toggle, ios simulator, ...).
//
// ArtefactName is the filename SCons drops under `bin/` after a
// successful build. InstallName is the filename the tooling catalog
// (ToolchainLibGodot / ToolchainLibGodotEditor) looks for under
// $GDPATH/lib — the builder renames on install so downstream consumers
// find the artefact at the expected path without knowing about
// Godot's naming convention.
type LibGodotRecipe struct {
	GOOS   string
	GOARCH string
	// LibC narrows the linux target between glibc (dynamic, uses the
	// host's ld.so at runtime) and musl (single-file + dlopen shim).
	// Empty for non-linux targets where the concept doesn't apply.
	LibC string
	// Editor selects the editor build (target=editor) instead of the
	// export-template one (target=template_release). The editor
	// artefact embeds the tools/asset importers; templates don't.
	Editor         bool
	GodotPlatform  string
	GodotArch      string
	ExtraSconsArgs []string
	ArtefactName   string
	InstallName    string
	// InstallSlug is the toolchain slug the sidecar is keyed on.
	// Either "libgodot" or "libgodot-editor" — matches ToolchainLibGodot
	// / ToolchainLibGodotEditor so `gdnext toolchain doctor` sees the
	// hash landed by `gdnext libgodot install`.
	InstallSlug string
	// ZigTarget, when non-empty, tells the builder to plant cc/c++/
	// ld/ar/ranlib shims that forward to `zig cc/c++/ar/ranlib -target
	// <ZigTarget>` and prepend them to PATH before invoking scons.
	// Shim names deliberately avoid `clang`/`clang++` — Godot's
	// linuxbsd detect.py flips use_llvm=True when CXX basename
	// contains "clang", which appends `.llvm` to every artefact.
	ZigTarget string
	// Quirks documents known-broken build cells (same shape as
	// Platform.Quirks). Consumed by ci/matrix_libgodot to gate
	// individual cells behind allow_fail without dropping them
	// from the matrix — the quirk row stays visible in workflow
	// summaries as the contract for why the cell is tolerated.
	Quirks []Quirk
}

// CIBlockedFor reports whether the recipe carries a QuirkCIBuildBroken
// for the given build host. The libgodot matrix generator uses it to
// mark the row as allow_fail (rather than omit it — visibility of
// the tolerated failure is the point of a Quirk).
func (r LibGodotRecipe) CIBlockedFor(hostGOOS, hostGOARCH string) bool {
	for _, q := range r.Quirks {
		if q.Scope == QuirkCIBuildBroken && q.AppliesToHost(hostGOOS, hostGOARCH) {
			return true
		}
	}
	return false
}

// LibC values a linux recipe can carry.
const (
	LibCGlibc = "glibc"
	LibCMusl  = "musl"
)

// muslCC returns the zig-cc target triple for a musl-linux static build.
func muslCC(goarch string) string {
	switch goarch {
	case GOARCHAmd64:
		return "x86_64-linux-musl"
	case GOARCHArm64:
		return "aarch64-linux-musl"
	}
	return ""
}

// LibGodotMatrix enumerates every (target, editor) row `gdnext libgodot
// build` knows how to drive. hostCanBuild in the builder still gates
// each row against the runtime host (linux target refuses to
// configure on darwin, ios needs a darwin host for the SDK, ...); the
// declarative matrix is the full "everything we intend to publish"
// list independent of what any single host can produce.
var LibGodotMatrix = []LibGodotRecipe{
	// linux/glibc via zig-cc pinned to glibc 2.28 — the default for
	// `gdnext libgodot build` and `gdnext --link=libgodot`. Produces
	// a normal glibc-dynamic .a; the resulting binary runs on any
	// linux with glibc ≥ 2.28 (Ubuntu 20.04+, Debian 11+, Fedora 30+,
	// Arch, SteamOS, NixOS, Bazzite). Alpine users need
	// `apk add gcompat`, same as upstream Godot's own docs.
	linuxGlibcRecipe(GOARCHAmd64, false),
	linuxGlibcRecipe(GOARCHAmd64, true),
	linuxGlibcRecipe(GOARCHArm64, false),
	linuxGlibcRecipe(GOARCHArm64, true),
	// linux/musl static — opt-in single-file variant that bakes in
	// the graphics.gd dlopen shim (bundled/libgodot/{musl_dlopen.c,
	// musl_foreign_tramp.S, musl_helper.c}). Requires no runtime dependencies
	// at all, but the shim's in-process ELF loader is a partial
	// reimplementation of the kernel's process bootstrap and has
	// distro-specific compatibility gaps (Bazzite MDWE fix works,
	// Ubuntu 24.04 glibc 2.39 still crashes in ld.so init). Keep as
	// opt-in for users who genuinely need the single-file property
	// (e.g. shipping to bare kernels or Alpine without gcompat).
	linuxMuslRecipe(GOARCHAmd64, false),
	linuxMuslRecipe(GOARCHAmd64, true),
	linuxMuslRecipe(GOARCHArm64, false),
	linuxMuslRecipe(GOARCHArm64, true),
	// windows — mingw via zig; both editor and template_release. The
	// editor variant embeds the tools/asset importers so `libgodot`
	// link mode can produce an editor executable on windows too.
	windowsMingwRecipe(GOARCHAmd64, false),
	windowsMingwRecipe(GOARCHAmd64, true),
	windowsMingwRecipe(GOARCHArm64, false),
	windowsMingwRecipe(GOARCHArm64, true),
	// darwin — needs a real darwin host; editor + template_release.
	macosRecipe(GOARCHAmd64, false),
	macosRecipe(GOARCHAmd64, true),
	macosRecipe(GOARCHArm64, false),
	macosRecipe(GOARCHArm64, true),
	// ios / android / web are omitted: upstream Godot's platform
	// drivers don't list "library" in get_flags().supported, so
	// library_type=static_library isn't a valid build for them.
	// Web is served by GDExtension side modules (dlink_enabled=yes),
	// not libgodot. Add recipes here once upstream flips the flag.
}

// linuxMuslRecipe builds the musl-static libgodot variant used when
// `--libc=musl` is requested. Ships with the graphics.gd dlopen shim
// baked into the merged archive so the static-musl binary can borrow
// the system's glibc ld.so at runtime. See linuxGlibcRecipe for the
// default (glibc-zig-cc) variant.
func linuxMuslRecipe(goarch string, editor bool) LibGodotRecipe {
	target, _, slug := editorTag(editor)
	installName := "libgodot.linux." + goarch + ".musl." + target + ".a"
	godotArch := "x86_64"
	if goarch == GOARCHArm64 {
		godotArch = "arm64"
	}
	// so_wrap stays ON (Godot default): X11/Wayland/xkbcommon/
	// alsa/pulse/dbus/fontconfig/speechd/udev get dlopen'd at
	// runtime rather than linked directly. The CLI's builder bakes
	// a dlopen shim (bundled copy of startup/internal/dlopen) into
	// the merged archive, so the static-musl binary can borrow the
	// system's glibc ld.so to resolve those .so.
	//
	// use_static_cpp static-links libstdc++/libgcc. builtin_sdl=yes
	// uses Godot's bundled SDL2 (avoids a system-SDL dlopen).
	// lto=none keeps the aggregate archive around 250MB; Godot's
	// default (thin) leaves every .o as LLVM bitcode and balloons
	// it past 1GB.
	//
	// Shim naming: upstream detect.py flips `use_llvm=True` (and
	// rewrites CC/CXX) the moment CXX basename contains "clang",
	// producing `.llvm` suffixed artefact names. Pinning CC=cc CXX=c++
	// keeps methods.py:using_gcc / using_clang both False (compiler
	// name is bare "cc"/"c++"), which skips the GCC-16+ branch that
	// otherwise adds `-Wno-sfinae-incomplete` — a flag clang doesn't
	// understand. The builder plants matching shims named cc/c++
	// under $GDPATH/libgodot-shim; see plantZigShims.
	extra := []string{
		"use_static_cpp=yes",
		"lto=none",
		"builtin_sdl=yes",
		"CC=cc",
		"CXX=c++",
		"AR=zig ar",
		"RANLIB=zig ranlib",
		"extra_suffix=" + LibCMusl,
	}
	return LibGodotRecipe{
		GOOS:           GOOSLinux,
		GOARCH:         goarch,
		LibC:           LibCMusl,
		Editor:         editor,
		GodotPlatform:  "linuxbsd",
		GodotArch:      godotArch,
		ExtraSconsArgs: extra,
		ZigTarget:      muslCC(goarch),
		ArtefactName:   "libgodot.linuxbsd." + target + "." + godotArch + "." + LibCMusl + ".a",
		InstallName:    installName,
		InstallSlug:    slug,
	}
}

// linuxGlibcRecipe builds the default linux libgodot variant using
// zig-cc pinned to `<arch>-linux-gnu.2.28` (matches the 2018 baseline
// Godot's own buildroot SDK provides). The resulting binary runs
// unmodified on any linux with glibc >= 2.28 (Ubuntu 20.04+, Debian 11+,
// Fedora 30+, Arch, SteamOS, NixOS, Bazzite). Alpine users need
// `apk add gcompat`, identical to upstream Godot's docs. Godot dlopens
// X11/Wayland/xkbcommon/alsa/pulse/dbus/fontconfig/speechd/udev at
// runtime via so_wrap (Godot default), so no build-time dependency
// on those libs.
func linuxGlibcRecipe(goarch string, editor bool) LibGodotRecipe {
	target, _, slug := editorTag(editor)
	installName := "libgodot.linux." + goarch + ".glibc." + target + ".a"
	godotArch := "x86_64"
	zigTarget := "x86_64-linux-gnu.2.28"
	if goarch == GOARCHArm64 {
		godotArch = "arm64"
		zigTarget = "aarch64-linux-gnu.2.28"
	}
	extra := []string{
		"use_static_cpp=yes",
		"lto=none",
		"builtin_sdl=yes",
		"CC=cc",
		"CXX=c++",
		"AR=zig ar",
		"RANLIB=zig ranlib",
		"extra_suffix=" + LibCGlibc,
	}
	return LibGodotRecipe{
		GOOS:           GOOSLinux,
		GOARCH:         goarch,
		LibC:           LibCGlibc,
		Editor:         editor,
		GodotPlatform:  "linuxbsd",
		GodotArch:      godotArch,
		ExtraSconsArgs: extra,
		ZigTarget:      zigTarget,
		ArtefactName:   "libgodot.linuxbsd." + target + "." + godotArch + "." + LibCGlibc + ".a",
		InstallName:    installName,
		InstallSlug:    slug,
	}
}

// editorTag returns (sconsTarget, installInfix, slug) for an editor
// vs template_release build. Shared by every per-platform factory so
// the naming stays consistent — release drops the `.editor.` infix,
// editor adds it and points at the libgodot-editor sidecar slug.
func editorTag(editor bool) (target, installInfix, slug string) {
	if editor {
		return "editor", ".editor", "libgodot-editor"
	}
	return "template_release", "", "libgodot"
}

// windowsMingwRecipe cross-compiles the windows libgodot via mingw-w64.
// Godot's own upstream CI uses this route (real mingw-w64 gcc from
// Debian/Ubuntu, not zig-cc), so it's the well-worn path. The build
// host needs mingw-w64 installed (apt install mingw-w64 on ubuntu;
// dnf install mingw64-gcc-c++ mingw64-winpthreads on Fedora). CI's
// libgodot.yml installs it in the build step for windows-targeting
// cells.
//
// The zig-cc route was tried and produces uncooperative behaviour:
// Godot's platform/windows/detect.py detects zig-cc as clang, flips
// use_llvm=True and appends .llvm to every artefact, then emits
// -Wa,-mbig-obj which clang rejects, and resolves windres to `None`.
// Real mingw-w64 sidesteps every one of those.
func windowsMingwRecipe(goarch string, editor bool) LibGodotRecipe {
	godotArch := "x86_64"
	mingwPrefix := "x86_64-w64-mingw32-"
	if goarch == GOARCHArm64 {
		godotArch = "arm64"
		mingwPrefix = "aarch64-w64-mingw32-"
	}
	target, _, slug := editorTag(editor)
	// use_mingw=yes routes Godot's windows platform driver through
	// gcc/mingw instead of MSVC. use_static_cpp=yes bundles libstdc++
	// into the archive. lto=none keeps aggregate size manageable
	// (mingw + lto ~= 1GB archives).
	extra := []string{
		"use_mingw=yes",
		"use_static_cpp=yes",
		"builtin_sdl=yes",
		"lto=none",
		"vulkan=no",
		"d3d12=no",
		"opengl3=no",
		"CC=" + mingwPrefix + "gcc",
		"CXX=" + mingwPrefix + "g++",
		"LINK=" + mingwPrefix + "g++",
		"AR=" + mingwPrefix + "ar",
		"RANLIB=" + mingwPrefix + "ranlib",
	}
	return LibGodotRecipe{
		GOOS:           GOOSWindows,
		GOARCH:         goarch,
		Editor:         editor,
		GodotPlatform:  "windows",
		GodotArch:      godotArch,
		ExtraSconsArgs: extra,
		ArtefactName:   "libgodot.windows." + target + "." + godotArch + ".a",
		InstallName:    "libgodot.windows." + goarch + "." + target + ".a",
		InstallSlug:    slug,
	}
}

// macosRecipe builds libgodot for macOS. Requires an Apple SDK
// available to scons; hostCanBuild rejects non-darwin hosts before
// this runs. Uses the platform driver's own compiler discovery
// (clang from Xcode) rather than overriding CC/CXX, since the sysroot
// wiring is intricate.
func macosRecipe(goarch string, editor bool) LibGodotRecipe {
	godotArch := "x86_64"
	if goarch == GOARCHArm64 {
		godotArch = "arm64"
	}
	target, _, slug := editorTag(editor)
	extra := []string{
		"vulkan=yes",
		"metal=yes",
		"builtin_sdl=yes",
	}
	return LibGodotRecipe{
		GOOS:           GOOSDarwin,
		GOARCH:         goarch,
		Editor:         editor,
		GodotPlatform:  "macos",
		GodotArch:      godotArch,
		ExtraSconsArgs: extra,
		ArtefactName:   "libgodot.macos." + target + "." + godotArch + ".a",
		InstallName:    "libgodot.darwin." + goarch + "." + target + ".a",
		InstallSlug:    slug,
	}
}

// FindLibGodotRecipe returns the matrix row matching (goos, goarch, editor),
// applying the same GOOS remaps FindPlatformByTargetEnv uses (so "musl"
// resolves to a linux row). For linux targets, returns the glibc variant
// by default; use FindLibGodotRecipeLibC to select musl explicitly. The
// "musl" GOOS remap steers linux + musl selection when callers still
// pass the legacy GOOS-based tuple. Returns false when no row matches.
func FindLibGodotRecipe(goos, goarch string, editor bool) (LibGodotRecipe, bool) {
	libc := ""
	if goos == "musl" {
		libc = LibCMusl
	}
	if remap, ok := GOOSRemaps[goos]; ok {
		goos = remap
	}
	return FindLibGodotRecipeLibC(goos, goarch, editor, libc)
}

// FindLibGodotRecipeLibC returns the matrix row matching
// (goos, goarch, editor, libc). Pass libc = "" to accept any variant
// (glibc preferred first). Only linux carries a meaningful libc; other
// targets ignore the parameter.
func FindLibGodotRecipeLibC(goos, goarch string, editor bool, libc string) (LibGodotRecipe, bool) {
	if goos == GOOSLinux && libc == "" {
		libc = LibCGlibc
	}
	for _, r := range LibGodotMatrix {
		if r.GOOS != goos || r.GOARCH != goarch || r.Editor != editor {
			continue
		}
		if goos == GOOSLinux && r.LibC != libc {
			continue
		}
		return r, true
	}
	return LibGodotRecipe{}, false
}

// SconsTarget returns the SCons `target=` token this recipe builds.
func (r LibGodotRecipe) SconsTarget() string {
	if r.Editor {
		return "editor"
	}
	return "template_release"
}

// SconsExtraSuffix returns the token SCons appends after `<arch>` in
// every `.o` / `.a` filename it emits (via `extra_suffix=<token>`).
// Empty means no extra suffix. Used today on linux to disambiguate
// glibc vs musl object files that would otherwise collide under the
// same source tree — SCons happily reuses cached `.o` files by name
// across incompatible toolchains, so distinct variants MUST carry a
// distinct suffix.
func (r LibGodotRecipe) SconsExtraSuffix() string {
	if r.GOOS == GOOSLinux && r.LibC != "" {
		return r.LibC
	}
	return ""
}

// SconsArgs returns the fully-assembled scons argv (minus the `scons`
// binary itself) for this recipe: base flags + ExtraSconsArgs.
func (r LibGodotRecipe) SconsArgs() []string {
	base := []string{
		"platform=" + r.GodotPlatform,
		"arch=" + r.GodotArch,
		"target=" + r.SconsTarget(),
		"library_type=static_library",
		"production=yes",
		"debug_symbols=no",
		// Godot 4.7's SCons defaults `redirect_build_objects=yes`,
		// which keeps every module archive under `bin/obj/` and
		// leaves the top-level `bin/libgodot.<...>.a` as a thin
		// shell containing only the linuxbsd platform objects
		// (~2.5MB). Downstream consumers linking a single .a would
		// miss the rest of the engine. Turn the redirect off so
		// SCons produces one fat archive under `bin/`.
		"redirect_build_objects=no",
		// SCons scrubs its subprocess env to only what upstream's
		// SConstruct copies in. We stash the musl <execinfo.h> shim
		// under CPATH (see plantMuslExecinfoInclude in the builder),
		// so import it into env["ENV"] here — the builder's own env
		// is otherwise invisible to the compiler.
		"import_env_vars=CPATH",
	}
	return append(base, r.ExtraSconsArgs...)
}
