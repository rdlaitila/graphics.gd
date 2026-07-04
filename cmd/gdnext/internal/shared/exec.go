package shared

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"graphics.gd/product"
)

// debugTrace is true when GD_DEBUG is set. Resolved once at package
// init so per-call cost stays at a single boolean read.
var debugTrace = os.Getenv(product.EnvDebug) != ""

// Announce prints a copy-pasteable banner for the command about to
// run. Extra env entries render as `KEY=val ...` before the command,
// matching the shell prefix syntax. When dir is non-empty a
// `directory:` trailer is appended. GD_DEBUG=1 additionally
// appends the Go call site chain up to the first frame outside shared.
func Announce(dir string, extraEnv []string, name string, args []string) {
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
		b.WriteString("\ndirectory: ")
		b.WriteString(dir)
	}
	if debugTrace {
		if trace := stackTrace(); trace != "" {
			b.WriteByte('\n')
			b.WriteString(trace)
		}
	}
	fmt.Fprintln(os.Stderr, b.String())
}

// stackTrace collects Announce's caller chain, skipping frames inside
// the shared package (so the top of the trace is the real exec site).
// Renders as `    <pkg>.<func> at <file>:<line>` per frame, one per
// line, up to 12 frames.
func stackTrace() string {
	var pcs [16]uintptr
	n := runtime.Callers(2, pcs[:]) // skip runtime.Callers + stackTrace
	if n == 0 {
		return ""
	}
	frames := runtime.CallersFrames(pcs[:n])
	var b strings.Builder
	shown := 0
	for {
		f, more := frames.Next()
		if !strings.HasPrefix(f.Function, "graphics.gd/cmd/gdnext/internal/shared.") {
			if shown > 0 {
				b.WriteByte('\n')
			}
			fmt.Fprintf(&b, "    %s at %s:%d", f.Function, trimRepoPath(f.File), f.Line)
			shown++
			if shown >= 12 {
				break
			}
		}
		if !more {
			break
		}
	}
	return b.String()
}

// trimRepoPath collapses an absolute source path down to its repo-
// relative form. Falls back to the basename when the layout has no
// `graphics.gd/` segment (module-cache builds).
func trimRepoPath(p string) string {
	const marker = "graphics.gd/"
	if i := strings.LastIndex(p, marker); i >= 0 {
		return p[i+len(marker):]
	}
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}

// shellQuote returns s wrapped in single quotes when it contains
// whitespace or shell metacharacters, so the announcement can be copy-
// pasted into a terminal. Plain alnum/`-_./:=,@+` stay unquoted.
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

// Run executes name args... with stdout+stderr forwarded and stdin
// closed. Announces first so adjacent invocations are distinguishable.
func Run(name string, args ...string) error {
	return RunInEnv("", nil, name, args...)
}

// RunIn is Run with a working directory.
func RunIn(dir, name string, args ...string) error {
	return RunInEnv(dir, nil, name, args...)
}

// RunEnv is Run with extra env entries appended to os.Environ().
func RunEnv(extraEnv []string, name string, args ...string) error {
	return RunInEnv("", extraEnv, name, args...)
}

// RunInEnv combines Dir + extra env. Zero-value dir / nil extraEnv are
// treated as unset.
func RunInEnv(dir string, extraEnv []string, name string, args ...string) error {
	Announce(dir, []string{}, name, args)
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

// RunStdin is Run with stdin piped from r. Working dir + env stay
// unset; add fields on the returned *exec.Cmd via a caller-side helper
// if you need more. Used by ar -M and other tools that read a script
// from stdin.
func RunStdin(r io.Reader, name string, args ...string) error {
	Announce("", nil, name, args)
	c := exec.Command(name, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = r
	return c.Run()
}

// RunInteractive is Run with stdin wired to os.Stdin so the child can
// read from the user's terminal. Used by tools that prompt (godot
// editor, godot --headless --export-debug with a live keystore
// password, etc.).
func RunInteractive(name string, args ...string) error {
	Announce("", nil, name, args)
	c := exec.Command(name, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	return c.Run()
}

// RunInEnvStdin is the general form covering dir + env + stdin. Passed
// env fully overrides os.Environ() when replaceEnv is true; otherwise
// entries append to it (matching RunInEnv).
func RunInEnvStdin(dir string, extraEnv []string, replaceEnv bool, stdin io.Reader, name string, args ...string) error {
	Announce(dir, []string{}, name, args)
	c := exec.Command(name, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = stdin
	if dir != "" {
		c.Dir = dir
	}
	switch {
	case replaceEnv:
		c.Env = extraEnv
	case len(extraEnv) > 0:
		c.Env = append(os.Environ(), extraEnv...)
	}
	return c.Run()
}

// Output runs name args... and returns trimmed stdout. Stderr is
// inherited so failures still surface diagnostics live. Announces the
// command so output-capturing invocations stay visible.
func Output(name string, args ...string) (string, error) {
	return OutputIn("", nil, name, args...)
}

// OutputIn is Output with dir + extra env.
func OutputIn(dir string, extraEnv []string, name string, args ...string) (string, error) {
	Announce(dir, []string{}, name, args)
	c := exec.Command(name, args...)
	c.Stderr = os.Stderr
	c.Stdin = nil
	if dir != "" {
		c.Dir = dir
	}
	if len(extraEnv) > 0 {
		c.Env = append(os.Environ(), extraEnv...)
	}
	out, err := c.Output()
	return strings.TrimSpace(string(out)), err
}

// OutputCombined captures stdout+stderr into a single buffer (mirrors
// `2>&1`). Used when the caller wants to display the combined
// transcript before deciding whether the failure is acceptable.
func OutputCombined(name string, args ...string) (string, error) {
	Announce("", nil, name, args)
	c := exec.Command(name, args...)
	c.Stdin = nil
	var buf bytes.Buffer
	c.Stdout = &buf
	c.Stderr = &buf
	err := c.Run()
	return buf.String(), err
}

// OutputBytes runs name args... and returns raw stdout (untrimmed).
// Used when the caller needs to feed the exact bytes to a downstream
// parser (adb pidof output, gh api JSON, etc.) rather than a trimmed
// string. Stderr is inherited so failures surface diagnostics live.
func OutputBytes(name string, args ...string) ([]byte, error) {
	Announce("", nil, name, args)
	c := exec.Command(name, args...)
	c.Stderr = os.Stderr
	c.Stdin = nil
	return c.Output()
}

// OutputBytesCapture is like OutputBytes but leaves stderr unassigned
// so os/exec populates ExitError.Stderr on failure. Callers use this
// when they inspect ExitError.Stderr to detect specific error messages
// (e.g. gh CLI "Not Found" → 404) rather than surface stderr live.
func OutputBytesCapture(name string, args ...string) ([]byte, error) {
	Announce("", nil, name, args)
	c := exec.Command(name, args...)
	c.Stdin = nil
	return c.Output()
}

// ProbeCombined runs a --version / -dumpversion style probe with
// stdout+stderr captured together. Silent by default; announces only
// under GD_DEBUG so bulk resolver walks don't drown the log.
func ProbeCombined(name string, args ...string) ([]byte, error) {
	if debugTrace {
		Announce("", nil, name, args)
	}
	c := exec.Command(name, args...)
	c.Stdin = nil
	return c.CombinedOutput()
}

// RunContext is Run with a context, so cancellation / timeout kills
// the child. Callers that also need env / dir / stdin build the
// *exec.Cmd themselves after Announce; this covers the plain
// long-running-with-timeout case.
func RunContext(ctx context.Context, name string, args ...string) error {
	Announce("", nil, name, args)
	c := exec.CommandContext(ctx, name, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = nil
	return c.Run()
}
