//go:build !js && !android

package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"graphics.gd/classdb/Engine"
	"graphics.gd/classdb/FileAccess"
	"graphics.gd/classdb/SceneTree"
	"graphics.gd/product"
	"graphics.gd/variant/Object"
)

// startupEnv snapshots os.Environ at package-init time so we can
// fall back to it when something between exec and finish() (libgodot
// init, Godot's OS layer) unsetenv's our entries from the live env.
var startupEnv = os.Environ()

func init() {
	for _, kv := range startupEnv {
		if strings.HasPrefix(kv, "GDNEXT_") {
			fmt.Println("GDNEXT_DBG_INIT " + kv)
		}
	}
}

// playRequested reports whether the play-bot should attach.
func playRequested() bool { return playEnv(product.EnvPlay) != "" }

// playEnv reads name from the live env, falling back to the
// startup-time snapshot. Strips one matching pair of surrounding
// double-quote bytes from the value (driver wraps values that way
// as a workaround for the env-loss the libgodot path exhibits).
func playEnv(name string) string {
	if v := os.Getenv(name); v != "" {
		return stripQuotes(v)
	}
	prefix := name + "="
	for _, kv := range startupEnv {
		if strings.HasPrefix(kv, prefix) {
			return stripQuotes(kv[len(prefix):])
		}
	}
	return ""
}

func stripQuotes(v string) string {
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		return v[1 : len(v)-1]
	}
	return v
}

// dumpGDNextEnv emits every GDNEXT_-prefixed entry in the live env
// AND in the startup snapshot so the CI log shows which one (or
// neither) is missing the report path.
func dumpGDNextEnv(where string) {
	for i, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GDNEXT_") {
			continue
		}
		eq := strings.IndexByte(kv, '=')
		name := kv
		val := ""
		if eq >= 0 {
			name = kv[:eq]
			val = kv[eq+1:]
		}
		line := fmt.Sprintf("GDNEXT_DBG_ENV %s live [%d] %s=%q (raw_len=%d)", where, i, name, val, len(kv))
		Engine.Print(line)
		fmt.Println(line)
	}
	for i, kv := range startupEnv {
		if !strings.HasPrefix(kv, "GDNEXT_") {
			continue
		}
		eq := strings.IndexByte(kv, '=')
		name := kv
		val := ""
		if eq >= 0 {
			name = kv[:eq]
			val = kv[eq+1:]
		}
		line := fmt.Sprintf("GDNEXT_DBG_ENV %s snap [%d] %s=%q (raw_len=%d)", where, i, name, val, len(kv))
		Engine.Print(line)
		fmt.Println(line)
	}
}

// writePlayReport writes the marshalled report to the canonical path
// and to every probe-suffixed path, logging each writer's outcome so
// CI can pick a permanent transport per platform.
func writePlayReport(data []byte) {
	dumpGDNextEnv("writePlayReport")
	path := playEnv(product.EnvPlayResult)
	dbg := fmt.Sprintf("GDNEXT_DBG writePlayReport entry env[%s]=%q bytes=%d", product.EnvPlayResult, path, len(data))
	Engine.Print(dbg)
	fmt.Println(dbg)
	if path == "" {
		return
	}
	probeWrite(path, data, "report", true)
}

// writePlayScreenshotFromViewport snapshots the root viewport and
// writes the PNG to $GDNEXT_PLAY_SCREENSHOT.
func writePlayScreenshotFromViewport() {
	path := playEnv(product.EnvPlayScreenshot)
	dbg := fmt.Sprintf("GDNEXT_DBG writePlayScreenshot entry env[%s]=%q", product.EnvPlayScreenshot, path)
	Engine.Print(dbg)
	fmt.Println(dbg)
	if path == "" {
		return
	}
	tree, ok := Object.As[SceneTree.Instance](Engine.GetMainLoop())
	if !ok {
		panic("play screenshot requested but engine main loop is not a SceneTree")
	}
	png := tree.Root().AsViewport().GetTexture().AsTexture2D().GetImage().SavePngToBuffer()
	probeWrite(path, png, "screenshot", false)
}

// probeWrite is a diagnostic: every writer variant runs to
// <canonical>.probe-<name>, each result lands on stdout AND in the
// engine log, then the canonical path is written with the cheapest
// option that worked. emitBase64 also dumps the payload to stdout
// (report only) as a last-resort transport for the driver.
func probeWrite(canonical string, data []byte, what string, emitBase64 bool) {
	type probe struct {
		name string
		fn   func(string, []byte) error
	}
	probes := []probe{
		{"go-writefile", writeGoFile},
		{"go-osync", writeGoOSync},
		{"fa-buffer", writeFABuffer},
		{"fa-string", writeFAString},
	}
	for _, p := range probes {
		out := canonical + ".probe-" + p.name
		line := probeReport(what, p.name, out, p.fn(out, data))
		Engine.Print(line)
		fmt.Println(line)
	}
	if emitBase64 {
		fmt.Println("GDNEXT_PLAY_RESULT_B64:" + base64.StdEncoding.EncodeToString(data))
	}
	if err := writeGoFile(canonical, data); err != nil {
		if err2 := writeFABuffer(canonical, data); err2 != nil {
			fmt.Fprintf(os.Stderr, "canonical %s write failed via both go (%v) and FileAccess (%v)\n", what, err, err2)
		}
	}
}

func probeReport(what, name, path string, werr error) string {
	if werr != nil {
		return fmt.Sprintf("GDNEXT_PROBE %s %s: ERR %v", what, name, werr)
	}
	st, sterr := os.Stat(path)
	if sterr != nil {
		return fmt.Sprintf("GDNEXT_PROBE %s %s: wrote_ok stat_err=%v", what, name, sterr)
	}
	return fmt.Sprintf("GDNEXT_PROBE %s %s: ok %dB at %s", what, name, st.Size(), path)
}

func writeGoFile(p string, data []byte) error { return os.WriteFile(p, data, 0644) }

func writeGoOSync(p string, data []byte) error {
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|os.O_SYNC, 0644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func writeFABuffer(p string, data []byte) error {
	f := FileAccess.Open(p, FileAccess.Write)
	if openErr := FileAccess.GetOpenError(); openErr != nil {
		return openErr
	}
	if !f.StoreBuffer(data) {
		return fmt.Errorf("FileAccess.StoreBuffer returned false")
	}
	f.Flush()
	return f.Close()
}

func writeFAString(p string, data []byte) error {
	f := FileAccess.Open(p, FileAccess.Write)
	if openErr := FileAccess.GetOpenError(); openErr != nil {
		return openErr
	}
	if !f.StoreString(string(data)) {
		return fmt.Errorf("FileAccess.StoreString returned false")
	}
	f.Flush()
	return f.Close()
}
