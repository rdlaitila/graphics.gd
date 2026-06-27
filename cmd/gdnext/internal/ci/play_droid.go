package ci

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"graphics.gd/product"
)

// androidPlayOpts mirrors browserPlayOpts but for the android-emu
// path. The play-cell dispatcher hands one of these to
// runAndroidPlay when compat=android-emu; the contract (the file
// pair reportPath + screenshotPath landing on the host) is
// identical to every other dispatch path.
type androidPlayOpts struct {
	scratch        string
	target         string
	link           string
	compat         string
	buildHost      string
	reportPath     string
	screenshotPath string
	hud            string
	timeout        time.Duration
}

var androidReportPrefix = product.EnvPlayReport + ":"

// runAndroidPlay installs the staged APK on the connected emulator,
// launches the activity, polls logcat for the play-bot's tagged
// report line (the Go runtime routes stdout to logcat under the
// "Go" tag), and pulls a host-side screencap. Release APKs aren't
// debuggable so run-as is off the table; logcat + screencap are
// both readable on any device without permissions, which is what
// the upstream android test harness already relies on.
func runAndroidPlay(opts androidPlayOpts) error {
	if opts.compat != "android-emu" && opts.compat != "waydroid" {
		return fmt.Errorf("android play: --compat must be android-emu or waydroid, got %q", opts.compat)
	}
	apk, err := androidAPKPath(opts.scratch, opts.target)
	if err != nil {
		return err
	}
	adbBin, err := exec.LookPath("adb")
	if err != nil {
		return fmt.Errorf("android play: adb not on PATH: %w", err)
	}
	dev, err := selectAdbDevice(adbBin, opts.compat)
	if err != nil {
		return err
	}
	aapt, err := findAapt()
	if err != nil {
		return err
	}
	pkg, err := androidPackageName(aapt, apk)
	if err != nil {
		return err
	}
	fmt.Printf("==> play %s [%s] compat=%s build-host=%s via adb %son package=%s apk=%s\n",
		opts.target, opts.link, opts.compat, buildHostOrLocal(opts.buildHost), dev.label(), pkg, apk)
	ctx, cancel := contextWithTimeout(opts.timeout)
	defer cancel()

	if out, err := dev.cmd("uninstall", pkg).CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "adb uninstall %s (ignored): %v\n%s", pkg, err, out)
	}
	if out, err := dev.cmd("install", "-r", "-g", apk).CombinedOutput(); err != nil {
		return fmt.Errorf("adb install %s: %w\n%s", apk, err, out)
	}
	defer func() { _ = dev.cmd("uninstall", pkg).Run() }()
	activity, err := androidLauncherActivity(ctx, dev, pkg)
	if err != nil {
		return err
	}
	_ = dev.cmd("shell", "am", "force-stop", pkg).Run()
	// Clear the logcat buffer so previous runs can't leak a stale
	// GDNEXT_PLAY_REPORT line into this read.
	_ = dev.cmd("logcat", "-c").Run()
	if out, err := dev.cmd("shell", "am", "start", "-n", activity).CombinedOutput(); err != nil {
		return fmt.Errorf("am start %s: %w\n%s", activity, err, out)
	}
	// Poll logcat for the play-bot's tagged report. -d dumps the
	// current buffer; the Go runtime emits stdout under the "Go" tag
	// at info level. Looping with -d keeps the read cheap and means
	// we can bail on ctx.Done() (the play-cell --timeout).
	deadline := time.Now()
	if opts.timeout > 0 {
		deadline = time.Now().Add(opts.timeout)
	}
	var reportBytes []byte
poll:
	for time.Now().Before(deadline) || opts.timeout == 0 {
		out, _ := dev.cmd("logcat", "-d", "-s", "Go:*").Output()
		if b64 := scanReportLine(string(out)); b64 != "" {
			if dec, err := base64.StdEncoding.DecodeString(b64); err == nil {
				reportBytes = dec
				break
			}
		}
		select {
		case <-ctx.Done():
			break poll
		case <-time.After(time.Second):
		}
	}
	if reportBytes != nil {
		if err := os.WriteFile(opts.reportPath, reportBytes, 0644); err != nil {
			return fmt.Errorf("write report %s: %w", opts.reportPath, err)
		}
	} else {
		// No report arrived. Dump the relevant logcat tags so the
		// CI log shows whether the activity launched at all, crashed
		// in native code, or simply silently exited. The bot's own
		// Go:* filter wouldn't surface any of that.
		dumpAndroidDiagnosticLogcat(dev, pkg)
	}
	// Always capture a host-side screenshot before tearing down so
	// the workflow summary still gets a frame even when the report
	// never arrived. `screencap -p` writes a PNG to stdout.
	if out, err := dev.cmd("exec-out", "screencap", "-p").Output(); err == nil && len(out) > 0 {
		_ = os.WriteFile(opts.screenshotPath, out, 0644)
	}
	_ = dev.cmd("shell", "am", "force-stop", pkg).Run()
	return nil
}

// scanReportLine walks `adb logcat` output and returns the base64
// payload of the most recent GDNEXT_PLAY_REPORT line. The logcat
// "raw" format isn't always available across SDK versions, so we
// tolerate any prefix and just search each line for the marker.
func scanReportLine(logcatOut string) string {
	for _, line := range strings.Split(logcatOut, "\n") {
		idx := strings.Index(line, androidReportPrefix)
		if idx < 0 {
			continue
		}
		return strings.TrimSpace(line[idx+len(androidReportPrefix):])
	}
	return ""
}

// androidAPKPath resolves the playable APK written by `gdnext build`
// for an android target tuple (see build_target.go.assertDistributable).
func androidAPKPath(scratch, target string) (string, error) {
	plat, ok := parseTuple(target)
	if !ok {
		return "", fmt.Errorf("android play: invalid --target %q", target)
	}
	dir := filepath.Join(scratch, "releases", "android", plat.GOARCH)
	matches, err := filepath.Glob(filepath.Join(dir, "*.apk"))
	if err != nil || len(matches) == 0 {
		return "", fmt.Errorf("android play: no apk under %s", dir)
	}
	return matches[0], nil
}

// androidPackageName runs `aapt2 dump packagename` to read the
// package id off the APK manifest.
func androidPackageName(aapt, apk string) (string, error) {
	out, err := exec.Command(aapt, "dump", "packagename", apk).Output()
	if err != nil {
		return "", fmt.Errorf("aapt dump packagename: %w", err)
	}
	pkg := strings.TrimSpace(string(out))
	if pkg == "" {
		return "", fmt.Errorf("aapt returned empty packagename for %s", apk)
	}
	return pkg, nil
}

// findAapt resolves an aapt2 (or aapt) binary the dispatcher can
// call. The reactivecircus emulator action only puts platform-tools
// on PATH (adb); build-tools aren't exposed shell-side, so we also
// look under gdnext's managed install root ($GDPATH/bin, default
// ~/gd/bin) where `gdnext toolchain install` drops aapt2.
func findAapt() (string, error) {
	if p, err := exec.LookPath("aapt2"); err == nil {
		return p, nil
	}
	if p, err := exec.LookPath("aapt"); err == nil {
		return p, nil
	}
	roots := []string{os.Getenv(product.EnvGDPath)}
	if home, err := os.UserHomeDir(); err == nil {
		roots = append(roots, filepath.Join(home, "gd"))
	}
	for _, root := range roots {
		if root == "" {
			continue
		}
		for _, name := range []string{"aapt2", "aapt"} {
			candidate := filepath.Join(root, "bin", name)
			if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
				return candidate, nil
			}
		}
	}
	return "", fmt.Errorf("android play: aapt2 not on PATH and not under $GDPATH/bin (run `gdnext toolchain install`)")
}

// androidLauncherActivity asks the package manager to resolve the
// LAUNCHER intent for pkg, retrying briefly because right after
// install the activity may not be registered yet.
func androidLauncherActivity(ctx context.Context, dev adbDevice, pkg string) (string, error) {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		out, _ := dev.cmd("shell", "cmd", "package", "resolve-activity", "--brief",
			"-c", "android.intent.category.LAUNCHER", pkg).Output()
		for _, ln := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			ln = strings.TrimSpace(ln)
			if strings.HasPrefix(ln, pkg+"/") {
				return ln, nil
			}
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return "", fmt.Errorf("could not resolve launcher activity for %s", pkg)
}

// dumpAndroidDiagnosticLogcat prints logcat (-d) lines mentioning
// godot or at error/fatal level to stderr when the activity failed
// to emit GDNEXT_PLAY_REPORT. Full unfiltered dumps drowned the CI
// log; this keeps the signal (engine traces, crashes) and drops
// kernel/init/system_server chatter.
func dumpAndroidDiagnosticLogcat(dev adbDevice, pkg string) {
	out, err := dev.cmd("logcat", "-d").Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "diagnostic logcat failed: %v\n", err)
		return
	}
	fmt.Fprintf(os.Stderr, "\n==> diagnostic logcat for %s (godot/errors only):\n", pkg)
	for _, line := range strings.Split(string(out), "\n") {
		if androidLogcatLineRelevant(line) {
			fmt.Fprintln(os.Stderr, line)
		}
	}
}

// androidLogcatLineRelevant keeps lines that mention godot
// (case-insensitive) or carry an error/fatal level marker in either
// threadtime (` E ` / ` F `) or brief (`E/` / `F/`) format.
func androidLogcatLineRelevant(line string) bool {
	if line == "" {
		return false
	}
	lower := strings.ToLower(line)
	if strings.Contains(lower, "godot") {
		return true
	}
	if strings.Contains(line, " E ") || strings.Contains(line, " F ") {
		return true
	}
	if strings.HasPrefix(line, "E/") || strings.HasPrefix(line, "F/") {
		return true
	}
	return false
}

// adbDevice is a thin wrapper around `adb` that pins every
// invocation to a specific serial when one is set. android-emu
// leaves the serial blank and trusts that the runner exposes a
// single device; waydroid sets it to the Waydroid
// container's network endpoint after an `adb connect`.
type adbDevice struct {
	bin    string
	serial string
}

// cmd builds an *exec.Cmd that runs `adb [-s serial] args...`.
func (d adbDevice) cmd(args ...string) *exec.Cmd {
	if d.serial == "" {
		return exec.Command(d.bin, args...)
	}
	return exec.Command(d.bin, append([]string{"-s", d.serial}, args...)...)
}

// label returns a short fragment to splice into log lines that
// identifies which device adb is targeting (or empty for the
// implicit-default device).
func (d adbDevice) label() string {
	if d.serial == "" {
		return ""
	}
	return "[" + d.serial + "] "
}

// selectAdbDevice resolves the adb target for a play compat layer.
// android-emu: empty serial, relies on the implicit single device
// the emulator-runner action exposes. waydroid: connects
// to the Waydroid container endpoint (defaults to 127.0.0.1:5555;
// override via GDNEXT_WAYDROID_ADB).
func selectAdbDevice(adb, compat string) (adbDevice, error) {
	switch compat {
	case "android-emu":
		return adbDevice{bin: adb}, nil
	case "waydroid":
		addr := os.Getenv(product.EnvWaydroidADB)
		if addr == "" {
			addr = "127.0.0.1:5555"
		}
		if out, err := exec.Command(adb, "connect", addr).CombinedOutput(); err != nil {
			return adbDevice{}, fmt.Errorf("adb connect %s: %w\n%s", addr, err, out)
		} else {
			fmt.Fprintf(os.Stderr, "adb connect %s: %s", addr, out)
		}
		return adbDevice{bin: adb, serial: addr}, nil
	default:
		return adbDevice{}, fmt.Errorf("android play: unsupported compat %q", compat)
	}
}
