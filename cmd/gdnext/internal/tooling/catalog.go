package tooling

import "graphics.gd/product"

// Catalog is the single source of truth for the set of toolchains gdnext
// knows about. Order matters: it's the order verbs like
// `gdnext toolchain list` and `gdnext toolchain doctor` use to render
// their tables, and the order `gdnext toolchain install` (no args)
// walks. Keep base-required tools (godot, go, zig) first.
//
// To add a new toolchain: declare its var in tools.go with a unique
// non-empty Slug, then append a pointer to it here. No other file
// needs to change — the cli package iterates Catalog reflectively.
var Catalog = []*toolchain{
	&Godot,
	&Go,
	&Zig,
	&LLVM,
	&AndroidDebugBridge,
	&AndroidPackageSigner,
	&AndroidAssetPackagingTool,
	&AndroidPackageKitTool,
	&BundleTool,
	&Android,
	&UltimatePackerForExecutables,
	&Velopack,
	&LibGodot,
	&LibGodotEditor,
	&ListDynamicDependencies,
}

// Entry is the read-only view of a catalog entry the cli package
// consumes. Mirrors the toolchain fields that downstream verbs actually
// care about; pulling them out lets the cli code stay decoupled from
// the (unexported, mutable, Path-caching) toolchain struct itself.
type Entry struct {
	Slug      string
	Name      string
	Version   string
	Purpose   string            // == RequiredFor (human-readable description)
	Required  product.Platforms // when this tool is needed for a build (target side)
	Available product.Platforms // where this tool can run (host side); zero = any host
	IsLibrary bool

	// Lookup is the bound method on the underlying *toolchain so callers
	// don't have to thread the receiver back through.
	Lookup func(...Mode) (string, error)
}

// IsRequiredFor reports whether this entry is needed when building for
// (targetGOOS, targetGOARCH).
func (e Entry) IsRequiredFor(targetGOOS, targetGOARCH string) bool {
	return e.Required.Matches(targetGOOS, targetGOARCH)
}

// IsAvailableOn reports whether this entry can be obtained on host
// (hostGOOS, hostGOARCH). An unset Available is treated as "any host".
func (e Entry) IsAvailableOn(hostGOOS, hostGOARCH string) bool {
	if len(e.Available.GOOS) == 0 {
		return true
	}
	return e.Available.Matches(hostGOOS, hostGOARCH)
}

// Entries returns a snapshot of every catalog entry. Iterating the
// snapshot rather than Catalog directly keeps callers from poking at
// the underlying toolchain values (which carry the mutable Path cache).
func Entries() []Entry {
	out := make([]Entry, 0, len(Catalog))
	for _, t := range Catalog {
		out = append(out, Entry{
			Slug:      t.Slug,
			Name:      t.Name,
			Version:   t.Version,
			Purpose:   t.RequiredFor,
			Required:  t.Required,
			Available: t.Available,
			IsLibrary: t.IsLibrary,
			Lookup:    t.Lookup,
		})
	}
	return out
}

// LookupBySlug returns the entry whose Slug matches name, or zero +
// false if no entry matches. Used by `gdnext toolchain install <name>`
// and `gdnext toolchain path <name>`.
func LookupBySlug(name string) (Entry, bool) {
	for _, e := range Entries() {
		if e.Slug == name {
			return e, true
		}
	}
	return Entry{}, false
}
