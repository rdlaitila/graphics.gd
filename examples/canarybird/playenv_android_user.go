//go:build android && !playbot

package main

// Android default build: the play-bot is NOT baked in, so the APK
// installed via `gdnext android install` runs the game normally.
// CI builds explicitly opt into the bot via `-tags playbot` (see
// the android branch in cmd/gdnext/internal/ci/build_target.go).
func playRequested() bool { return false }

// Stub the rest of the playenv API so canarybird compiles; the
// playbot never wires when playRequested is false, so these are
// dead code at runtime.

func playEnv(string) string { return "" }

func writePlayReport([]byte) {}

func writePlayScreenshotFromViewport() {}
