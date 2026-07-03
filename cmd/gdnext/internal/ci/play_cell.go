package ci

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"graphics.gd/cmd/gdnext/internal/shared"
	"graphics.gd/product"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
)

// PlayCellCommand wires `gdnext ci play-cell`. Runtime state lives
// on *PlayCellActions.
type PlayCellCommand struct {
	*cli.Command
	Injector do.Injector `do:""`
}

// PlayCellActions carries the runtime state.
type PlayCellActions struct{}

type target struct{ GOOS, GOARCH string }

// NewPlayCellCommand constructs the play-cell subcommand.
func NewPlayCellCommand(di do.Injector) (*PlayCellCommand, error) {
	t := do.MustInvokeStruct[*PlayCellCommand](di)
	t.Command = &cli.Command{
		Name:  "play-cell",
		Usage: "drive one already-built example headlessly via the play-bot",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "scratch", Required: true, Usage: "directory containing the built example (releases/<goos>/<goarch>/...)"},
			&cli.StringFlag{Name: "example", Required: true, Usage: "example name (binary basename inside releases/<goos>/<goarch>/)"},
			&cli.StringFlag{Name: "target", Required: true, Usage: "target goos/goarch (e.g. linux/amd64, windows/amd64)"},
			&cli.StringFlag{Name: "link", Usage: "link mode (gdextension|libgodot); blank = platform default"},
			&cli.StringFlag{Name: "libc", Usage: "linux libc variant (glibc|musl); surfaces in the HUD next to link mode"},
			&cli.StringFlag{Name: "compat", Usage: "compatibility layer to drive the target through (wine|proton|proton-9|...); blank = native"},
			&cli.StringFlag{Name: "build-host", Usage: "GHA runner label that produced the artefact (informational; surfaced on the HUD)"},
			&cli.StringFlag{Name: "screenshot", Required: true, Usage: "absolute path the play-bot writes the screenshot to"},
			&cli.DurationFlag{Name: "timeout", Value: 90 * time.Second, Usage: "hard kill after this much wall-clock time"},
			&cli.BoolFlag{Name: "no-xvfb", Usage: "skip the xvfb-run wrapper even on headless hosts (binary must bring its own display, or use --headless)"},
		},
		Action: shared.BindAction(t.Injector, (*PlayCellActions).action),
	}
	return t, nil
}

// NewPlayCellActions resolves the runtime state.
func NewPlayCellActions(di do.Injector) (*PlayCellActions, error) {
	return do.InvokeStruct[*PlayCellActions](di)
}

func (t *PlayCellActions) action(_ context.Context, cmd *cli.Command) error {
	scratch, err := filepath.Abs(cmd.String("scratch"))
	if err != nil {
		return err
	}
	example := cmd.String("example")
	target := cmd.String("target")
	link := cmd.String("link")
	libc := cmd.String("libc")
	compat := cmd.String("compat")
	buildHost := cmd.String("build-host")
	timeout := cmd.Duration("timeout")
	noXvfb := cmd.Bool("no-xvfb")
	plat, ok := parseTuple(target)
	if !ok {
		return fmt.Errorf("invalid --target %q (want goos/goarch)", target)
	}
	mode, err := product.ParseLinkMode(link)
	if err != nil {
		return err
	}
	if mode == 0 {
		mode = product.GDExtension
	}
	reportPath := filepath.Join(scratch, "play-report.json")
	_ = os.Remove(reportPath)
	screenshotPath, err := filepath.Abs(cmd.String("screenshot"))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(screenshotPath), 0755); err != nil {
		return err
	}
	_ = os.Remove(screenshotPath)
	hud := buildPlayHUD(target, mode, libc, compat, buildHost)
	var runErr error
	switch {
	case plat.GOOS == product.GOOSJS || compat == "chrome" || compat == "firefox":
		runErr = runBrowserPlay(browserPlayOpts{
			scratch:        scratch,
			target:         target,
			link:           mode.String(),
			compat:         compat,
			buildHost:      buildHost,
			reportPath:     reportPath,
			screenshotPath: screenshotPath,
			hud:            hud,
			timeout:        timeout,
		})
	case plat.GOOS == product.GOOSAndroid || compat == "android-emu" || compat == "waydroid":
		runErr = runAndroidPlay(androidPlayOpts{
			scratch:        scratch,
			target:         target,
			link:           mode.String(),
			compat:         compat,
			buildHost:      buildHost,
			reportPath:     reportPath,
			screenshotPath: screenshotPath,
			hud:            hud,
			timeout:        timeout,
		})
	default:
		bin, err := releaseBinary(scratch, example, plat, mode)
		if err != nil {
			return err
		}
		if err := os.Chmod(bin, 0755); err != nil {
			return fmt.Errorf("chmod +x %s: %w", bin, err)
		}
		// The proton branch of launchCommand no longer wraps in
		// xvfb-run — umu-run re-execs inside pressure-vessel's
		// bubblewrap and xvfb-run's env-prefix DISPLAY doesn't
		// survive. Bring up a persistent Xvfb here so we can put
		// DISPLAY into c.Env directly.
		xvfbDisplay := ""
		if protonRelease(compat) != "" {
			disp, stop, err := startXvfb(context.Background())
			if err != nil {
				return err
			}
			defer stop()
			xvfbDisplay = disp
			// Use the same log dir Proton writes into.
			defer protonDumpLog(scratch)
		}
		argv, err := launchCommand(bin, plat, mode, compat, noXvfb)
		if err != nil {
			return err
		}
		fmt.Printf("==> play %s [%s] compat=%s build-host=%s\n", target, mode, compatOrNative(compat), buildHostOrLocal(buildHost))
		shared.Announce("", nil, argv[0], argv[1:])
		ctx, cancel := contextWithTimeout(timeout)
		defer cancel()
		c := exec.CommandContext(ctx, argv[0], argv[1:]...)
		// GDNEXT_PLAY_* paths must be translated to Z:\... form for
		// wine/proton so the windows binary can find the host file
		// (see winePath). Native cells keep the linux path as-is.
		childResult := reportPath
		childScreenshot := screenshotPath
		childHUD := hud
		if needsWinePathTranslation(compat, plat) {
			childResult = winePath(reportPath)
			childScreenshot = winePath(screenshotPath)
		}
		c.Env = append(os.Environ(),
			product.EnvPlay+"=active",
			product.EnvPlayResult+"="+childResult,
			product.EnvPlayScreenshot+"="+childScreenshot,
			product.EnvPlayHUD+"="+childHUD,
		)
		c.Env = append(c.Env, protonEnv(compat, scratch)...)
		if xvfbDisplay != "" {
			c.Env = append(c.Env, product.EnvDisplay+"="+xvfbDisplay)
			// pressure-vessel's SDL loader will try Wayland first
			// otherwise and fail cryptically before Xvfb is even
			// consulted.
			c.Env = append(c.Env, "SDL_VIDEODRIVER=x11")
		}
		fmt.Printf("==> play env: %s=active %s=%q %s=%q %s=%q\n",
			product.EnvPlay,
			product.EnvPlayResult, childResult,
			product.EnvPlayScreenshot, childScreenshot,
			product.EnvPlayHUD, childHUD,
		)
		if xvfbDisplay != "" {
			fmt.Printf("==> play xvfb: DISPLAY=%s SDL_VIDEODRIVER=x11\n", xvfbDisplay)
		}
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		runErr = c.Run()
	}
	if runErr != nil {
		// engine exit is best-effort: bot quits via SceneTree which exits 0 even on player crash; the report is the source of truth.
		fmt.Fprintf(os.Stderr, "engine exit: %v\n", runErr)
	}
	if st, err := os.Stat(screenshotPath); err == nil {
		fmt.Printf("==> screenshot: %s (%d bytes)\n", screenshotPath, st.Size())
	}
	report, err := readReport(reportPath)
	if err != nil {
		return fmt.Errorf("play report unreadable at %s: %w (engine exit: %v)", reportPath, err, runErr)
	}
	fmt.Printf("==> report: success=%v game_data=%s\n", report.Success, gameDataSummary(report.GameData))
	if !report.Success {
		return fmt.Errorf("play failed: example reported success=false (game_data=%s)", gameDataSummary(report.GameData))
	}
	return nil
}

// gameDataSummary renders a compact one-line preview of an example's
// game_data blob for the CI log. Empty payloads render as `{}`.
func gameDataSummary(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "{}"
	}
	return string(raw)
}

// releaseBinary resolves the playable artefact written by `gdnext
// build` (see build_target.go.assertDistributable).
func releaseBinary(scratch, example string, plat target, mode product.LinkMode) (string, error) {
	if mode.Has(product.LibGodot) {
		return mustExecutable(filepath.Join(scratch, "releases", "linux", plat.GOARCH, example))
	}
	switch plat.GOOS {
	case product.GOOSLinux:
		return mustExecutable(filepath.Join(scratch, "releases", "linux", plat.GOARCH, example))
	case product.GOOSWindows:
		return mustExecutable(filepath.Join(scratch, "releases", "windows", plat.GOARCH, example+".exe"))
	case product.GOOSDarwin:
		return mustExecutable(filepath.Join(scratch, "releases", "darwin", "universal", example+".app", "Contents", "MacOS", example))
	default:
		return "", fmt.Errorf("no play recipe for %s/%s (link=%s)", plat.GOOS, plat.GOARCH, mode)
	}
}

// launchCommand wraps the produced binary in whatever runtime the
// play host needs to drive a (potentially foreign) target. compat
// selects the wrapper explicitly ("wine", "proton", "proton-<ver>",
// or ""/"native" for direct execution); the dispatcher falls back to
// auto-detect when compat is unset so local invocations of play-cell
// keep working. Proton variants are routed through umu-run, with the
// concrete GE-Proton tag resolved by protonRelease(); the caller is
// expected to set PROTONPATH in the child env (see protonEnv()).
// noXvfb forces the xvfb-run wrapper off even on headless hosts; the
// caller is then responsible for providing a display (or passing
// --headless to the engine via some other path).
func launchCommand(bin string, plat target, mode product.LinkMode, compat string, noXvfb bool) ([]string, error) {
	xvfb := withXvfb
	if noXvfb {
		xvfb = func(argv ...string) []string { return argv }
	}
	hostGOOS := runtime.GOOS
	switch compat {
	case "", "native":
		switch {
		case hostGOOS == product.GOOSLinux && plat.GOOS == product.GOOSLinux:
			return xvfb(bin), nil
		case hostGOOS == product.GOOSLinux && plat.GOOS == product.GOOSWindows:
			wine, err := exec.LookPath("wine")
			if err != nil {
				return nil, fmt.Errorf("linux→windows play needs wine on PATH (or --compat=wine|proton): %w", err)
			}
			return xvfb(wine, bin), nil
		case hostGOOS == product.GOOSWindows && plat.GOOS == product.GOOSWindows:
			return []string{bin}, nil
		case hostGOOS == product.GOOSDarwin && plat.GOOS == product.GOOSDarwin:
			return []string{bin}, nil
		}
	case "wine":
		wine, err := exec.LookPath("wine")
		if err != nil {
			return nil, fmt.Errorf("compat=wine: wine not on PATH: %w", err)
		}
		return xvfb(wine, bin), nil
	case "proton", "proton-8", "proton-9", "proton-10":
		umu, err := exec.LookPath("umu-run")
		if err != nil {
			return nil, fmt.Errorf("compat=%s: umu-run not on PATH (apt install umu-launcher or pip install umu-launcher): %w", compat, err)
		}
		// No xvfb-run wrap: umu-run re-execs inside pressure-vessel's
		// bubblewrap which drops the env-prefix DISPLAY. The caller
		// stands up a persistent Xvfb and puts DISPLAY into c.Env
		// instead so it survives the re-exec.
		return []string{umu, bin}, nil
	case "rosetta":
		if hostGOOS != product.GOOSDarwin {
			return nil, fmt.Errorf("compat=rosetta only runs on darwin hosts (got %s)", hostGOOS)
		}
		return []string{"arch", "-x86_64", bin}, nil
	default:
		return nil, fmt.Errorf("unknown --compat %q", compat)
	}
	return nil, fmt.Errorf("no launch recipe: host=%s target=%s/%s compat=%s", hostGOOS, plat.GOOS, plat.GOARCH, compat)
}

// protonRelease returns the GE-Proton tag pinned to a compat token.
// Pinned versions mirror Steam's bundled compatibility-tool dropdown
// so users can correlate a CI green/red signal with the Proton they
// actually run locally:
//
//	proton    -> latest GE-Proton (Proton 11 series, alias for users
//	             who don't care which sub-version)
//	proton-10 -> last GE release on the Proton 10 line
//	proton-9  -> last GE release on the Proton 9 line ("Proton 9.0")
//	proton-8  -> last GE release on the Proton 8 line ("Proton 8.0")
//
// Used by the workflow's install step to fetch the tarball and by
// protonEnv() to set PROTONPATH. Returns "" for non-proton tokens.
func protonRelease(compat string) string {
	switch compat {
	case "proton":
		return "GE-Proton11-1"
	case "proton-10":
		return "GE-Proton10-34"
	case "proton-9":
		return "GE-Proton9-25"
	case "proton-8":
		return "GE-Proton8-32"
	}
	return ""
}

// protonEnv returns the env vars umu-run needs to drive bin through
// the GE-Proton release pinned to compat. Empty slice for non-proton
// tokens. GAMEID=0 selects the umu-default protonfix. logDir is
// where PROTON_LOG=1 tells Proton to write its steam-<gameid>.log —
// dumped to stdout by protonDumpLog() after the run so it's visible
// in the CI job log.
func protonEnv(compat, logDir string) []string {
	tag := protonRelease(compat)
	if tag == "" {
		return nil
	}
	home := os.Getenv(product.EnvHome)
	if home == "" {
		home = "."
	}
	pp := filepath.Join(home, ".local", "share", "Steam", "compatibilitytools.d", tag)
	return []string{
		"GAMEID=0",
		"PROTONPATH=" + pp,
		// GE-Proton11+ ships Xalia, an accessibility helper that
		// initializes SDL3 before launching the game. Inside
		// pressure-vessel's bubblewrap container it can't see the
		// host Xvfb socket and crashes the whole launch with
		// 'No displays available' before godot starts. The real
		// disable knob is PROTON_USE_XALIA=0 (see Proton's `proton`
		// launcher + README; PROTON_DISABLE_XALIA does not exist).
		// We don't need accessibility hooks in headless CI.
		"PROTON_USE_XALIA=0",
		// Proton's `proton` launcher hard-codes stdout/stderr of
		// every wine subprocess into a log file. Without PROTON_LOG
		// the log path is /dev/null-equivalent (WINEDEBUG=-all), so
		// Godot's own output never reaches our c.Stdout. Set it to
		// 1 for a general log; drop the file into logDir so
		// protonDumpLog can find it regardless of $HOME.
		"PROTON_LOG=1",
		"PROTON_LOG_DIR=" + logDir,
		// umu-launcher itself is quiet by default; debug mode makes
		// it log where it's putting things and which runtime it
		// picked, which is what we need to diagnose "no output" runs.
		"UMU_LOG=debug",
	}
}

// protonLogPath returns the file Proton writes when PROTON_LOG=1 is
// set: $PROTON_LOG_DIR/steam-<SteamGameId>.log. We pass GAMEID=0 →
// umu-launcher rewrites it to a prefix-md5 hash, so we glob rather
// than hard-code the exact SteamGameId. Returns "" when no matching
// log exists.
func protonLogPath(logDir string) string {
	matches, _ := filepath.Glob(filepath.Join(logDir, "steam-*.log"))
	if len(matches) == 0 {
		return ""
	}
	return matches[0]
}

// protonDumpLog prints the Proton log to stdout so it lands in the
// CI job's raw log. No-op when logDir has no log (proton wasn't
// used, or PROTON_LOG never fired).
func protonDumpLog(logDir string) {
	path := protonLogPath(logDir)
	if path == "" {
		fmt.Printf("==> proton log: none found under %s\n", logDir)
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "==> proton log: read %s: %v\n", path, err)
		return
	}
	fmt.Printf("==> proton log (%s, %d bytes):\n", path, len(data))
	fmt.Println("::group::proton log")
	os.Stdout.Write(data)
	fmt.Println("::endgroup::")
}

// winePath converts a host linux path to the Z:\ form the wine child
// process sees. Proton auto-symlinks $WINEPREFIX/dosdevices/z: → /,
// so /tmp/foo becomes Z:\tmp\foo and points at the same host inode.
// Godot compiled for windows treats a bare leading '/' as a rooted
// path on the current drive (C:), which under Proton lives inside
// the prefix and doesn't map to the host — hence the GDNEXT_PLAY_*
// paths only ever work when translated here.
func winePath(hostPath string) string {
	return "Z:" + strings.ReplaceAll(hostPath, "/", `\`)
}

// needsWinePathTranslation reports whether c.Env values holding host
// paths must be rewritten to Z:\... form for the child. True when
// driving a windows binary through wine or proton on a linux host;
// false for native linux/darwin/windows-on-windows plays.
func needsWinePathTranslation(compat string, plat target) bool {
	if runtime.GOOS != product.GOOSLinux || plat.GOOS != product.GOOSWindows {
		return false
	}
	switch compat {
	case "wine", "proton", "proton-8", "proton-9", "proton-10":
		return true
	}
	return false
}

func compatOrNative(compat string) string {
	if compat == "" {
		return "native"
	}
	return compat
}

func buildHostOrLocal(buildHost string) string {
	if buildHost == "" {
		return "(local)"
	}
	return buildHost
}

// withXvfb prepends `xvfb-run -a` when running headlessly so the
// engine can open a window. No-op when DISPLAY is set or xvfb-run
// isn't installed.
func withXvfb(argv ...string) []string {
	if os.Getenv(product.EnvDisplay) != "" {
		return argv
	}
	if _, err := exec.LookPath("xvfb-run"); err != nil {
		return argv
	}
	return append([]string{"xvfb-run", "-a", "--server-args=-screen 0 1280x720x24"}, argv...)
}

// startXvfb launches a long-lived Xvfb on a free :N display and
// returns the display string + a stop function. Use this when the
// child needs DISPLAY in its env block (so detached grandchildren
// inherit it through fork), rather than xvfb-run's wrap mode which
// only sets DISPLAY for the immediate child via an env-prefix.
func startXvfb(ctx context.Context) (display string, stop func(), err error) {
	xvfb, err := exec.LookPath("Xvfb")
	if err != nil {
		return "", func() {}, fmt.Errorf("Xvfb not on PATH (apt install xvfb): %w", err)
	}
	// Pick a high display number unlikely to collide with another
	// xvfb-run on the same runner (xvfb-run -a starts at :99 and
	// climbs).
	n := 100 + (os.Getpid() % 100)
	disp := fmt.Sprintf(":%d", n)
	cmd := exec.Command(xvfb, disp, "-screen", "0", "1280x720x24", "-nolisten", "tcp")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return "", func() {}, fmt.Errorf("start Xvfb: %w", err)
	}
	// Wait briefly so the X server has time to bind before any
	// client tries to connect.
	time.Sleep(300 * time.Millisecond)
	stop = func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}
	return disp, stop, nil
}

func mustExecutable(path string) (string, error) {
	st, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("expected playable binary missing: %s (%w)", path, err)
	}
	if st.IsDir() {
		return "", fmt.Errorf("expected playable binary, got directory: %s", path)
	}
	return path, nil
}

func readReport(path string) (product.PlayReport, error) {
	var r product.PlayReport
	data, err := os.ReadFile(path)
	if err != nil {
		return r, err
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return r, err
	}
	return r, nil
}

func parseTuple(s string) (target, bool) {
	slash := strings.IndexByte(s, '/')
	if slash <= 0 || slash == len(s)-1 {
		return target{}, false
	}
	return target{GOOS: s[:slash], GOARCH: s[slash+1:]}, true
}

func contextWithTimeout(d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 {
		return context.WithCancel(context.Background())
	}
	return context.WithTimeout(context.Background(), d)
}

// hudColumn mirrors the struct the canarybird example parses out of
// $GDNEXT_PLAY_HUD; ordering is preserved so the HUD's column layout
// matches the slice order built by buildPlayHUD.
type hudColumn = product.PlayHUDColumn

// buildPlayHUD returns the JSON payload the example renders as a
// bottom-spanning, one-row provenance table. The Godot Version
// column is filled in by the running engine itself, so this slice
// covers everything the CLI/CI side knows: target tuple, link mode,
// build/play hosts, compat layer, and the workflow's GITHUB_* env.
// libc — when non-empty (linux libgodot fans out) — is appended to
// the Link Mode value as "libgodot(glibc)" / "libgodot(musl)".
func buildPlayHUD(target string, mode product.LinkMode, libc, compat, buildHost string) string {
	linkValue := mode.String()
	if libc != "" && mode.Has(product.LibGodot) {
		linkValue += "(" + libc + ")"
	}
	cols := []hudColumn{
		{Name: "Target Host", Value: target},
		{Name: "Link Mode", Value: linkValue},
		{Name: "Build Host", Value: buildHostOrLocal(buildHost)},
		{Name: "Play Host", Value: runtime.GOOS + "/" + runtime.GOARCH},
		{Name: "Compat Mode", Value: compatOrNative(compat)},
		{Name: "GH Runner", Value: strings.ToLower(os.Getenv(product.EnvRunnerOS))},
		{Name: "GH Run ID", Value: os.Getenv(product.EnvGitHubRunID)},
		{Name: "GIT Ref", Value: os.Getenv(product.EnvGitHubRefName)},
		{Name: "GIT Sha", Value: shortSha(os.Getenv(product.EnvGitHubSHA))},
	}
	data, err := json.Marshal(cols)
	if err != nil {
		return ""
	}
	return string(data)
}

func shortSha(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}
