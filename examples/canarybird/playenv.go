//go:build !js && !android

package main

import (
	"fmt"
	"os"

	"graphics.gd/classdb/Engine"
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

// writePlayReport persists the marshalled report. Native builds write
// to the path the driver named in $GDNEXT_PLAY_REPORT; the WASM build
// (see play_env_js.go) prints a `GDNEXT_PLAY_REPORT:<base64>` line on
// the JS console where Playwright captures it.
func writePlayReport(data []byte) {
	path := os.Getenv(product.EnvPlayReport)
	if path == "" {
		return
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		panic(fmt.Errorf("write play report %s: %w", path, err))
	}
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
	if err := os.WriteFile(path, png, 0644); err != nil {
		panic(fmt.Errorf("write play screenshot %s: %w", path, err))
	}
}
