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
	JDK                          *Tool
	SCons                        *Tool
	GodotSrc                     *Tool
	GodotBuildroot               *Tool
	Zig                          *Tool
	LLVM                         *Tool
	AndroidBuildTools            *Tool
	AndroidPlatformTools         *Tool
	AndroidPlatform35            *Tool
	AndroidNDK                   *Tool
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
	// bySlug maps every product.Toolchain.Slug in the catalog to its
	// live *Tool. Populated by NewCatalog once and read-only from
	// then on. Both Tools() and BySlug walk this map so the row
	// order + lookup path stay in lockstep with product.ToolchainMatrix.
	bySlug map[string]*Tool
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
	c := Catalog{
		Host:                         env.Host,
		Godot:                        mk(product.ToolchainGodot),
		Go:                           mk(product.ToolchainGo),
		JDK:                          mk(product.ToolchainAndroidJDK),
		SCons:                        mk(product.ToolchainSCons),
		GodotSrc:                     mk(product.ToolchainGodotSrc),
		GodotBuildroot:               mk(product.ToolchainGodotBuildroot),
		Zig:                          mk(product.ToolchainZig),
		LLVM:                         mk(product.ToolchainLLVM),
		AndroidBuildTools:            mk(product.ToolchainAndroidBuildTools),
		AndroidPlatformTools:         mk(product.ToolchainAndroidPlatformTools),
		AndroidPlatform35:            mk(product.ToolchainAndroidPlatform35),
		AndroidNDK:                   mk(product.ToolchainAndroidNDK),
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
	}
	// admittedly a bit of hack / duplication here to sort toolchain command output, sorry.
	c.bySlug = map[string]*Tool{
		product.ToolchainGodot.Slug:                c.Godot,
		product.ToolchainGo.Slug:                   c.Go,
		product.ToolchainAndroidJDK.Slug:           c.JDK,
		product.ToolchainSCons.Slug:                c.SCons,
		product.ToolchainGodotSrc.Slug:             c.GodotSrc,
		product.ToolchainGodotBuildroot.Slug:       c.GodotBuildroot,
		product.ToolchainZig.Slug:                  c.Zig,
		product.ToolchainLLVM.Slug:                 c.LLVM,
		product.ToolchainAndroidBuildTools.Slug:    c.AndroidBuildTools,
		product.ToolchainAndroidPlatformTools.Slug: c.AndroidPlatformTools,
		product.ToolchainAndroidPlatform35.Slug:    c.AndroidPlatform35,
		product.ToolchainAndroidNDK.Slug:           c.AndroidNDK,
		product.ToolchainADB.Slug:                  c.AndroidDebugBridge,
		product.ToolchainApkSigner.Slug:            c.AndroidPackageSigner,
		product.ToolchainAAPT2.Slug:                c.AndroidAssetPackagingTool,
		product.ToolchainApkTool.Slug:              c.AndroidPackageKitTool,
		product.ToolchainBundleTool.Slug:           c.BundleTool,
		product.ToolchainAndroidJar.Slug:           c.Android,
		product.ToolchainUPX.Slug:                  c.UltimatePackerForExecutables,
		product.ToolchainVPK.Slug:                  c.Velopack,
		product.ToolchainLibGodot.Slug:             c.LibGodot,
		product.ToolchainLibGodotEditor.Slug:       c.LibGodotEditor,
		product.ToolchainLDD.Slug:                  c.ListDynamicDependencies,
	}
	return c, nil
}

// Tools returns every Tool in product.ToolchainMatrix order. That
// order is the canonical source of truth: catalog.Tools() drives the
// row ordering of `gdnext toolchain list` and `doctor`, so reordering
// entries in matrix.go automatically reorders both surfaces without
// a second edit here.
func (t Catalog) Tools() []*Tool {
	out := make([]*Tool, 0, len(product.ToolchainMatrix))
	for _, entry := range product.ToolchainMatrix {
		if tool, ok := t.bySlug[entry.Slug]; ok {
			out = append(out, tool)
		}
	}
	return out
}

// BySlug returns the catalog entry whose Slug matches slug, or nil
// when no entry matches.
func (t Catalog) BySlug(slug string) *Tool {
	return t.bySlug[slug]
}
