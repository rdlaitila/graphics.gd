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

const androidReportPrefix = "GDNEXT_PLAY_REPORT:"

// runAndroidPlay installs the staged APK on the connected emulator,
// launches the activity, polls logcat for the play-bot's tagged
// report line (the Go runtime routes stdout to logcat under the
// "Go" tag), and pulls a host-side screencap. Release APKs aren't
// debuggable so run-as is off the table; logcat + screencap are
// both readable on any device without permissions, which is what
// the upstream android test harness already relies on.
func runAndroidPlay(opts androidPlayOpts) error {
	if opts.compat != "android-emu" {
		return fmt.Errorf("android play: --compat must be android-emu, got %q", opts.compat)
	}
	apk, err := androidAPKPath(opts.scratch, opts.target)
	if err != nil {
		return err
	}
	adb, err := exec.LookPath("adb")
	if err != nil {
		return fmt.Errorf("android play: adb not on PATH: %w", err)
	}
	aapt, err := exec.LookPath("aapt2")
	if err != nil {
		if a, e := exec.LookPath("aapt"); e == nil {
			aapt = a
		} else {
			return fmt.Errorf("android play: aapt2 not on PATH (install build-tools): %w", err)
		}
	}
	pkg, err := androidPackageName(aapt, apk)
	if err != nil {
		return err
	}
	fmt.Printf("==> play %s [%s] compat=%s build-host=%s via adb on package=%s apk=%s\n",
		opts.target, opts.link, opts.compat, buildHostOrLocal(opts.buildHost), pkg, apk)
	ctx, cancel := contextWithTimeout(opts.timeout)
	defer cancel()

	if out, err := exec.Command(adb, "uninstall", pkg).CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "adb uninstall %s (ignored): %v\n%s", pkg, err, out)
	}
	if out, err := exec.Command(adb, "install", "-r", "-g", apk).CombinedOutput(); err != nil {
		return fmt.Errorf("adb install %s: %w\n%s", apk, err, out)
	}
	defer func() { _ = exec.Command(adb, "uninstall", pkg).Run() }()
	activity, err := androidLauncherActivity(ctx, adb, pkg)
	if err != nil {
		return err
	}
	_ = exec.Command(adb, "shell", "am", "force-stop", pkg).Run()
	// Clear the logcat buffer so previous runs can't leak a stale
	// GDNEXT_PLAY_REPORT line into this read.
	_ = exec.Command(adb, "logcat", "-c").Run()
	if out, err := exec.Command(adb, "shell", "am", "start", "-n", activity).CombinedOutput(); err != nil {
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
		out, _ := exec.Command(adb, "logcat", "-d", "-s", "Go:*").Output()
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
	}
	// Always capture a host-side screenshot before tearing down so
	// the workflow summary still gets a frame even when the report
	// never arrived. `screencap -p` writes a PNG to stdout.
	if out, err := exec.Command(adb, "exec-out", "screencap", "-p").Output(); err == nil && len(out) > 0 {
		_ = os.WriteFile(opts.screenshotPath, out, 0644)
	}
	_ = exec.Command(adb, "shell", "am", "force-stop", pkg).Run()
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

// androidLauncherActivity asks the package manager to resolve the
// LAUNCHER intent for pkg, retrying briefly because right after
// install the activity may not be registered yet.
func androidLauncherActivity(ctx context.Context, adb, pkg string) (string, error) {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		out, _ := exec.Command(adb, "shell", "cmd", "package", "resolve-activity", "--brief",
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
