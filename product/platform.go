package product

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// Platform is one row in the graphics.gd support matrix: a canonical
// (GOOS, GOARCH) pair together with the metadata gdnext + downstream
// tooling needs to reason about it.
type Platform struct {
	XMLName    xml.Name    `json:"-"                     xml:"platform"                     yaml:"-"`
	Title      string      `json:"title,omitempty"       xml:"title,attr,omitempty"         yaml:"title,omitempty"`
	GOOS       string      `json:"goos"                  xml:"goos"                         yaml:"goos"`
	GOARCH     string      `json:"goarch"                xml:"goarch"                       yaml:"goarch"`
	Aliases    []string    `json:"aliases,omitempty"     xml:"aliases>alias,omitempty"      yaml:"aliases,omitempty"`
	Kind       Kind        `json:"kind"                  xml:"kind,attr"                    yaml:"kind"`
	Status     Status      `json:"status"                xml:"status,attr"                  yaml:"status"`
	LinkModes  LinkMode    `json:"link_modes,omitempty"  xml:"link_modes,attr,omitempty"    yaml:"link_modes,omitempty"`
	BuildHosts []BuildHost `json:"build_hosts,omitempty" xml:"build_hosts,omitempty"        yaml:"build_hosts,omitempty"`
	PlayHosts  []PlayHost  `json:"play_hosts,omitempty"  xml:"play_hosts,omitempty"         yaml:"play_hosts,omitempty"`
	BuildTools []Toolchain `json:"build_tools,omitempty" xml:"build_tools,omitempty" yaml:"build_tools,omitempty"`
	Renderers  []string    `json:"renderers,omitempty"   xml:"renderers>renderer,omitempty" yaml:"renderers,omitempty"`
	Notes      string      `json:"notes,omitempty"       xml:"notes,omitempty"              yaml:"notes,omitempty"`
	Quirks     []Quirk     `json:"quirks,omitempty"      xml:"quirks>quirk,omitempty"       yaml:"quirks,omitempty"`
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

// DisplayTitle returns the human-friendly label suitable for user-facing
// messaging ("Android Arm64", "Windows x86_64"). Falls back to a
// "<GOOS> <GOARCH>" construction when Title is unset — callers can
// rely on this never returning the empty string.
func (t Platform) DisplayTitle() string {
	if t.Title != "" {
		return t.Title
	}
	return t.GOOS + " " + t.GOARCH
}

// Tuple returns the canonical "goos/goarch" platform identifier (the
// same shape Docker / buildx / `go env` use, e.g. "linux/amd64").
// Use this anywhere you'd otherwise hand-format the pair.
func (t Platform) Tuple() string {
	return Tuple(t.GOOS, t.GOARCH)
}

// Names returns every name (canonical + aliases) that should resolve to
// this row. Convenience for completion + lookup callers.
func (t Platform) Names() []string {
	out := make([]string, 0, 1+len(t.Aliases))
	out = append(out, t.GOOS)
	out = append(out, t.Aliases...)
	return out
}

// CanPlayOn reports whether this target can be launched + ticked
// headlessly on the given host. An empty PlayHosts is treated as
// "no host can play this yet" — the play matrix skips the row.
func (t Platform) CanPlayOn(hostGOOS, hostGOARCH string) bool {
	for _, host := range t.PlayHosts {
		if host.GOOS == hostGOOS && host.GOARCH == hostGOARCH {
			return true
		}
	}
	return false
}

// CanBuildOn reports whether this target can be built from the host
// (hostGOOS, hostGOARCH). An empty BuildHosts is treated as "any host",
// which is the right default for zig-cross-compilable targets. Set
// BuildHosts when a target genuinely needs a specific host — darwin
// (no zig cross path), musl (linux-only build chain), etc.
func (t Platform) CanBuildOn(hostGOOS, hostGOARCH string) bool {
	if len(t.BuildHosts) == 0 {
		return false
	}
	for _, host := range t.BuildHosts {
		if host.GOOS == hostGOOS && host.GOARCH == hostGOARCH {
			return true
		}
	}
	return false
}

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

// Tuple is the package-level form for callers that have a (goos, goarch)
// pair without a Platform value handy — typically when reporting the
// host (runtime.GOOS / runtime.GOARCH).
func Tuple(goos, goarch string) string {
	if goarch == "" {
		return goos
	}
	return goos + "/" + goarch
}

// Targets returns the subset of Matrix that can be built for.
func Targets() []Platform {
	out := make([]Platform, 0, len(PlatformMatrix))
	for _, platform := range PlatformMatrix {
		if platform.Kind.Has(Target) {
			out = append(out, platform)
		}
	}
	return out
}

// Hosts returns the subset of Matrix where gdnext can run.
func Hosts() []Platform {
	out := make([]Platform, 0, len(PlatformMatrix))
	for _, platform := range PlatformMatrix {
		if platform.Kind.Has(Host) {
			out = append(out, platform)
		}
	}
	return out
}

// PlatformGOOSes returns the unique canonical GOOS values declared by
// PlatformMatrix, in iteration order. Aliases (web, macos, iphone, ...)
// are not included — those are accepted by FindPlatformByName but not
// listed here.
func PlatformGOOSes() []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(PlatformMatrix))
	for _, p := range PlatformMatrix {
		if seen[p.GOOS] {
			continue
		}
		seen[p.GOOS] = true
		out = append(out, p.GOOS)
	}
	return out
}

// PlatformGOARCHes returns the unique GOARCH values declared by
// PlatformMatrix, in iteration order.
func PlatformGOARCHes() []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(PlatformMatrix))
	for _, p := range PlatformMatrix {
		if seen[p.GOARCH] {
			continue
		}
		seen[p.GOARCH] = true
		out = append(out, p.GOARCH)
	}
	return out
}

// FindPlatformByTargetEnv returns the first Matrix row whose (GOOS, GOARCH) matches the
// pair exactly. An empty GOARCH matches any architecture for the same
// GOOS — useful when the caller only knows the operating system.
func FindPlatformByTargetEnv(goos, goarch string) (Platform, bool) {
	for _, platform := range PlatformMatrix {
		if platform.GOOS != goos {
			continue
		}
		if goarch == "" || platform.GOARCH == goarch {
			return platform, true
		}
	}
	return Platform{}, false
}

// FindPlatformByName maps a user-supplied alias (case-insensitive) to its canonical
// row. The match space is the union of every Platform's Names. Returns
// the first matching row; aliases are required to be unique across the
// matrix (enforced by TestAliasesUnique).
func FindPlatformByName(alias string) (Platform, bool) {
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
