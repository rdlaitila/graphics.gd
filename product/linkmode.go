package product

import (
	"fmt"
	"strings"
)

// LinkMode is a bitmask of linking recipes graphics.gd can produce for
// a target. A Platform may support more than one; the user picks via
// the --link CLI flag. See docs/plans/gdnext.md for the underlying
// model.
type LinkMode uint8

const (
	// GDExtension produces a shared library (.so / .dll / .dylib, or
	// .a on iOS) that Godot loads at runtime via library.gdextension.
	// Godot is a separate process installed on the user's machine.
	GDExtension LinkMode = 1 << iota
	// LibGodot produces a single statically-linked executable that
	// embeds the Godot engine itself, by linking the Go code against
	// a per-target libgodot.$(GOOS).$(GOARCH).$(EXT) archive.
	LibGodot
)

// Has reports whether m contains every bit in want.
func (m LinkMode) Has(want LinkMode) bool { return m&want == want }

// linkModeOrder is the rendering order of bits for String() /
// MarshalText, matching how gdnext platform tables read.
var linkModeOrder = []struct {
	bit  LinkMode
	name string
}{
	{GDExtension, "gdextension"},
	{LibGodot, "libgodot"},
}

// String renders the bitmask as a `+`-joined list ("gdextension",
// "libgodot", "gdextension+libgodot"). Zero value renders as "?".
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

// MarshalText implements encoding.TextMarshaler.
func (m LinkMode) MarshalText() ([]byte, error) { return []byte(m.String()), nil }

// UnmarshalText accepts the same `+`-joined form String produces, plus
// the bare bit names "gdextension" and "libgodot".
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
// on the CLI (--link flag) and elsewhere. Order matches linkModeOrder
// so help text and table columns stay in sync.
var LinkModeMatrix = []string{
	"gdextension",
	"libgodot",
}

// GOOSLinkModeDefaults picks the default LinkMode for a given target
// GOOS when --link is not set. Today every entry is GDExtension; this
// table is the single change point if the project ever flips a host's
// default to LibGodot.
var GOOSLinkModeDefaults = map[string]LinkMode{
	GOOSLinux:     GDExtension,
	GOOSWindows:   GDExtension,
	GOOSDarwin:    GDExtension,
	GOOSIOS:       GDExtension,
	GOOSAndroid:   GDExtension,
	GOOSMetaQuest: GDExtension,
	GOOSJS:        GDExtension,
}

// ParseLinkMode decodes a CLI / env-var token ("gdextension" or
// "libgodot") into the corresponding bit. Empty input returns 0 so
// callers can defer to GOOSLinkModeDefaults.
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
