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
		got, ok := LookupToolchain(e.Slug)
		if !ok {
			t.Errorf("LookupToolchain(%q) missing", e.Slug)
			continue
		}
		if got.Name != e.Name {
			t.Errorf("LookupToolchain(%q) = name %q, want %q", e.Slug, got.Name, e.Name)
		}
	}
}

// TestToolchainRequiredCoverage spot-checks that the build-everywhere
// tools (godot, go, zig) are required for every target in the platform
// matrix. A regression in AllPlatforms / Matches would surface here.
func TestToolchainRequiredCoverage(t *testing.T) {
	always := []string{"godot", "go", "zig"}
	for _, slug := range always {
		tc, ok := LookupToolchain(slug)
		if !ok {
			t.Fatalf("expected toolchain %q in matrix", slug)
		}
		for _, p := range PlatformMatrix {
			if !tc.IsRequiredFor(p.GOOS, p.GOARCH) {
				t.Errorf("%s not required for %s — AllPlatforms broken?", slug, p.Tuple())
			}
		}
	}
}

// TestAndroidToolsTargetAndroidAndQuest confirms the android toolchain
// fan-out covers both android and metaquest builds (the latter is an
// android variant with an OpenXR loader).
func TestAndroidToolsTargetAndroidAndQuest(t *testing.T) {
	for _, slug := range []string{"apksigner", "aapt2", "apktool", "bundletool", "android.jar"} {
		tc, ok := LookupToolchain(slug)
		if !ok {
			t.Fatalf("missing toolchain %q", slug)
		}
		for _, goos := range []string{"android", "metaquest"} {
			if !tc.IsRequiredFor(goos, "arm64") {
				t.Errorf("%s not required for %s/arm64", slug, goos)
			}
		}
	}
}

// TestLddIsLinuxOnly confirms the host-only constraint on ldd survives
// matrix edits.
func TestLddIsLinuxOnly(t *testing.T) {
	tc, ok := LookupToolchain("ldd")
	if !ok {
		t.Fatal("missing toolchain ldd")
	}
	if !tc.IsAvailableOn("linux", "amd64") {
		t.Error("ldd should be available on linux")
	}
	if tc.IsAvailableOn("darwin", "arm64") {
		t.Error("ldd should not be available on darwin")
	}
	if tc.IsAvailableOn("windows", "amd64") {
		t.Error("ldd should not be available on windows")
	}
}
