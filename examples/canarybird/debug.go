package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// debugf prints a debug line to stdout, prefixed with "DEBUG: ". It is
// used for logging from the Go side of canarybird, which is not visible
// in the engine console.
func debugf(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	fmt.Println("DEBUG: " + line)
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
