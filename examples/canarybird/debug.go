package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"graphics.gd/classdb/Engine"
)

// debugf prints a single line to both the Go stdout (so CI logs see it
// even before the engine is initialised, and so it shows up under
// "Show play report" before any Godot output) and the engine print
// stream (so the in-engine console / editor output shows it too).
// Use sparingly — every call is a duplicated line in CI logs.
func debugf(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	fmt.Println("GO: " + line)
	// Engine.Print only works after the engine has booted enough for
	// classdb to be live. Wrapping in recover keeps a too-early call
	// from panicking out of init code, which would lose the more
	// useful Go-stdout line we just printed.
	defer func() { _ = recover() }()
	Engine.Print("GD: " + line)
}

// dumpEnv prints every environment variable to stdout (sorted), tagged
// with where it was called from. Sorted so successive dumps diff
// cleanly. Only emitted on the Go side — the engine console doesn't
// need the wall of text.
func dumpEnv(where string) {
	env := os.Environ()
	sort.Strings(env)
	fmt.Printf("==> env dump (%s, %d entries)\n", where, len(env))
	for _, kv := range env {
		// Truncate enormous values (e.g. PATH on windows runners) so
		// the dump stays readable.
		if len(kv) > 512 {
			kv = kv[:509] + "..."
		}
		fmt.Println("    " + kv)
	}
}

// hasPrefixAny reports whether s starts with any of prefixes.
//
//nolint:unused // kept for future filtered dumps.
func hasPrefixAny(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}
