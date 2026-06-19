package product

import "testing"

// TestTitlesPresent enforces that every matrix row has a non-empty
// Title, which downstream user-facing comms (docs, release notes, the
// Godot export preset display name) rely on. DisplayTitle() does fall
// back gracefully, but missing a Title is almost certainly a forgotten
// field rather than an intentional choice.
func TestTitlesPresent(t *testing.T) {
	for _, p := range PlatformMatrix {
		if p.Title == "" {
			t.Errorf("%s/%s has empty Title (DisplayTitle would fall back to %q)",
				p.GOOS, p.GOARCH, p.DisplayTitle())
		}
	}
}

// TestTitlesUnique catches accidental copy-paste duplicates in matrix.go.
func TestTitlesUnique(t *testing.T) {
	seen := map[string]Platform{}
	for _, p := range PlatformMatrix {
		if p.Title == "" {
			continue
		}
		if other, dup := seen[p.Title]; dup {
			t.Errorf("title %q collides: %s/%s vs %s/%s",
				p.Title, other.GOOS, other.GOARCH, p.GOOS, p.GOARCH)
		}
		seen[p.Title] = p
	}
}

// TestTuple confirms Tuple() / Tuple(goos, goarch) produce the docker-
// style "goos/goarch" identifier downstream code expects.
func TestTuple(t *testing.T) {
	cases := []struct {
		goos, goarch, want string
	}{
		{"linux", "amd64", "linux/amd64"},
		{"darwin", "arm64", "darwin/arm64"},
		{"js", "wasm", "js/wasm"},
		{"linux", "", "linux"},
	}
	for _, c := range cases {
		if got := Tuple(c.goos, c.goarch); got != c.want {
			t.Errorf("Tuple(%q, %q) = %q, want %q", c.goos, c.goarch, got, c.want)
		}
		p := Platform{GOOS: c.goos, GOARCH: c.goarch}
		if got := p.Tuple(); got != c.want {
			t.Errorf("Platform{%q, %q}.Tuple() = %q, want %q", c.goos, c.goarch, got, c.want)
		}
	}
}

// TestAliasesUnique enforces the contract Resolve() relies on for
// aliases: each *alias* maps to at most one Platform row. Canonical
// GOOS strings deliberately appear multiple times (once per GOARCH)
// and are excluded from this check; the GOOS+GOARCH uniqueness is
// covered by TestRowsUnique.
func TestAliasesUnique(t *testing.T) {
	seen := map[string]Platform{}
	for _, p := range PlatformMatrix {
		for _, alias := range p.Aliases {
			if other, dup := seen[alias]; dup {
				t.Errorf("alias %q collides: %s/%s vs %s/%s",
					alias, other.GOOS, other.GOARCH, p.GOOS, p.GOARCH)
			}
			seen[alias] = p
		}
	}
}

// TestRowsUnique ensures no two matrix rows share a (GOOS, GOARCH)
// pair. Lookup(goos, goarch) returns the first match by design, so a
// duplicate row would silently shadow the second.
func TestRowsUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range PlatformMatrix {
		key := p.GOOS + "/" + p.GOARCH
		if seen[key] {
			t.Errorf("duplicate row %s", key)
		}
		seen[key] = true
	}
}

// TestAliasesDontShadowCanonical guards against an alias of one row
// colliding with another row's canonical GOOS (e.g. someone adding
// "linux" as an alias on a different platform).
func TestAliasesDontShadowCanonical(t *testing.T) {
	canonical := map[string]bool{}
	for _, p := range PlatformMatrix {
		canonical[p.GOOS] = true
	}
	for _, p := range PlatformMatrix {
		for _, alias := range p.Aliases {
			if canonical[alias] && alias != p.GOOS {
				t.Errorf("alias %q on %s/%s shadows canonical GOOS",
					alias, p.GOOS, p.GOARCH)
			}
		}
	}
}

// TestKindCoverage rejects rows with no role assigned — every Matrix
// entry must be reachable as either a host or a target (or both).
func TestKindCoverage(t *testing.T) {
	for _, p := range PlatformMatrix {
		if p.Kind == 0 {
			t.Errorf("%s/%s has Kind == 0; must set Host, Target, or both",
				p.GOOS, p.GOARCH)
		}
	}
}

// TestResolveCanonical confirms the canonical GOOS string resolves to
// at least one matrix row.
func TestResolveCanonical(t *testing.T) {
	for _, p := range PlatformMatrix {
		got, ok := Resolve(p.GOOS)
		if !ok {
			t.Errorf("Resolve(%q) missing", p.GOOS)
		}
		// Note: Resolve returns the *first* match for a GOOS, so we
		// only check that some row came back, not equality.
		_ = got
	}
}

// TestResolveAliases confirms every alias resolves to a row with the
// alias actually in its Names list (catches typos in matrix.go).
func TestResolveAliases(t *testing.T) {
	for _, p := range PlatformMatrix {
		for _, alias := range p.Aliases {
			got, ok := Resolve(alias)
			if !ok {
				t.Errorf("Resolve(%q) missing for %s/%s", alias, p.GOOS, p.GOARCH)
				continue
			}
			if got.GOOS != p.GOOS {
				t.Errorf("Resolve(%q) returned %s, expected %s", alias, got.GOOS, p.GOOS)
			}
		}
	}
}

// TestHostsAndTargetsNonEmpty makes sure the filter helpers find rows.
// A regression where Hosts() or Targets() returns nothing would silently
// break `gdnext platforms hosts` / `targets` output.
func TestHostsAndTargetsNonEmpty(t *testing.T) {
	if len(Hosts()) == 0 {
		t.Error("Hosts() is empty; at least one Host row must exist")
	}
	if len(Targets()) == 0 {
		t.Error("Targets() is empty; at least one Target row must exist")
	}
}
