//go:build !js && !android

package main

import (
	"fmt"
	"os"

	"graphics.gd/classdb/Engine"
	"graphics.gd/classdb/FileAccess"
	"graphics.gd/classdb/SceneTree"
	"graphics.gd/product"
	"graphics.gd/variant/Object"
)

// playRequested reports whether the play-bot should attach. Native
// builds gate on $GDNEXT_PLAY; the WASM build (see play_env_js.go)
// reads the same flag from the URL query string.
func playRequested() bool { return os.Getenv(product.EnvPlay) != "" }

// playEnv returns the value the driver passed for name. Native builds
// just pass through os.Getenv; the WASM build (see play_env_js.go)
// hydrates the same names from URL query params.
func playEnv(name string) string { return os.Getenv(name) }

// writePlayReport persists the marshalled report via Godot's
// FileAccess. Go's io subsystem mis-behaves under libgodot's hosted
// runtime on some platforms (writes to the driver-named path either
// fail silently or hit half-mapped fd tables); routing through Godot
// avoids that path entirely.
func writePlayReport(data []byte) {
	path := os.Getenv(product.EnvPlayReport)
	if path == "" {
		return
	}
	storeBytes(path, data, "play report")
}

// writePlayScreenshotFromViewport snapshots the root viewport and
// writes the PNG to $GDNEXT_PLAY_SCREENSHOT. WASM builds are a no-op
// (see play_env_js.go) because the Playwright driver takes the
// screenshot host-side via page.screenshot().
func writePlayScreenshotFromViewport() {
	path := os.Getenv(product.EnvPlayScreenshot)
	if path == "" {
		return
	}
	tree, ok := Object.As[SceneTree.Instance](Engine.GetMainLoop())
	if !ok {
		panic("play screenshot requested but engine main loop is not a SceneTree")
	}
	png := tree.Root().AsViewport().GetTexture().AsTexture2D().GetImage().SavePngToBuffer()
	storeBytes(path, png, "play screenshot")
}

// storeBytes writes data to path via FileAccess. Path may be a Godot
// resource URI (user://, res://) or a host filesystem path. The
// FileAccess instance is RefCounted and closes when the local
// reference falls out of scope; Flush forces the write to land
// before that.
func storeBytes(path string, data []byte, what string) {
	f := FileAccess.Open(path, FileAccess.Write)
	if !f.StoreBuffer(data) {
		panic(fmt.Errorf("write %s to %s: FileAccess.StoreBuffer returned false (open error: %v)", what, path, FileAccess.GetOpenError()))
	}
	f.Flush()
}
