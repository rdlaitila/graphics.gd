//go:build android

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"graphics.gd/classdb/Engine"
	"graphics.gd/classdb/SceneTree"
	"graphics.gd/product"
	"graphics.gd/variant/Object"
)

// driverFile is the JSON envelope the play-cell driver pushes via
// `adb push` to /data/local/tmp/ before launching the activity. The
// envelope replaces the unix-style env-var contract that other
// playenv variants get from $GDNEXT_PLAY_* because Android's
// activity boot strips the host environment.
const driverFile = "/data/local/tmp/gdnext-play.json"

var driverEnv = func() map[string]string {
	data, err := os.ReadFile(driverFile)
	if err != nil {
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	return m
}()

func playRequested() bool { return driverEnv[product.EnvPlay] != "" }

func playEnv(name string) string { return driverEnv[name] }

func writePlayReport(data []byte) {
	path := driverEnv[product.EnvPlayReport]
	if path == "" {
		return
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		panic(fmt.Errorf("write play report %s: %w", path, err))
	}
}

func writePlayScreenshotFromViewport() {
	path := driverEnv[product.EnvPlayScreenshot]
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
