//go:build js && wasm

package main

import (
	"encoding/base64"
	"fmt"
	"syscall/js"

	"graphics.gd/product"
)

// playRequested mirrors play_env.go's native check but reads from the
// URL query string Playwright populates when navigating to the
// served index.html. We deliberately avoid JavaScriptBridge here:
// the stock Godot 4.7 web export template ships without GDExtension
// support, so every bridge call resolves to a null function and
// kills the page. syscall/js is part of Go's runtime and routes
// through wasm_exec.js, which works regardless of engine flags.
func playRequested() bool { return jsQueryParam("gdnext_play") != "" }

// playEnv hydrates the env names the playbot otherwise reads from
// os.Getenv. Values arrive via URL query params; GDNEXT_PLAY_HUD is
// base64-encoded (RawURL) to avoid percent-escape ambiguity.
func playEnv(name string) string {
	switch name {
	case product.EnvPlay:
		return jsQueryParam("gdnext_play")
	case product.EnvPlayHUD:
		raw := jsQueryParam("gdnext_play_hud")
		if raw == "" {
			return ""
		}
		decoded, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil {
			panic(fmt.Errorf("decode gdnext_play_hud: %w", err))
		}
		return string(decoded)
	}
	return ""
}

// writePlayReport pushes the report to the browser console where the
// Playwright driver picks it up. Stdout is routed to console.log by
// wasm_exec.js (one line per newline-terminated chunk), so a single
// fmt.Println of the tagged base64 is enough — no JavaScriptBridge
// dependency.
func writePlayReport(data []byte) {
	fmt.Println(product.EnvPlayReport + ":" + base64.StdEncoding.EncodeToString(data))
}

// writePlayScreenshotFromViewport is a no-op in WASM; the Playwright
// driver captures the screenshot host-side after the report arrives.
func writePlayScreenshotFromViewport() {}

// jsQueryParam reads window.location.search via syscall/js without
// touching JavaScriptBridge.
func jsQueryParam(name string) string {
	params := js.Global().Get("URLSearchParams").New(js.Global().Get("location").Get("search"))
	v := params.Call("get", name)
	if v.IsNull() || v.IsUndefined() {
		return ""
	}
	return v.String()
}
