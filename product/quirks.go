package product

import "sort"

// Quirk is a structured caveat attached to a Platform. Empty Hosts
// means the quirk applies to every build host (build-scope quirks)
// or every play host (play-scope quirks) for the Platform. Empty
// Compat (play-scope only) means every compat layer on those hosts.
type Quirk struct {
	Title  string     `json:"title"            xml:"title,attr"             yaml:"title"`
	Scope  QuirkScope `json:"scope"            xml:"scope,attr"             yaml:"scope"`
	Hosts  []string   `json:"hosts,omitempty"  xml:"hosts>host,omitempty"  yaml:"hosts,omitempty"`
	Compat []string   `json:"compat,omitempty" xml:"compat>layer,omitempty" yaml:"compat,omitempty"`
	Reason string     `json:"reason"           xml:"reason"                 yaml:"reason"`
	Result []string   `json:"result,omitempty" xml:"result>item,omitempty" yaml:"result,omitempty"`
	Refs   []string   `json:"refs,omitempty"   xml:"refs>ref,omitempty"    yaml:"refs,omitempty"`
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
	QuirkCIPlayAllowFail
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

// QuirkWebWasmGDExtensionPlayBroken marks js/wasm play under headless
// chrome/firefox as allow-fail: the engine boots and renders a frame
// (so we still get a screenshot artefact for the workflow summary)
// but the stock Godot 4.7 web export template ships without
// GDExtension support, so any graphics.gd extension call hits a
// null function pointer and the page crashes before the play-bot
// can write its report.
var QuirkWebWasmGDExtensionPlayBroken = Quirk{
	Title:  "js/wasm play under chrome/firefox: stock Godot 4.7 web template lacks GDExtension support",
	Scope:  QuirkCIPlayAllowFail,
	Hosts:  []string{Tuple(GOOSLinux, GOARCHAmd64)},
	Compat: []string{"chrome", "firefox"},
	Reason: "The browser cell launches via Playwright with COEP/COOP " +
		"headers, the wasm bundle loads, and Godot reaches the initial " +
		"frame (chrome+SwiftShader on linux). The bundle's own log line " +
		"reads `Build configuration: Emscripten ..., no GDExtension " +
		"support.` \u2014 the stock Godot 4.7 web export template is " +
		"compiled without GDExtension. Any graphics.gd runtime call that " +
		"crosses the extension boundary resolves to a null function " +
		"pointer and crashes the page before the play-bot can emit its " +
		"GDNEXT_PLAY_REPORT line. Upstream's own web tests sidestep this " +
		"by linking via libgodot (statically embedding the engine in " +
		"library.wasm), but no libgodot.web.wasm artefact is published at " +
		"release.graphics.gd yet, so gdnext build can't take that route.",
	Result: []string{
		"the (js/wasm, chrome) and (js/wasm, firefox) play cells run with continue-on-error",
		"each cell still uploads a play-screenshot.png (the driver captures it after the wasm crash) for the workflow summary",
		"the js/wasm build itself stays green; the cell turns green automatically once the upstream Godot web template ships with GDExtension support, or once a libgodot.web.wasm artefact is published and gdnext build is taught to use it",
	},
	Refs: []string{
		"https://github.com/godotengine/godot/issues/100789",
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
		"darwin/* targets stay Supported|Quirky on a windows host",
		"CI runs (windows host, darwin target) cells with continue-on-error so failures don't fail the workflow",
		"local builds on a user-owned windows host may still succeed",
	},
	Refs: []string{
		"https://github.com/rdlaitila/graphics.gd/actions/runs/28122819304/job/83279433625",
		"https://github.com/rdlaitila/graphics.gd/actions/runs/28133000447/job/83313886492",
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
	case QuirkCIPlayAllowFail:
		return "ci-play-allow-fail"
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

// CIBlockedFor reports whether p carries a QuirkCIBuildBroken
// matching the given build host.
func (t Platform) CIBlockedFor(hostGOOS, hostGOARCH string) bool {
	for _, q := range t.Quirks {
		if q.Scope == QuirkCIBuildBroken && q.AppliesToHost(hostGOOS, hostGOARCH) {
			return true
		}
	}
	return false
}

// PlayBlockedFor reports whether p carries a QuirkCIPlayBroken
// matching the given (play host, compat layer) pair. Used by the
// play-matrix generator to omit cells we already know don't reach
// the play-bot signal.
func (t Platform) PlayBlockedFor(playHostGOOS, playHostGOARCH, compat string) bool {
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
		return true
	}
	return false
}

// PlayAllowFailFor reports whether p carries a QuirkCIPlayAllowFail
// matching the given (play host, compat layer) pair. Used by the
// play-matrix generator to mark cells that should still run (and
// upload artefacts like the screenshot) but not fail the workflow
// when they don't reach the play-bot signal.
func (t Platform) PlayAllowFailFor(playHostGOOS, playHostGOARCH, compat string) bool {
	for _, q := range t.Quirks {
		if q.Scope != QuirkCIPlayAllowFail {
			continue
		}
		if !q.AppliesToHost(playHostGOOS, playHostGOARCH) {
			continue
		}
		if !q.AppliesToCompat(compat) {
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
	for _, p := range PlatformMatrix {
		for _, q := range p.Quirks {
			e, ok := by[q.Title]
			if !ok {
				e = &QuirkEntry{Quirk: q}
				by[q.Title] = e
				order = append(order, q.Title)
			}
			e.Platforms = append(e.Platforms, p.Tuple())
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
