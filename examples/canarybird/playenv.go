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

func playRequested() bool { return os.Getenv(product.EnvPlay) != "" }

func playEnv(name string) string { return os.Getenv(name) }

func writePlayReport(data []byte) {
	path := os.Getenv(product.EnvPlayResult)
	if path == "" {
		return
	}
	Engine.Print(fmt.Sprintf("writing play report: %s (%d bytes)", path, len(data)))
	if err := storeBytes(path, data); err != nil {
		panic(fmt.Errorf("write play report to %s: %w", path, err))
	}
}

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
	Engine.Print(fmt.Sprintf("writing play screenshot: %s (%d bytes)", path, len(png)))
	if err := storeBytes(path, png); err != nil {
		panic(fmt.Errorf("write play screenshot to %s: %w", path, err))
	}
}

// storeBytes routes through Godot's FileAccess so writes land on the
// host filesystem even when the binary is loaded via libgodot (where
// os.WriteFile can target a non-host fs layer).
func storeBytes(path string, data []byte) error {
	f := FileAccess.Open(path, FileAccess.Write)
	if openErr := FileAccess.GetOpenError(); openErr != nil {
		return openErr
	}
	if !f.StoreBuffer(data) {
		return fmt.Errorf("FileAccess.StoreBuffer returned false")
	}
	f.Flush()
	return f.Close()
}
