package internal

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

	"graphics.gd/product"

	"github.com/urfave/cli/v3"
)

// PlayCellCmd launches the produced binary headlessly with the
// canarybird play-bot enabled and asserts the resulting report.
func PlayCellCmd() *cli.Command {
	return &cli.Command{
		Name:  "play-cell",
		Usage: "drive one already-built example headlessly via the play-bot",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "scratch", Required: true, Usage: "directory containing the built example (releases/<goos>/<goarch>/...)"},
			&cli.StringFlag{Name: "example", Required: true, Usage: "example name (binary basename inside releases/<goos>/<goarch>/)"},
			&cli.StringFlag{Name: "target", Required: true, Usage: "target goos/goarch (e.g. linux/amd64, windows/amd64)"},
			&cli.StringFlag{Name: "link", Usage: "link mode (gdextension|libgodot); blank = platform default"},
			&cli.DurationFlag{Name: "timeout", Value: 90 * time.Second, Usage: "hard kill after this much wall-clock time"},
			&cli.IntFlag{Name: "min-score", Value: 1, Usage: "minimum score for a passing report"},
		},
		Action: playCellAction,
	}
}

type playReport struct {
	Score   int     `json:"score"`
	Flaps   int     `json:"flaps"`
	Elapsed float64 `json:"elapsed"`
	Crashed bool    `json:"crashed"`
}

func playCellAction(_ context.Context, cmd *cli.Command) error {
	scratch, err := filepath.Abs(cmd.String("scratch"))
	if err != nil {
		return err
	}
	example := cmd.String("example")
	target := cmd.String("target")
	link := cmd.String("link")
	timeout := cmd.Duration("timeout")
	minScore := int(cmd.Int("min-score"))

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
	bin, err := releaseBinary(scratch, example, plat, mode)
	if err != nil {
		return err
	}
	reportPath := filepath.Join(scratch, "play-report.json")
	_ = os.Remove(reportPath)
	argv, err := launchCommand(bin, plat, mode)
	if err != nil {
		return err
	}
	fmt.Printf("==> play %s [%s]: %s\n", target, mode, strings.Join(argv, " "))

	ctx, cancel := contextWithTimeout(timeout)
	defer cancel()
	c := exec.CommandContext(ctx, argv[0], argv[1:]...)
	c.Env = append(os.Environ(),
		"GDNEXT_PLAY=1",
		"GDNEXT_PLAY_REPORT="+reportPath,
	)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	runErr := c.Run()
	if runErr != nil {
		// engine exit is best-effort: bot quits via SceneTree which exits 0 even on player crash; the report is the source of truth.
		fmt.Fprintf(os.Stderr, "engine exit: %v\n", runErr)
	}
	report, err := readReport(reportPath)
	if err != nil {
		return fmt.Errorf("play report unreadable at %s: %w (engine exit: %v)", reportPath, err, runErr)
	}
	fmt.Printf("==> report: score=%d flaps=%d elapsed=%.2fs crashed=%v\n",
		report.Score, report.Flaps, report.Elapsed, report.Crashed)
	if report.Score < minScore {
		return fmt.Errorf("play failed: score %d < min %d", report.Score, minScore)
	}
	if report.Elapsed < 1.0 {
		return fmt.Errorf("play failed: elapsed %.2fs < 1s (engine likely exited before bot ticked)", report.Elapsed)
	}
	if !report.Crashed {
		return fmt.Errorf("play failed: bot exited without crashing (gameOver path not exercised)")
	}
	return nil
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
	default:
		return "", fmt.Errorf("no play recipe for %s/%s (link=%s)", plat.GOOS, plat.GOARCH, mode)
	}
}

// launchCommand wraps the produced binary in whatever runtime the
// play host needs to drive a (potentially foreign) target.
func launchCommand(bin string, plat target, mode product.LinkMode) ([]string, error) {
	hostGOOS := runtime.GOOS
	switch {
	case hostGOOS == "linux" && plat.GOOS == product.GOOSLinux:
		return withXvfb(bin), nil
	case hostGOOS == "linux" && plat.GOOS == product.GOOSWindows:
		wine, err := exec.LookPath("wine")
		if err != nil {
			return nil, fmt.Errorf("linux→windows play needs wine on PATH: %w", err)
		}
		return withXvfb(wine, bin), nil
	case hostGOOS == "windows" && plat.GOOS == product.GOOSWindows:
		return []string{bin}, nil
	case hostGOOS == "darwin" && plat.GOOS == product.GOOSDarwin:
		return []string{bin}, nil
	}
	return nil, fmt.Errorf("no launch recipe: host=%s target=%s/%s", hostGOOS, plat.GOOS, plat.GOARCH)
}

// withXvfb prepends `xvfb-run -a` when running headlessly so the
// engine can open a window. No-op when DISPLAY is set or xvfb-run
// isn't installed.
func withXvfb(argv ...string) []string {
	if os.Getenv("DISPLAY") != "" {
		return argv
	}
	if _, err := exec.LookPath("xvfb-run"); err != nil {
		return argv
	}
	return append([]string{"xvfb-run", "-a", "--server-args=-screen 0 1280x720x24"}, argv...)
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

func readReport(path string) (playReport, error) {
	var r playReport
	data, err := os.ReadFile(path)
	if err != nil {
		return r, err
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return r, err
	}
	return r, nil
}

type target struct{ GOOS, GOARCH string }

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
