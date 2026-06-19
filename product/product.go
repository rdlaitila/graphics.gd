// Package product is the single source of truth for the platforms,
// architectures, runtimes, toolchains, and other metadata graphics.gd
// ships across. It is pure data + lookup helpers: no other graphics.gd
// package imports are allowed so every other layer (ci, cli, builders,
// tooling, docs, examples, downstream callers) can depend on it without
// circular-import risk.
package product

import (
	"fmt"
	"strings"
)

// Status is a bitmask of support attributes graphics.gd advertises for
// a matrix entry (platform row, toolchain row, future rows). Supported
// is the headline bit — when set, gdnext covers the entry in CI and we
// treat regressions as bugs. The other bits are modifiers that describe
// *how* it is supported (or why it isn't):
//
//   - Supported | Stable        rock-solid, no known issues
//   - Supported | Quirky        works with documented caveats (see Notes)
//   - Supported | Experimental  no CI, no stability promise
//   - Supported | Deprecated    still maintained while phasing out
//   - Experimental | Broken     known not to build or run; CI may fail
//
// An entry without Supported set is unsupported by definition;
// downstream gates should check Status.Has(Supported) to decide whether
// to act on it. The zero value renders as "?" — a fill-me-in signal.
type Status uint8

const (
	// Supported is the headline bit: CI covers this entry and we ship
	// fixes for regressions.
	Supported Status = 1 << iota
	// Stable modifier: rock-solid, exercised by every release.
	Stable
	// Quirky modifier: builds and runs but has known oddities
	// (renderer fallbacks, missing features, upstream bugs) that
	// callers need to design around. See the Notes field where present.
	Quirky
	// Experimental modifier: build works but lacks CI / has known gaps;
	// usable, but no stability promise.
	Experimental
	// Deprecated modifier: the entry is on the way out and will be removed.
	Deprecated
	// Broken modifier: the entry is known not to build or run.
	// Usually set alone (without Supported) so users see it isn't an
	// oversight; CI is allowed to fail on Broken entries.
	Broken
)

// Has reports whether s contains every bit in want.
func (s Status) Has(want Status) bool { return s&want == want }

// statusOrder is the rendering order of bits for String() and
// MarshalText(): headline first, then modifiers. Determines the
// shape of `"supported+stable"`, `"supported+quirky"`, etc.
var statusOrder = []struct {
	bit  Status
	name string
}{
	{Supported, "supported"},
	{Stable, "stable"},
	{Quirky, "quirky"},
	{Experimental, "experimental"},
	{Deprecated, "deprecated"},
	{Broken, "broken"},
}

// String renders the bitmask as a `+`-joined lowercase list used in the
// `gdnext platforms` / `gdnext toolchain` tables and as the encoding/
// json + encoding/xml text form (see MarshalText). The zero value
// renders as "?".
func (s Status) String() string {
	if s == 0 {
		return "?"
	}
	parts := make([]string, 0, 2)
	for _, e := range statusOrder {
		if s.Has(e.bit) {
			parts = append(parts, e.name)
		}
	}
	return strings.Join(parts, "+")
}

// MarshalText implements encoding.TextMarshaler so Status serialises
// to JSON / XML / YAML as its String() form rather than as a uint8.
func (s Status) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

// UnmarshalText accepts the same `+`-joined form String() produces.
// Unknown tokens are an error so typos in serialised matrices don't
// silently round-trip.
func (s *Status) UnmarshalText(b []byte) error {
	var out Status
	for _, token := range strings.Split(string(b), "+") {
		switch token {
		case "stable":
			out |= Stable
		case "supported":
			out |= Supported
		case "experimental":
			out |= Experimental
		case "quirky":
			out |= Quirky
		case "deprecated":
			out |= Deprecated
		case "broken":
			out |= Broken
		default:
			return fmt.Errorf("product: unknown Status %q", token)
		}
	}
	*s = out
	return nil
}
