//go:build android && playbot

package main

import (
	"encoding/base64"
	"fmt"
)

// On android the APK is dedicated to a single play run: build with
// `-tags playbot` to bake the bot in. The non-playbot variant in
// playenv_android_user.go returns false from playRequested so the
// normal `gdnext build` APK installs and runs the game directly.
func playRequested() bool { return true }

// playEnv hydrates the same names the rest of the playbot reads on
// other platforms. EnvPlay returns "1" so the play codepath fires;
// EnvPlayHUD comes from the driver as a build-time --es extra passed
// through the launcher intent. We don't currently surface those
// extras to the playbot (would require a Java glue), so the HUD
// stays empty on android cells for now — the run-time Godot Version
// row that the playbot prepends still appears.
func playEnv(name string) string {
	if name == "GDNEXT_PLAY" {
		return "1"
	}
	return ""
}

// writePlayReport pushes the report to logcat where the android-emu
// driver picks it up via `adb logcat -s Go:I`. Go's android runtime
// routes os.Stdout to logcat under the "Go" tag, so fmt.Println of
// the tagged base64 reaches the host without needing app-private
// file access (release APKs aren't debuggable and run-as is refused
// on them; logcat is read-anywhere).
func writePlayReport(data []byte) {
	fmt.Println("GDNEXT_PLAY_REPORT:" + base64.StdEncoding.EncodeToString(data))
}

// writePlayScreenshotFromViewport is a no-op on android: the driver
// captures a host-side screenshot via `adb shell screencap` just
// before tearing down the app, mirroring how the browser path
// captures via page.screenshot(). Skipping the in-engine snapshot
// keeps the APK free of any storage-permission dependencies.
func writePlayScreenshotFromViewport() {}
