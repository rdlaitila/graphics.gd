package product

import "sort"

// Quirk is a structured caveat attached to a Platform. Empty Hosts
// means the quirk applies to every build host for the Platform.
type Quirk struct {
	Title  string     `json:"title"            xml:"title,attr"             yaml:"title"`
	Scope  QuirkScope `json:"scope"            xml:"scope,attr"             yaml:"scope"`
	Hosts  []string   `json:"hosts,omitempty"  xml:"hosts>host,omitempty"  yaml:"hosts,omitempty"`
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
)

// QuirkWindowsDarwinBuildAccessDenied marks darwin/* targets as
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
