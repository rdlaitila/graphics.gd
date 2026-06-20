package tooling

import "graphics.gd/product"

// Catalog is the single source of truth for the set of toolchains gdnext
// knows about. Order matters: it's the order verbs like
// `gdnext toolchain list` and `gdnext toolchain doctor` use to render
// their tables, and the order `gdnext toolchain install` (no args)
// walks. Keep base-required tools (godot, go, zig) first.
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

// The named vars below are the runtime handles for each entry in
// product.ToolchainMatrix. They wrap the declarative product.Toolchain
// record with a mutable Path cache and the Lookup / Exec / Action /
// Output / CombinedOutput methods. Catalog (in catalog.go) holds the
// pointer set in matrix order.
var (
	Godot                        = Tool{Toolchain: product.ToolchainGodot}
	Go                           = Tool{Toolchain: product.ToolchainGo}
	Zig                          = Tool{Toolchain: product.ToolchainZig}
	LLVM                         = Tool{Toolchain: product.ToolchainLLVM}
	AndroidDebugBridge           = Tool{Toolchain: product.ToolchainADB}
	AndroidPackageSigner         = Tool{Toolchain: product.ToolchainApkSigner}
	AndroidAssetPackagingTool    = Tool{Toolchain: product.ToolchainAAPT2}
	AndroidPackageKitTool        = Tool{Toolchain: product.ToolchainApkTool}
	BundleTool                   = Tool{Toolchain: product.ToolchainBundleTool}
	Android                      = Tool{Toolchain: product.ToolchainAndroidJar}
	UltimatePackerForExecutables = Tool{Toolchain: product.ToolchainUPX}
	Velopack                     = Tool{Toolchain: product.ToolchainVPK}
	LibGodot                     = Tool{Toolchain: product.ToolchainLibGodot}
	LibGodotEditor               = Tool{Toolchain: product.ToolchainLibGodotEditor}
	ListDynamicDependencies      = Tool{Toolchain: product.ToolchainLDD}
)

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
