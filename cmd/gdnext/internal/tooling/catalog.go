package tooling

// Catalog is the single source of truth for the set of toolchains gdnext
// knows about. Order matters: it's the order verbs like
// `gdnext toolchain list` and `gdnext toolchain doctor` use to render
// their tables, and the order `gdnext toolchain install` (no args)
// walks. Keep base-required tools (godot, go, zig) first.
//
// To add a new toolchain: declare its var in tools.go with a unique
// non-empty Slug, then append a pointer to it here. No other file
// needs to change — the cli package iterates Catalog directly.
var Catalog = []*Tool{
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

// BySlug returns the catalog entry whose Slug matches slug, or nil
// when no entry matches.
func BySlug(slug string) *Tool {
	for _, t := range Catalog {
		if t.Slug == slug {
			return t
		}
	}
	return nil
}
