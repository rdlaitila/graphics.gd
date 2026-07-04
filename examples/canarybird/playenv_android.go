//go:build android

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"graphics.gd/classdb/Engine"
	"graphics.gd/classdb/FileAccess"
	"graphics.gd/classdb/SceneTree"
	"graphics.gd/product"
	"graphics.gd/variant/Object"
)

// externalFilesDir is the app's external-files directory
// (/sdcard/Android/data/<pkg>/files/). It's the only writable path
// the app uid shares with `adb push/pull`: /data/local/tmp is shell-
// owned (drwxrwx--x) so the app can't open files inside, and the
// app's private /data/data/<pkg>/files is uid-fenced so `adb pull`
// can't read it without run-as (release APKs refuse run-as). The
// driver pushes the envelope here pre-launch and pulls the report
// and screenshot here post-launch.
var externalFilesDir = func() string {
	cmdline, err := os.ReadFile("/proc/self/cmdline")
	if err != nil {
		return ""
	}
	pkg := string(bytes.TrimRight(bytes.SplitN(cmdline, []byte{0}, 2)[0], "\x00"))
	if pkg == "" {
		return ""
	}
	return "/sdcard/Android/data/" + pkg + "/files"
}()

var driverFile = filepath.Join(externalFilesDir, "gd-play.json")

var driverEnv = func() map[string]string {
	if externalFilesDir == "" {
		return nil
	}
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

// writePlayReport writes the JSON payload to the app's external-files
// dir. The driver `adb pull`s the same path after the activity exits.
// logcat is the wrong transport here: per-message payloads are capped
// well below a full HUD-bearing report (~1 KB after base64), so even
// `Engine.Print` lines get truncated mid-string.
func writePlayReport(data []byte) {
	if externalFilesDir == "" {
		return
	}
	storeBytes(filepath.Join(externalFilesDir, "gd-play-report.json"), data, "play report")
}

// writePlayScreenshotFromViewport captures the engine viewport and
// writes the PNG next to the report. The driver pulls both via adb.
func writePlayScreenshotFromViewport() {
	if externalFilesDir == "" {
		return
	}
	tree, ok := Object.As[SceneTree.Instance](Engine.GetMainLoop())
	if !ok {
		panic("play screenshot requested but engine main loop is not a SceneTree")
	}
	png := tree.Root().AsViewport().GetTexture().AsTexture2D().GetImage().SavePngToBuffer()
	storeBytes(filepath.Join(externalFilesDir, "gd-play-screenshot.png"), png, "play screenshot")
}

// storeBytes writes data to path via FileAccess. Go's io subsystem
// mis-behaves under libgodot's hosted runtime on some platforms;
// routing through Godot avoids that path entirely. The FileAccess
// instance is RefCounted and closes when the local reference falls
// out of scope; Flush forces the write to land before that.
func storeBytes(path string, data []byte, what string) {
	f := FileAccess.Open(path, FileAccess.Write)
	if !f.StoreBuffer(data) {
		panic(fmt.Errorf("write %s to %s: FileAccess.StoreBuffer returned false (open error: %v)", what, path, FileAccess.GetOpenError()))
	}
	f.Flush()
}
