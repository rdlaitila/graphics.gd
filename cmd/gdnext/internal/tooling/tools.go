package tooling

import (
	"fmt"

	"graphics.gd/product"
)

// The named vars below are the runtime handles for each entry in
// product.ToolchainMatrix. They wrap the declarative product.Toolchain
// record with a mutable Path cache and the Lookup / Exec / Action /
// Output / CombinedOutput methods. Catalog (in catalog.go) holds the
// pointer set in matrix order.
//
// To add a toolchain: append the record to product.ToolchainMatrix
// (product/toolchains.go), then add a named var here for the slug and
// list it in Catalog. The wrap() helper panics at init if the slug is
// missing from the product matrix, so a typo surfaces immediately.

var (
	Godot                        = wrap("godot")
	Go                           = wrap("go")
	Zig                          = wrap("zig")
	LLVM                         = wrap("llvm")
	AndroidDebugBridge           = wrap("adb")
	AndroidPackageSigner         = wrap("apksigner")
	AndroidAssetPackagingTool    = wrap("aapt2")
	AndroidPackageKitTool        = wrap("apktool")
	BundleTool                   = wrap("bundletool")
	Android                      = wrap("android.jar")
	UltimatePackerForExecutables = wrap("upx")
	Velopack                     = wrap("vpk")
	LibGodot                     = wrap("libgodot")
	LibGodotEditor               = wrap("libgodot-editor")
	ListDynamicDependencies      = wrap("ldd")
)

func wrap(slug string) toolchain {
	t, ok := product.LookupToolchain(slug)
	if !ok {
		panic(fmt.Sprintf("tooling: product.ToolchainMatrix is missing slug %q", slug))
	}
	return toolchain{Toolchain: t}
}
