package main

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// announce prints a copy-pasteable banner for the command about to run.
// One blank line precedes the arrow so adjacent invocations stand apart
// in CI log output without needing per-call fmt.Println bookkeeping.
func announce(dir string, extraEnv []string, name string, args []string) {
	var b strings.Builder
	b.WriteString("\n==> ")
	for _, kv := range extraEnv {
		b.WriteString(kv)
		b.WriteByte(' ')
	}
	b.WriteString(shellQuote(name))
	for _, a := range args {
		b.WriteByte(' ')
		b.WriteString(shellQuote(a))
	}
	if dir != "" {
		b.WriteString("   # in ")
		b.WriteString(dir)
	}
	fmt.Fprintln(os.Stderr, b.String())
}

// shellQuote returns s wrapped in single quotes when it contains
// whitespace or shell metacharacters, so the announcement can be copy-
// pasted into a terminal. Plain alnum/`-_./:=` stay unquoted.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		switch r {
		case '-', '_', '.', '/', ':', '=', ',', '@', '+':
			continue
		}
		return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
	}
	return s
}

// run executes cmd args... with stdout+stderr forwarded to ours and
// stdin closed (so interactive prompts inside gdnext see EOF, just
// like the shell scripts' `< /dev/null` redirect). Returns the first
// error from exec. Prints a `==> cmd args` banner first so adjacent
// invocations are distinguishable in CI logs.
func run(name string, args ...string) error {
	return runEnv(nil, name, args...)
}

// runIn is run with a working directory set to dir.
func runIn(dir, name string, args ...string) error {
	return runInEnv(dir, nil, name, args...)
}

// runEnv is run with extra env entries appended to os.Environ() —
// used by build-target's `GOOS=... GOARCH=... gdnext toolchain doctor`
// invocation, where the env decides what counts as REQUIRED.
func runEnv(extraEnv []string, name string, args ...string) error {
	return runInEnv("", extraEnv, name, args...)
}

// runInEnv is the general form: extra env + working directory.
func runInEnv(dir string, extraEnv []string, name string, args ...string) error {
	announce(dir, extraEnv, name, args)
	c := exec.Command(name, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = nil
	if dir != "" {
		c.Dir = dir
	}
	if len(extraEnv) > 0 {
		c.Env = append(os.Environ(), extraEnv...)
	}
	return c.Run()
}

// output runs the command and returns trimmed stdout. Stderr is
// inherited so failures still surface their diagnostics live. Also
// announces the command so output-capturing calls show up in the log
// alongside run() invocations.
func output(name string, args ...string) (string, error) {
	announce("", nil, name, args)
	c := exec.Command(name, args...)
	c.Stderr = os.Stderr
	c.Stdin = nil
	out, err := c.Output()
	return strings.TrimSpace(string(out)), err
}

// outputCombined captures stdout+stderr together (mirrors `2>&1` in
// shell). Used when a command may print failure context to stderr but
// the caller still wants to display the combined transcript before
// deciding whether the failure is acceptable (toolchain install of
// optional tools is the only current user).
func outputCombined(name string, args ...string) (string, error) {
	announce("", nil, name, args)
	c := exec.Command(name, args...)
	c.Stdin = nil
	var buf bytes.Buffer
	c.Stdout = &buf
	c.Stderr = &buf
	err := c.Run()
	return buf.String(), err
}

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
	if v := os.Getenv("GRAPHICS_GD_ROOT"); v != "" {
		return filepath.Abs(v)
	}
	if v := os.Getenv("GITHUB_WORKSPACE"); v != "" {
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
