// Package projmod contains the optional "tool autoinstall" helpers that
// were drafted in cmd/gd/main.go (currently commented out) to make gdnext
// re-exec via "go tool gd" whenever the project's go.mod pins a different
// graphics.gd version. They're kept here as exported helpers ready for a
// future re-enablement of that feature.
package projmod

import (
	"os"
	"path/filepath"
	"strings"

	"graphics.gd/cmd/gdnext/internal/shared"
)

// FindProjectGoMod walks up from the current working directory looking for a
// go.mod file. Returns (dir, goModPath, true) when found, ("", "", false)
// otherwise.
func FindProjectGoMod() (dir string, goModPath string, ok bool) {
	wd, err := os.Getwd()
	if err != nil {
		return "", "", false
	}
	for last := ""; last != wd; last, wd = wd, filepath.Dir(wd) {
		path := filepath.Join(wd, "go.mod")
		if _, err := os.Stat(path); err == nil {
			return wd, path, true
		}
	}
	return "", "", false
}

// ReadGoModGraphicsVersion reads a go.mod file and returns the required
// version of graphics.gd, or an empty string if graphics.gd is not a
// dependency.
func ReadGoModGraphicsVersion(goModPath string) string {
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "graphics.gd ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "graphics.gd "))
		}
	}
	return ""
}

// EnsureGoToolGd checks if graphics.gd/cmd/gd is registered as a tool in
// go.mod, and if not runs "go get -tool graphics.gd/cmd/gd" to add it. Best
// effort — errors are swallowed because this is optional bootstrap.
func EnsureGoToolGd(goPath, dir, goModPath string) {
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return
	}
	if strings.Contains(string(data), "graphics.gd/cmd/gd") {
		return
	}
	_ = shared.RunIn(dir, goPath, "get", "-tool", "graphics.gd/cmd/gd")
}
