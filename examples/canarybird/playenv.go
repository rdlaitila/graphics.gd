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
		debugf("==> canarybird playenv: no play report requested, skipping write (%d bytes)", len(data))
		return
	}
	debugf("==> canarybird playenv: writing play report: %s (%d bytes)", path, len(data))
	if err := storeBytes(path, data); err != nil {
		panic(fmt.Errorf("write play report to %s: %w", path, err))
	}
}

func writePlayScreenshotFromViewport() {
	path := os.Getenv(product.EnvPlayScreenshot)
	if path == "" {
		debugf("==> canarybird playenv: no play screenshot requested, skipping write")
		return
	}
	tree, ok := Object.As[SceneTree.Instance](Engine.GetMainLoop())
	if !ok {
		panic("play screenshot requested but engine main loop is not a SceneTree")
	}
	png := tree.Root().AsViewport().GetTexture().AsTexture2D().GetImage().SavePngToBuffer()
	debugf("==> canarybird playenv: writing play screenshot: %s (%d bytes)", path, len(png))
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
		debugf("==> canarybird playenv: FileAccess.Open returned error for %s: %v", path, openErr)
		return openErr
	}
	var err error
	if !f.StoreBuffer(data) {
		debugf("==> canarybird playenv: FileAccess.StoreBuffer returned false for %s (%d bytes)", path, len(data))
		err = fmt.Errorf("FileAccess.StoreBuffer returned false")
	}
	defer f.Flush()
	defer f.Close()
	if err == nil {
		debugf("==> canarybird playenv: FileAccess.StoreBuffer succeeded for %s (%d bytes)", path, len(data))
	}
	if err != nil {
		// try go io/os fallback if FileAccess failed, so the report is still written in libgodot mode.
		debugf("==> canarybird playenv: falling back to os.WriteFile for %s (%d bytes)", path, len(data))
		if writeErr := os.WriteFile(path, data, 0644); writeErr != nil {
			debugf("==> canarybird playenv: os.WriteFile returned error for %s: %v", path, writeErr)
			return writeErr
		}
		debugf("==> canarybird playenv: os.WriteFile succeeded for %s (%d bytes)", path, len(data))
	}
	return nil
}
