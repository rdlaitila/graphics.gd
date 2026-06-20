package product

import (
	"fmt"
	"strings"
)

// LinkMode is a bitmask of linking recipes graphics.gd can produce for
// a target. A Platform may support more than one; the user picks via
// the --link CLI flag.
type LinkMode uint8

const (
	// GDExtension produces a shared library Godot loads via
	// library.gdextension at runtime.
	GDExtension LinkMode = 1 << iota
	// LibGodot produces a static executable that embeds the engine by
	// linking against libgodot.$(GOOS).$(GOARCH).$(EXT).
	LibGodot
)

// Has reports whether m contains every bit in want.
func (m LinkMode) Has(want LinkMode) bool { return m&want == want }

var linkModeOrder = []struct {
	bit  LinkMode
	name string
}{
	{GDExtension, "gdextension"},
	{LibGodot, "libgodot"},
}

// String renders the bitmask as a `+`-joined list; zero renders as "?".
func (m LinkMode) String() string {
	if m == 0 {
		return "?"
	}
	parts := make([]string, 0, 2)
	for _, e := range linkModeOrder {
		if m.Has(e.bit) {
			parts = append(parts, e.name)
		}
	}
	return strings.Join(parts, "+")
}

func (m LinkMode) MarshalText() ([]byte, error) { return []byte(m.String()), nil }

// UnmarshalText accepts the same `+`-joined form String produces.
func (m *LinkMode) UnmarshalText(b []byte) error {
	*m = 0
	if len(b) == 0 || string(b) == "?" {
		return nil
	}
	for _, part := range strings.Split(string(b), "+") {
		switch part {
		case "gdextension":
			*m |= GDExtension
		case "libgodot":
			*m |= LibGodot
		default:
			return fmt.Errorf("product: unknown LinkMode %q", part)
		}
	}
	return nil
}

// LinkModeMatrix is the canonical list of LinkMode tokens recognised
// on the CLI (--link flag).
var LinkModeMatrix = []string{
	"gdextension",
	"libgodot",
}

// GOOSLinkModeDefaults picks the default LinkMode for a target GOOS
// when --link is unset.
var GOOSLinkModeDefaults = map[string]LinkMode{
	GOOSLinux:     GDExtension,
	GOOSWindows:   GDExtension,
	GOOSDarwin:    GDExtension,
	GOOSIOS:       GDExtension,
	GOOSAndroid:   GDExtension,
	GOOSMetaQuest: GDExtension,
	GOOSJS:        GDExtension,
}

// ParseLinkMode decodes a CLI token into a single bit. Empty returns
// 0 so callers can defer to GOOSLinkModeDefaults.
func ParseLinkMode(s string) (LinkMode, error) {
	if s == "" {
		return 0, nil
	}
	var m LinkMode
	if err := m.UnmarshalText([]byte(s)); err != nil {
		return 0, err
	}
	if m != GDExtension && m != LibGodot {
		return 0, fmt.Errorf("product: --link expects a single mode (gdextension or libgodot), got %q", s)
	}
	return m, nil
}
