package product

import "sort"

// Quirk is a structured caveat attached to a Platform. Empty Hosts
// means the quirk applies to every build host (build-scope quirks)
// or every play host (play-scope quirks) for the Platform. Empty
// Compat (play-scope only) means every compat layer on those hosts.
// Empty LinkModes (play-scope only) means every link mode.
type Quirk struct {
	Title     string     `json:"title"                xml:"title,attr"                 yaml:"title"`
	Scope     QuirkScope `json:"scope"                xml:"scope,attr"                 yaml:"scope"`
	Hosts     []string   `json:"hosts,omitempty"      xml:"hosts>host,omitempty"      yaml:"hosts,omitempty"`
	Compat    []string   `json:"compat,omitempty"     xml:"compat>layer,omitempty"    yaml:"compat,omitempty"`
	LinkModes []LinkMode `json:"link_modes,omitempty" xml:"link_modes>mode,omitempty" yaml:"link_modes,omitempty"`
	Reason    string     `json:"reason"               xml:"reason"                    yaml:"reason"`
	Result    []string   `json:"result,omitempty"     xml:"result>item,omitempty"     yaml:"result,omitempty"`
	Refs      []string   `json:"refs,omitempty"       xml:"refs>ref,omitempty"        yaml:"refs,omitempty"`
}

// QuirkScope is the operational consequence of a Quirk on a Platform row.
type QuirkScope uint8

const (
	QuirkUnknown QuirkScope = iota
	QuirkInformational
	QuirkRuntime
	QuirkBuildFlaky
	QuirkCIBuildBroken
	QuirkCIPlayBroken
)

// QuirkWindowsAmd64WinePlayBroken marks windows/amd64 artefacts as
// unplayable under wine on a linux play host. The (play host, wine)
// cell is omitted from the play matrix — the build itself stays
// green; switch to compat=proton for linux→windows play coverage.
var QuirkWindowsAmd64WinePlayBroken = Quirk{
	Title:  "linux play host: windows/amd64 under wine hangs before reaching the play-bot signal",
	Scope:  QuirkCIPlayBroken,
	Hosts:  []string{Tuple(GOOSLinux, GOARCHAmd64)},
	Compat: []string{"wine"},
	Reason: "On ubuntu-latest with WineHQ stable 11.x + Xvfb, the " +
		"canarybird .exe launches under wine but produces no Godot " +
		"engine output past initial OLE/RpcSs warnings and never " +
		"writes play-report.json. play-cell hits its 90s timeout and " +
		"reports `engine exit: signal: killed`. Cause not pinned: " +
		"Godot 4 + cgo windows builds appear to stall during early " +
		"engine bring-up under headless Wine, even with a fresh " +
		"prefix. Proton via umu-launcher is the recommended route " +
		"for linux→windows play coverage.",
	Result: []string{
		"the (linux play host, windows/amd64 target, compat=wine) cell is omitted from the play matrix",
		"the windows/amd64 build itself stays green",
		"users wanting linux→windows play coverage should select compat=proton instead",
	},
	Refs: []string{
		"https://github.com/rdlaitila/graphics.gd/actions/runs/28194989839/job/83521456750",
	},
}

// QuirkAndroidArm64EmuMissingOnArm64Host marks android/arm64 plays
// under reactivecircus/android-emulator-runner on the arm64 GitHub
// runner as broken: Google publishes the `emulator` SDK package
// (the AVD engine binary) only as x86_64 builds, so on the arm64
// runner `sdkmanager --install emulator` returns "Failed to find
// package 'emulator'" and the action exits before the AVD ever
// starts. Tracked separately from arm64 plays through cross-arch
// translation on amd64 hosts (which is a different failure mode).
var QuirkAndroidArm64EmuMissingOnArm64Host = Quirk{
	Title:  "android/arm64 play: Google's `emulator` SDK package isn't published for arm64 hosts",
	Scope:  QuirkCIPlayBroken,
	Hosts:  []string{Tuple(GOOSLinux, GOARCHArm64)},
	Compat: []string{"android-emu"},
	Reason: "On the GitHub ubuntu-24.04-arm runner, reactivecircus/" +
		"android-emulator-runner@v2 calls `sdkmanager --install " +
		"emulator` to lay down the AVD binary, and sdkmanager " +
		"answers `Warning: Failed to find package 'emulator'` because " +
		"Google ships the AVD engine as an x86_64 native binary " +
		"only. The action then bails before booting the device. No " +
		"action-side workaround until upstream publishes an arm64 " +
		"build of the emulator binary, or until we swap to a runner " +
		"that ships an arm64 AVD binary out-of-band.",
	Result: []string{
		"the (ubuntu-24.04-arm, android/arm64, android-emu) play cell is omitted from the play matrix",
		"the android/arm64 build itself stays green",
		"users with arm64 dev machines (Apple Silicon, Linux/arm64 desktops) can still install the APK on a physical device or x86_64 emulator outside CI",
	},
	Refs: []string{
		"https://github.com/rdlaitila/graphics.gd/actions/runs/28216544795/job/83589164073",
		"https://issuetracker.google.com/issues/240866758",
	},
}

// QuirkWebWasmGDExtensionPlayBroken marks js/wasm play under headless
// chrome/firefox as broken: the engine boots and renders a frame,
// but the stock Godot 4.7 web export template ships without
// GDExtension support, so any graphics.gd extension call hits a
// null function pointer and the page crashes before the play-bot
// can write its report. The matrix omits the cells outright; we
// keep the play_browser dispatcher and the canarybird wasm shim in
// place so the moment a GDExtension-enabled web template (or a
// libgodot.web.wasm artefact) ships, removing this quirk re-enables
// the cells with zero other code changes.
var QuirkWebWasmGDExtensionPlayBroken = Quirk{
	Title:  "js/wasm play under chrome/firefox: stock Godot 4.7 web template lacks GDExtension support",
	Scope:  QuirkCIPlayBroken,
	Hosts:  []string{Tuple(GOOSLinux, GOARCHAmd64)},
	Compat: []string{"chrome", "firefox"},
	Reason: "The browser cell launches via Playwright with COEP/COOP " +
		"headers, the wasm bundle loads, and Godot reaches the initial " +
		"frame. The bundle's own log line reads `Build configuration: " +
		"Emscripten ..., no GDExtension support.` \u2014 the stock Godot " +
		"4.7 web export template is compiled without GDExtension. Any " +
		"graphics.gd runtime call that crosses the extension boundary " +
		"resolves to a null function pointer and crashes the page before " +
		"the play-bot can emit its GDNEXT_PLAY_RESULT line. Upstream's own " +
		"web tests sidestep this by linking via libgodot (statically " +
		"embedding the engine in library.wasm), but no libgodot.web.wasm " +
		"artefact is published at release.graphics.gd yet, so gdnext build " +
		"can't take that route.",
	Result: []string{
		"the (js/wasm, chrome) and (js/wasm, firefox) play cells are omitted from the play matrix",
		"the js/wasm build itself stays green",
		"cells turn green automatically once the upstream Godot web template ships with GDExtension support, or once a libgodot.web.wasm artefact is published and gdnext build is taught to use it",
	},
	Refs: []string{
		"https://github.com/godotengine/godot/issues/100789",
	},
}

// QuirkAndroidAmd64EmuShaderUniformsCap marks android/amd64 plays
// under android-emu as broken: the AVD's SwiftShader GLES driver
// advertises GL_MAX_FRAGMENT_UNIFORM_VECTORS at the spec floor
// (256), but Godot 4.7's SceneShaderGLES3 compiles a variant that
// needs ~261 fragment uniform vectors. The activity launches, the
// engine reaches first frame, then the shader link fails and the
// scene renderer never comes up — `_start_success == false` fires
// from cleanup and the play-bot never gets to run. Real devices
// and Waydroid (host Mesa) expose 1024+, so the same APK runs fine
// off-CI. The proposed long-term path is the new waydroid
// play host; until that lands the cell is omitted from the play
// matrix.
// https://github.com/godotengine/godot/issues/109550
var QuirkAndroidAmd64EmuShaderUniformsCap = Quirk{
	Title:  "android/amd64 play under android-emu: SwiftShader caps fragment uniforms below Godot 4.7's SceneShaderGLES3 requirement",
	Scope:  QuirkCIPlayBroken,
	Hosts:  []string{Tuple(GOOSLinux, GOARCHAmd64)},
	Compat: []string{"android-emu"},
	Reason: "The reactivecircus/android-emulator-runner AVD runs " +
		"with `-gpu swiftshader_indirect`. SwiftShader implements " +
		"the GLES3 spec at the floor — GL_MAX_FRAGMENT_UNIFORM_VECTORS " +
		"reports 256 (the minimum the spec requires drivers to " +
		"advertise). Godot 4.7's SceneShaderGLES3 compiles a " +
		"specialization variant whose fragment shader declares ~261 " +
		"uniform vectors, so the GL program-link step fails with " +
		"`Fragment shader active uniforms exceed GL_MAX_FRAGMENT_" +
		"UNIFORM_VECTORS (261)`. The Godot activity then bails " +
		"during scene-renderer bring-up (`_start_success == false` " +
		"at main/main.cpp cleanup) before the play-bot's main loop " +
		"runs. Confirmed not a graphics.gd / GDExtension issue: the " +
		"same APK boots and reports `success=true` on Waydroid " +
		"(host Mesa, 4096+ fragment uniforms) and on physical " +
		"devices. No emulator-side workaround: `-gpu host` requires " +
		"a real X server (unavailable on the runner) and `-gpu auto` " +
		"falls back to swiftshader.",
	Result: []string{
		"the (linux/amd64 play host, android/amd64 target, android-emu) play cell is omitted from the play matrix",
		"the android/amd64 build itself stays green",
		"play coverage for android/amd64 moves to compat=waydroid",
	},
	Refs: []string{
		"https://github.com/rdlaitila/graphics.gd/actions/runs/28276986664/job/83785858474",
		"https://github.com/godotengine/godot/blob/master/drivers/gles3/shader_gles3.cpp",
		"https://registry.khronos.org/OpenGL-Refpages/es3.0/html/glGet.xhtml",
	},
}

// QuirkLinuxAmd64LibGodotPlayEnvLoss marks libgodot-linked plays on
// linux/amd64 as broken: the libgodot bootstrap calls unsetenv() on
// the GDNEXT_PLAY_RESULT entry between execve and the Go user-package
// init, so the example never sees the report path and the driver's
// readReport(reportPath) fails. The other GDNEXT_PLAY_* entries
// (PLAY, SCREENSHOT, HUD) survive — confirmed by emitting
// os.Environ() snapshots from package init and from finish(); only
// the *_RESULT name is consistently dropped, regardless of c.Env
// position, surrounding quotes, or the value itself. Suspected
// upstream in libgodot's OS_Unix / dlopen helper init path.
// The cell stays omitted until the libgodot init is fixed upstream
// or the example is taught to read the report path via a non-env
// channel (e.g. a stdin envelope like the android driver uses).
var QuirkLinuxAmd64LibGodotPlayEnvLoss = Quirk{
	Title:     "linux/amd64 libgodot play: GDNEXT_PLAY_RESULT is unsetenv'd by libgodot init before the example reads it",
	Scope:     QuirkCIPlayBroken,
	Hosts:     []string{Tuple(GOOSLinux, GOARCHAmd64)},
	LinkModes: []LinkMode{LibGodot},
	Reason: "Driver injects GDNEXT_PLAY_RESULT into c.Env alongside " +
		"PLAY, SCREENSHOT, and HUD (verified by logging every " +
		"GDNEXT_-prefixed c.Env entry pre-exec). In the child process, " +
		"os.Environ() at Go user-package init time shows PLAY, " +
		"SCREENSHOT, and HUD survived but RESULT is gone — not empty, " +
		"absent from the environ block entirely. Tested workarounds " +
		"that did NOT recover the entry: renaming the var (REPORT \u2192 " +
		"RESULT), changing the value (1 \u2192 'active' \u2192 example name), " +
		"reordering c.Env so RESULT is no longer adjacent to PLAY, " +
		"wrapping the value in literal '\"' bytes, removing the " +
		"xvfb-run shell wrapper. The other three vars round-trip in " +
		"every combination. Suspected libgodot bootstrap path " +
		"(OS_Unix init or the dlopen helper) calls unsetenv on the " +
		"name; cause not yet pinned upstream.",
	Result: []string{
		"the (linux/amd64 play host, linux/amd64 target, libgodot link) play cell is omitted from the play matrix",
		"the libgodot build itself stays green (assertDistributable verifies releases/linux/amd64/<example> exists)",
		"gdextension-mode play on linux/amd64 is unaffected and continues to cover the example",
		"cell turns green once libgodot stops dropping the env entry, or the example is taught to read the report path via a non-env channel",
	},
	Refs: []string{
		"https://github.com/rdlaitila/graphics.gd/actions/runs/28296813097",
	},
}

// allow-fail when built from a windows host; user builds on a local
// windows host may still succeed.
var QuirkWindowsDarwinBuildAccessDenied = Quirk{
	Title: "windows hosts: github windows-latest runner dylib link rename fails with `Access is denied`",
	Scope: QuirkCIBuildBroken,
	Hosts: []string{Tuple(GOOSWindows, GOARCHAmd64)},
	Reason: "Go's external linker writes `<out>~` then atomic-renames " +
		"to `<out>`; on windows-latest the rename consistently fails " +
		"with `rename darwin_<arch>.dylib~ darwin_<arch>.dylib: Access " +
		"is denied`. Cause not identified. What we tried, all without " +
		"effect: Add-MpPreference path/process/extension exclusions, " +
		"Set-MpPreference -DisableRealtimeMonitoring + behaviour/IOAV/" +
		"script/archive, Stop-Service WSearch, elevating the build " +
		"via gsudo, per-build retry. Cross-build darwin from a linux " +
		"or darwin host.",
	Result: []string{
		"the (windows host, darwin target) build cells are omitted from the matrix",
		"darwin/* targets continue to build green on linux and darwin hosts",
		"local builds on a user-owned windows host may still succeed",
	},
	Refs: []string{
		"https://github.com/rdlaitila/graphics.gd/actions/runs/28122819304/job/83279433625",
		"https://github.com/rdlaitila/graphics.gd/actions/runs/28133000447/job/83313886492",
	},
}

// QuirkIOSArm64TemplateLinkUndefined marks ios/arm64 builds as
// broken: the prebuilt libgodot.ios.release.xcframework served from
// release.graphics.gd references hundreds of symbols our bundled
// .tbd stubs and the template itself don't provide, so ld64.lld
// aborts the final Mach-O link.
var QuirkIOSArm64TemplateLinkUndefined = Quirk{
	Title: "ios/arm64 build: prebuilt libgodot template fails to link with ld64.lld",
	Scope: QuirkCIBuildBroken,
	Reason: "The iOS export bundles libgodot.ios.release.xcframework + " +
		"MoltenVK.xcframework and hands them to ld64.lld with " +
		"-syslibroot /dev/null and our minimal bundled .tbd stubs. " +
		"The 4.7 template references symbols our stubs do not " +
		"advertise (Metal counter set markers, NSError keys, " +
		"NSProcessInfo notification names, CADynamicRange*, " +
		"UISceneConfiguration, libc++ aligned new/delete, " +
		"std::to_string(long)), and \u2014 more fundamentally \u2014 " +
		"references SDL functions (_SDL_IsIPad, _SDL_IsAppleTV) " +
		"that should be defined inside libgodot.a itself but are " +
		"absent, suggesting the prebuilt template was compiled " +
		"without the SDL platform sources. ___kCFBooleanTrue also " +
		"reports as undefined even though our stub advertises it, " +
		"implying a name-mangling mismatch between MoltenVK's " +
		"Objective-C++ bridge and the stub format. Stub expansion " +
		"alone cannot resolve the SDL gap.",
	Result: []string{
		"the ios/arm64 build cell is omitted from the build matrix",
		"local builds will hit the same ld64.lld error until the " +
			"prebuilt libgodot.ios.release.xcframework is rebuilt " +
			"with SDL sources included and our bundled .tbd stubs " +
			"are regenerated from Apple's real iOS SDK tbds",
	},
}

// QuirkLibGodotWindowsMingwSconsArgSplit marks the windows libgodot
// recipes as broken because the recipe passes CC="zig cc -target ..."
// as a multi-word SCons argument. Upstream SCons parses that as
// separate positional tokens and drops platform=windows, so the
// compile fails at "Please run SCons again and select a valid
// platform". The fix is to plant a zig-cc forwarder shim (same
// pattern as the linux musl recipe uses via plantZigShims) so
// CC=cc / CXX=c++ resolve to single-token PATH lookups.
var QuirkLibGodotWindowsMingwSconsArgSplit = Quirk{
	Title:  "windows libgodot: recipe's multi-word CC= confuses SCons and drops platform=",
	Scope:  QuirkCIBuildBroken,
	Reason: "windowsMingwRecipe passes CC=\"zig cc -target x86_64-windows-gnu\" as a single ARGUMENTS entry; SCons's argv parser treats the whitespace as a delimiter and reads the remaining tokens as positional args, which leaves platform= empty and bails with `Please run SCons again and select a valid platform: platform=<string>`. Needs plantZigShims-style forwarders (cc, c++, ar, ranlib) prepended to PATH so the recipe can pass bare CC=cc CXX=c++ single-token args.",
	Result: []string{
		"windows/amd64 and windows/arm64 libgodot build cells surface as allow-fail in the matrix",
		"the linux + darwin libgodot cells stay unaffected",
	},
}

// QuirkLibGodotDarwinMoltenVKMissing marks the darwin libgodot recipes
// as broken because they pass vulkan=yes but the macos-latest GitHub
// runner doesn't ship the MoltenVK SDK. Either drop vulkan=yes (metal
// alone is enough for darwin production builds) or install MoltenVK
// via `brew install --cask vulkan-sdk` before scons runs.
var QuirkLibGodotDarwinMoltenVKMissing = Quirk{
	Title:  "darwin libgodot: recipe requires vulkan_sdk_path but MoltenVK isn't installed on macos-latest",
	Scope:  QuirkCIBuildBroken,
	Reason: "macosRecipe passes vulkan=yes to enable Godot's MoltenVK-backed vulkan driver. macos-latest runners don't ship MoltenVK, so upstream Godot's platform/macos/detect.py aborts with `MoltenVK SDK installation directory not found, use 'vulkan_sdk_path' SCons parameter to specify SDK path.` before compilation starts. Fix by adding a MoltenVK install step (`brew install --cask vulkan-sdk`) or by dropping vulkan=yes (metal alone covers darwin production paths).",
	Result: []string{
		"darwin/amd64 and darwin/arm64 libgodot build cells surface as allow-fail in the matrix",
		"the linux + windows libgodot cells stay unaffected",
	},
}

// QuirkLibGodotLinuxMuslExecinfoMissing marks the linux musl EDITOR
// libgodot recipes as broken because the linuxbsd crash_handler pulls
// in <execinfo.h>, which musl doesn't ship. The release template
// variant builds fine (crash_handler is a no-op in release); only
// the editor variant hits the header. Needs a scons flag or upstream
// patch to gate execinfo behind __GLIBC__.
var QuirkLibGodotLinuxMuslExecinfoMissing = Quirk{
	Title:     "linux/musl editor libgodot: crash_handler_linuxbsd.cpp includes <execinfo.h>, missing on musl",
	Scope:     QuirkCIBuildBroken,
	LinkModes: nil, // applies regardless of link mode
	Reason:    "platform/linuxbsd/crash_handler_linuxbsd.cpp:49 does `#include <execinfo.h>` unconditionally when compiling the editor. musl deliberately does not provide execinfo.h (it's a glibc-specific backtrace API). Legacy musl builds must have carried an upstream patch or a scons flag to compile the crash handler out; needs re-derivation. Fix candidates: patch the include site with `#ifdef __GLIBC__`, or set `disable_exceptions=yes debug_symbols=no` (already set), or upstream a scons flag that maps to `-DNO_EXECINFO` in the compilation unit.",
	Result: []string{
		"linux/amd64 musl editor and linux/arm64 musl editor libgodot cells surface as allow-fail",
		"the linux musl release/template cells stay unaffected (crash_handler compiles in a no-op form)",
		"the linux glibc cells stay unaffected (glibc ships execinfo.h)",
	},
}

func (t QuirkScope) String() string {
	switch t {
	case QuirkInformational:
		return "informational"
	case QuirkRuntime:
		return "runtime"
	case QuirkBuildFlaky:
		return "build-flaky"
	case QuirkCIBuildBroken:
		return "ci-build-broken"
	case QuirkCIPlayBroken:
		return "ci-play-broken"
	}
	return "?"
}

func (t QuirkScope) MarshalText() ([]byte, error) { return []byte(t.String()), nil }

func (t Quirk) AppliesToHost(hostGOOS, hostGOARCH string) bool {
	if len(t.Hosts) == 0 {
		return true
	}
	tuple := Tuple(hostGOOS, hostGOARCH)
	for _, h := range t.Hosts {
		if h == tuple {
			return true
		}
	}
	return false
}

// AppliesToCompat reports whether t's Compat list matches layer.
// Empty Compat means every layer (including native, represented by
// the empty string).
func (t Quirk) AppliesToCompat(layer string) bool {
	if len(t.Compat) == 0 {
		return true
	}
	for _, c := range t.Compat {
		if c == layer {
			return true
		}
	}
	return false
}

// AppliesToLinkMode reports whether t's LinkModes list matches mode.
// Empty LinkModes means every mode.
func (t Quirk) AppliesToLinkMode(mode LinkMode) bool {
	if len(t.LinkModes) == 0 {
		return true
	}
	for _, m := range t.LinkModes {
		if m == mode {
			return true
		}
	}
	return false
}

// CIBlockedFor reports whether p carries a QuirkCIBuildBroken
// matching the given build host. The matrix generator uses it to
// omit the (host, target) cell entirely: build-broken means we
// don't want CI noise from a cell we know won't work, and the
// quirk row in the summary is the contract that explains why.
func (t Platform) CIBlockedFor(hostGOOS, hostGOARCH string) bool {
	for _, q := range t.Quirks {
		if q.Scope == QuirkCIBuildBroken && q.AppliesToHost(hostGOOS, hostGOARCH) {
			return true
		}
	}
	return false
}

// PlayBlockedFor reports whether p carries a QuirkCIPlayBroken
// matching the given (play host, compat layer, link mode) triple.
// Used by the play-matrix generator to omit cells we already know
// don't reach the play-bot signal.
func (t Platform) PlayBlockedFor(playHostGOOS, playHostGOARCH, compat string, mode LinkMode) bool {
	for _, q := range t.Quirks {
		if q.Scope != QuirkCIPlayBroken {
			continue
		}
		if !q.AppliesToHost(playHostGOOS, playHostGOARCH) {
			continue
		}
		if !q.AppliesToCompat(compat) {
			continue
		}
		if !q.AppliesToLinkMode(mode) {
			continue
		}
		return true
	}
	return false
}

// QuirkEntry pairs a Quirk with the platforms it is attached to.
// Returned by KnownQuirks; CLI + summary renderers consume the same value.
type QuirkEntry struct {
	Quirk     Quirk    `json:"quirk"`
	Platforms []string `json:"platforms"`
}

// KnownQuirks walks PlatformMatrix and returns every distinct Quirk
// (deduplicated by Title) with its attached platform tuples. Scope
// descending then Title ascending so renderers don't re-sort.
func KnownQuirks() []QuirkEntry {
	by := map[string]*QuirkEntry{}
	var order []string
	add := func(q Quirk, tuple string) {
		e, ok := by[q.Title]
		if !ok {
			e = &QuirkEntry{Quirk: q}
			by[q.Title] = e
			order = append(order, q.Title)
		}
		e.Platforms = append(e.Platforms, tuple)
	}
	for _, p := range PlatformMatrix {
		for _, q := range p.Quirks {
			add(q, p.Tuple())
		}
	}
	// Recipe-scoped quirks (libgodot per-cell caveats) render with a
	// `libgodot(...)` prefix so consumers can tell them apart from
	// Platform-scoped rows.
	for _, r := range LibGodotMatrix {
		for _, q := range r.Quirks {
			add(q, libgodotRecipeTuple(r))
		}
	}
	out := make([]QuirkEntry, 0, len(order))
	for _, title := range order {
		e := by[title]
		sort.Strings(e.Platforms)
		out = append(out, *e)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Quirk.Scope != out[j].Quirk.Scope {
			return out[i].Quirk.Scope > out[j].Quirk.Scope
		}
		return out[i].Quirk.Title < out[j].Quirk.Title
	})
	return out
}

// libgodotRecipeTuple renders a LibGodotRecipe as a stable tuple for
// KnownQuirks / summary output. Format: `libgodot(<goos>/<goarch>[/<libc>]/<variant>)`.
func libgodotRecipeTuple(r LibGodotRecipe) string {
	tuple := "libgodot(" + r.GOOS + "/" + r.GOARCH
	if r.LibC != "" {
		tuple += "/" + r.LibC
	}
	if r.Editor {
		tuple += "/editor"
	} else {
		tuple += "/template_release"
	}
	return tuple + ")"
}
