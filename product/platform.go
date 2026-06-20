package product

import (
	"encoding/xml"
	"fmt"
	"runtime"
	"strings"
)

// Platform is one row in the graphics.gd support matrix: a canonical
// (GOOS, GOARCH) pair together with the metadata gdnext + downstream
// tooling needs to reason about it.
type Platform struct {
	XMLName    xml.Name  `json:"-"                     xml:"platform"                     yaml:"-"`
	Title      string    `json:"title,omitempty"       xml:"title,attr,omitempty"         yaml:"title,omitempty"`
	GOOS       string    `json:"goos"                  xml:"goos"                         yaml:"goos"`
	GOARCH     string    `json:"goarch"                xml:"goarch"                       yaml:"goarch"`
	Aliases    []string  `json:"aliases,omitempty"     xml:"aliases>alias,omitempty"      yaml:"aliases,omitempty"`
	Kind       Kind      `json:"kind"                  xml:"kind,attr"                    yaml:"kind"`
	Status     Status    `json:"status"                xml:"status,attr"                  yaml:"status"`
	BuildHosts Platforms `json:"build_hosts,omitempty" xml:"build_hosts,omitempty"        yaml:"build_hosts,omitempty"`
	Renderers  []string  `json:"renderers,omitempty"   xml:"renderers>renderer,omitempty" yaml:"renderers,omitempty"`
	Notes      string    `json:"notes,omitempty"       xml:"notes,omitempty"              yaml:"notes,omitempty"`
}

// DisplayTitle returns the human-friendly label suitable for user-facing
// messaging ("Android Arm64", "Windows x86_64"). Falls back to a
// "<GOOS> <GOARCH>" construction when Title is unset — callers can
// rely on this never returning the empty string.
func (p Platform) DisplayTitle() string {
	if p.Title != "" {
		return p.Title
	}
	return p.GOOS + " " + p.GOARCH
}

// Tuple returns the canonical "goos/goarch" platform identifier (the
// same shape Docker / buildx / `go env` use, e.g. "linux/amd64").
// Use this anywhere you'd otherwise hand-format the pair.
func (p Platform) Tuple() string {
	return Tuple(p.GOOS, p.GOARCH)
}

// Tuple is the package-level form for callers that have a (goos, goarch)
// pair without a Platform value handy — typically when reporting the
// host (runtime.GOOS / runtime.GOARCH).
func Tuple(goos, goarch string) string {
	if goarch == "" {
		return goos
	}
	return goos + "/" + goarch
}

// BuildEnv is a (host, target) pair: the platform gdnext itself is
// running on, and the platform a build is producing artefacts for.
// Used by every verb in the toolchain / build pipeline that needs
// both — the pattern was repeated inline across ~20 sites before this
// lifted out.
type BuildEnv struct {
	HostGOOS, HostGOARCH     string
	TargetGOOS, TargetGOARCH string
}

// NewBuildEnv returns a BuildEnv whose host AND target are
// runtime.GOOS / runtime.GOARCH — the right starting point for a
// no-args invocation before any --goos / --goarch flag is folded in.
func NewBuildEnv() BuildEnv {
	return BuildEnv{
		HostGOOS:     runtime.GOOS,
		HostGOARCH:   runtime.GOARCH,
		TargetGOOS:   runtime.GOOS,
		TargetGOARCH: runtime.GOARCH,
	}
}

// ResolveEnv returns a BuildEnv for the runtime host targeting
// (goos, goarch). Empty target strings fall back to the host, so
// ResolveEnv("", "") == NewBuildEnv().
func ResolveEnv(goos, goarch string) BuildEnv {
	env := NewBuildEnv()
	if goos != "" {
		env.TargetGOOS = goos
	}
	if goarch != "" {
		env.TargetGOARCH = goarch
	}
	return env
}

// HostTuple returns the host as a Platforms-friendly tuple ("linux/amd64").
func (e BuildEnv) HostTuple() string { return Tuple(e.HostGOOS, e.HostGOARCH) }

// TargetTuple returns the target as a tuple ("android/arm64").
func (e BuildEnv) TargetTuple() string { return Tuple(e.TargetGOOS, e.TargetGOARCH) }

// Platforms is an OR-set of (GOOS, GOARCH) constraints. Used wherever a
// caller needs to express "this subset of the matrix" — most notably by
// Toolchain (Required: which build targets need this tool; Available:
// which hosts can obtain it).
//
// An empty Platforms matches nothing. GOOS containing "all" is the
// shorthand for "every GOOS"; an empty GOARCH list inside a non-empty
// GOOS means "any architecture".
type Platforms struct {
	GOOS   []string `json:"goos,omitempty"   xml:"goos,omitempty"   yaml:"goos,omitempty"`
	GOARCH []string `json:"goarch,omitempty" xml:"goarch,omitempty" yaml:"goarch,omitempty"`
}

// AllPlatforms is shorthand for a Platforms set that matches every host
// or target. Used by entries that have no restriction (go, zig, godot).
var AllPlatforms = Platforms{GOOS: []string{"all"}}

// Matches reports whether (goos, goarch) is in the constraint set.
func (p Platforms) Matches(goos, goarch string) bool {
	if len(p.GOOS) == 0 {
		return false
	}
	var osMatch bool
	for _, o := range p.GOOS {
		if o == "all" || o == goos {
			osMatch = true
			break
		}
	}
	if !osMatch {
		return false
	}
	if len(p.GOARCH) == 0 {
		return true
	}
	for _, a := range p.GOARCH {
		if a == goarch {
			return true
		}
	}
	return false
}

// Kind is a bitmask of platform roles. Use Has to test for membership.
type Kind uint8

const (
	// Target means graphics.gd can build for this platform.
	Target Kind = 1 << iota
	// Host means gdnext itself can run on this platform (the build
	// driver, not the produced artefact).
	Host
)

// Has reports whether k contains every bit in want.
func (k Kind) Has(want Kind) bool { return k&want == want }

// String renders the bitmask as a human-readable label used in the
// `gdnext platforms` table and as the encoding/json + encoding/xml
// text form (see MarshalText).
func (k Kind) String() string {
	switch k {
	case Host | Target:
		return "host+target"
	case Host:
		return "host"
	case Target:
		return "target"
	default:
		return "?"
	}
}

// MarshalText implements encoding.TextMarshaler so Kind serialises to
// JSON / XML as its String() form rather than as a uint8.
func (k Kind) MarshalText() ([]byte, error) { return []byte(k.String()), nil }

// UnmarshalText is the symmetric reader; tolerant of the few spellings
// the matrix actually produces.
func (k *Kind) UnmarshalText(b []byte) error {
	switch string(b) {
	case "host+target", "target+host":
		*k = Host | Target
	case "host":
		*k = Host
	case "target":
		*k = Target
	default:
		return fmt.Errorf("product: unknown Kind %q", b)
	}
	return nil
}

// Names returns every name (canonical + aliases) that should resolve to
// this row. Convenience for completion + lookup callers.
func (p Platform) Names() []string {
	out := make([]string, 0, 1+len(p.Aliases))
	out = append(out, p.GOOS)
	out = append(out, p.Aliases...)
	return out
}

// CanBuildOn reports whether this target can be built from the host
// (hostGOOS, hostGOARCH). An empty BuildHosts is treated as "any host",
// which is the right default for zig-cross-compilable targets. Set
// BuildHosts when a target genuinely needs a specific host — darwin
// (no zig cross path), musl (linux-only build chain), etc.
func (p Platform) CanBuildOn(hostGOOS, hostGOARCH string) bool {
	if len(p.BuildHosts.GOOS) == 0 {
		return true
	}
	return p.BuildHosts.Matches(hostGOOS, hostGOARCH)
}

// Targets returns the subset of Matrix that can be built for.
func Targets() []Platform {
	out := make([]Platform, 0, len(PlatformMatrix))
	for _, p := range PlatformMatrix {
		if p.Kind.Has(Target) {
			out = append(out, p)
		}
	}
	return out
}

// Hosts returns the subset of Matrix where gdnext can run.
func Hosts() []Platform {
	out := make([]Platform, 0, len(PlatformMatrix))
	for _, p := range PlatformMatrix {
		if p.Kind.Has(Host) {
			out = append(out, p)
		}
	}
	return out
}

// Lookup returns the first Matrix row whose (GOOS, GOARCH) matches the
// pair exactly. An empty GOARCH matches any architecture for the same
// GOOS — useful when the caller only knows the operating system.
func Lookup(goos, goarch string) (Platform, bool) {
	for _, p := range PlatformMatrix {
		if p.GOOS != goos {
			continue
		}
		if goarch == "" || p.GOARCH == goarch {
			return p, true
		}
	}
	return Platform{}, false
}

// Resolve maps a user-supplied alias (case-insensitive) to its canonical
// row. The match space is the union of every Platform's Names. Returns
// the first matching row; aliases are required to be unique across the
// matrix (enforced by TestAliasesUnique).
func Resolve(alias string) (Platform, bool) {
	alias = strings.ToLower(alias)
	for _, p := range PlatformMatrix {
		for _, n := range p.Names() {
			if strings.ToLower(n) == alias {
				return p, true
			}
		}
	}
	return Platform{}, false
}
