package product

import "testing"

// TestToolchainSlugsPresent enforces a non-empty Slug on every row.
// Slug is the only identifier `gdnext toolchain {list,path,install}` use,
// so an empty one is silently unreachable.
func TestToolchainSlugsPresent(t *testing.T) {
	for i, e := range ToolchainMatrix {
		if e.Slug == "" {
			t.Errorf("ToolchainMatrix[%d] has empty Slug", i)
		}
	}
}

// TestToolchainSlugsUnique catches accidental copy-paste duplicates.
// LookupToolchain returns the first match, so a duplicate slug would
// silently shadow.
func TestToolchainSlugsUnique(t *testing.T) {
	seen := map[string]Toolchain{}
	for _, e := range ToolchainMatrix {
		if other, dup := seen[e.Slug]; dup {
			t.Errorf("toolchain slug %q collides: %q vs %q",
				e.Slug, other.Name, e.Name)
		}
		seen[e.Slug] = e
	}
}

// TestToolchainLookup confirms every matrix slug round-trips through
// LookupToolchain.
func TestToolchainLookup(t *testing.T) {
	for _, e := range ToolchainMatrix {
		got, ok := FindToolchainBySlug(e.Slug)
		if !ok {
			t.Errorf("LookupToolchain(%q) missing", e.Slug)
			continue
		}
		if got.Name != e.Name {
			t.Errorf("LookupToolchain(%q) = name %q, want %q", e.Slug, got.Name, e.Name)
		}
	}
}
