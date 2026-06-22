package tooling

import (
	"github.com/samber/do/v2"
	"graphics.gd/product"
)

// Catalog is the per-(host) runtime view of every product.Toolchain
// gdnext knows about. The CLI builds one via NewCatalog in the root
// Before hook, threads it into the Builder methods, and reads from
// it directly in subcommands. Each field wraps the matching
// product.Toolchain in a Tool whose Host points at the BuildHost the
// CLI resolved, so LookupPlatform can find GD*Path / UserHomeRoot
// without re-deriving them on every call.
//
// Iterate via Tools() to walk every entry in matrix order, or
// resolve a single one with BySlug.
type Catalog struct {
	Host                         product.BuildHost
	Godot                        *Tool
	Go                           *Tool
	Zig                          *Tool
	LLVM                         *Tool
	AndroidDebugBridge           *Tool
	AndroidPackageSigner         *Tool
	AndroidAssetPackagingTool    *Tool
	AndroidPackageKitTool        *Tool
	BundleTool                   *Tool
	Android                      *Tool
	UltimatePackerForExecutables *Tool
	Velopack                     *Tool
	LibGodot                     *Tool
	LibGodotEditor               *Tool
	ListDynamicDependencies      *Tool
}

// NewCatalog constructs the canonical tooling.Catalog for the current
// host, using the BuildEnv resolved via dependency injection.
func NewCatalog(di do.Injector) (Catalog, error) {
	env := do.MustInvoke[product.BuildEnv](di)
	mk := func(t product.Toolchain) *Tool {
		return &Tool{
			Toolchain: t,
			Host:      env.Host,
		}
	}
	return Catalog{
		Host:                         env.Host,
		Godot:                        mk(product.ToolchainGodot),
		Go:                           mk(product.ToolchainGo),
		Zig:                          mk(product.ToolchainZig),
		LLVM:                         mk(product.ToolchainLLVM),
		AndroidDebugBridge:           mk(product.ToolchainADB),
		AndroidPackageSigner:         mk(product.ToolchainApkSigner),
		AndroidAssetPackagingTool:    mk(product.ToolchainAAPT2),
		AndroidPackageKitTool:        mk(product.ToolchainApkTool),
		BundleTool:                   mk(product.ToolchainBundleTool),
		Android:                      mk(product.ToolchainAndroidJar),
		UltimatePackerForExecutables: mk(product.ToolchainUPX),
		Velopack:                     mk(product.ToolchainVPK),
		LibGodot:                     mk(product.ToolchainLibGodot),
		LibGodotEditor:               mk(product.ToolchainLibGodotEditor),
		ListDynamicDependencies:      mk(product.ToolchainLDD),
	}, nil
}

// Tools returns every Tool in matrix order.
func (t Catalog) Tools() []*Tool {
	return []*Tool{
		t.Godot,
		t.Go,
		t.Zig,
		t.LLVM,
		t.AndroidDebugBridge,
		t.AndroidPackageSigner,
		t.AndroidAssetPackagingTool,
		t.AndroidPackageKitTool,
		t.BundleTool,
		t.Android,
		t.UltimatePackerForExecutables,
		t.Velopack,
		t.LibGodot,
		t.LibGodotEditor,
		t.ListDynamicDependencies,
	}
}

// BySlug returns the catalog entry whose Slug matches slug, or nil
// when no entry matches.
func (t Catalog) BySlug(slug string) *Tool {
	for _, tool := range t.Tools() {
		if tool.Slug == slug {
			return tool
		}
	}
	return nil
}
