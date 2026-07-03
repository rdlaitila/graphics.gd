package ci

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"graphics.gd/cmd/gdnext/internal/shared"
	"graphics.gd/product"
)

// Package-local aliases so existing ci/ call sites (announce, run,
// runIn, runEnv, runInEnv, output, outputCombined) stay unchanged while
// the shared implementations live in cmd/gdnext/internal/shared.
var (
	announce       = shared.Announce
	run            = shared.Run
	runIn          = shared.RunIn
	runEnv         = shared.RunEnv
	runInEnv       = shared.RunInEnv
	output         = shared.Output
	outputCombined = shared.OutputCombined
)

// dirSize returns the total bytes occupied by every regular file
// reachable from root. Missing root returns (0, nil) so the diagnostic
// `before == after` check works on a fresh CI runner with no $GDPATH.
func dirSize(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return fs.SkipDir
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}
		total += info.Size()
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return total, err
	}
	return total, nil
}

// graphicsGDRoot resolves the path of the graphics.gd checkout the
// staged example should replace into its go.mod. Priority:
//
//  1. explicit --root / first arg if provided by caller
//  2. $GRAPHICS_GD_ROOT env (local dev)
//  3. $GITHUB_WORKSPACE env (CI)
//  4. walk up from the current working directory until we find a
//     go.mod whose module line says "module graphics.gd"
//
// Returns "" + an error if none of the above succeed; callers should
// surface that error rather than guess.
func graphicsGDRoot(explicit string) (string, error) {
	if explicit != "" {
		return filepath.Abs(explicit)
	}
	if v := os.Getenv(product.EnvGraphicsGDRoot); v != "" {
		return filepath.Abs(v)
	}
	if v := os.Getenv(product.EnvGitHubWorkspace); v != "" {
		return filepath.Abs(v)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := cwd
	for {
		mod := filepath.Join(dir, "go.mod")
		b, err := os.ReadFile(mod)
		if err == nil && bytes.Contains(b, []byte("module graphics.gd\n")) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not locate graphics.gd checkout root; set GRAPHICS_GD_ROOT or GITHUB_WORKSPACE")
		}
		dir = parent
	}
}

// mustContain reports an error when haystack does not contain needle,
// the Go equivalent of `grep -q` against captured output. Used by the
// help-text + go-passthrough smoke tests.
func mustContain(label, haystack, needle string) error {
	if strings.Contains(haystack, needle) {
		return nil
	}
	return fmt.Errorf("%s: expected output to contain %q, got:\n%s", label, needle, haystack)
}

// copyTree recursively copies the contents of src into dst (which must
// already exist). Preserves file modes; not symlinks (the examples tree
// has none today). Mirrors `cp -R "$src"/. "$dst"/` from the original
// stage-example.sh.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			return os.MkdirAll(target, info.Mode().Perm())
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
