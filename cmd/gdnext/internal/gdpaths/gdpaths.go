// Package gdpaths resolves the two directories gdnext caches downloaded
// toolchains and libraries under. The root (GDPATH) is configurable via the
// $GDPATH env var or the gdnext --gdpath flag; it defaults to ~/gd.
//
// Resolution is intentionally lazy: every call reads the env at use time so
// that gdnext's CLI Before hook (which pushes the --gdpath flag into the
// env) is honoured even when this package was imported earlier.
package gdpaths

import (
	"os"
	"os/user"
	"path/filepath"
)

// Root returns the active GDPATH (the value of $GDPATH if set, else
// ~/gd). Empty string is only returned if neither $GDPATH is set nor a
// home directory can be resolved.
func Root() string {
	if v := os.Getenv("GDPATH"); v != "" {
		return v
	}
	if whoami, err := user.Current(); err == nil {
		return filepath.Join(whoami.HomeDir, "gd")
	}
	return ""
}

// Bin returns the GDPATH/bin directory used to store downloaded
// executables.
func Bin() string { return filepath.Join(Root(), "bin") }

// Lib returns the GDPATH/lib directory used to store downloaded static
// libraries (libgodot.*, musl staging, etc.).
func Lib() string { return filepath.Join(Root(), "lib") }
