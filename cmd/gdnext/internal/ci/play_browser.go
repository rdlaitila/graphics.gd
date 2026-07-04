package ci

import (
	"context"
	_ "embed"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"graphics.gd/cmd/gdnext/internal/shared"
	"graphics.gd/product"
)

//go:embed play_browser/play.mjs
var playBrowserScript []byte

// browserPlayOpts is the small struct the play-cell dispatcher hands
// to runBrowserPlay when the target is js/wasm. Mirrors the same
// (reportPath, screenshotPath, hud, timeout) contract the native exec
// path produces, so the downstream report-reading logic is unchanged.
type browserPlayOpts struct {
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

// runBrowserPlay stands up an in-process HTTP server over the
// scratch/releases/js/wasm/ bundle (with the COEP/COOP headers the
// wasm runtime needs), then drives a headless browser via the
// embedded Playwright script. The script captures a tagged
// `GD_PLAY_RESULT:<base64>` console line out of the page,
// decodes it to opts.reportPath, and writes a page.screenshot() to
// opts.screenshotPath. From the caller's perspective the contract
// is identical to the native exec path.
func runBrowserPlay(opts browserPlayOpts) error {
	root := filepath.Join(opts.scratch, "releases", "js", "wasm")
	if _, err := os.Stat(filepath.Join(root, "index.html")); err != nil {
		return fmt.Errorf("browser play: %w", err)
	}
	if opts.compat != "chrome" && opts.compat != "firefox" {
		return fmt.Errorf("browser play: --compat must be chrome|firefox, got %q", opts.compat)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		return fmt.Errorf("browser play: node not on PATH (apt install nodejs / brew install node): %w", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("browser play: listen: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	fs := http.FileServer(http.Dir(root))
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		fs.ServeHTTP(w, r)
	})}
	go func() { _ = srv.Serve(listener) }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	hudB64 := base64.RawURLEncoding.EncodeToString([]byte(opts.hud))
	url := fmt.Sprintf("http://127.0.0.1:%d/index.html?gd_play=active&gd_play_hud=%s", port, hudB64)
	scriptPath := filepath.Join(opts.scratch, ".play_browser.mjs")
	if err := os.WriteFile(scriptPath, playBrowserScript, 0644); err != nil {
		return fmt.Errorf("browser play: stage script: %w", err)
	}
	defer os.Remove(scriptPath)
	fmt.Printf("==> play %s [%s] compat=%s build-host=%s via headless %s at %s\n",
		opts.target, opts.link, opts.compat, buildHostOrLocal(opts.buildHost), opts.compat, url)
	runCtx, cancel := contextWithTimeout(opts.timeout)
	defer cancel()
	nodeArgs := []string{scriptPath,
		"--url", url,
		"--browser", opts.compat,
		"--report", opts.reportPath,
		"--screenshot", opts.screenshotPath,
		"--timeout", strconv.Itoa(int(opts.timeout / time.Millisecond)),
	}
	if os.Getenv(product.EnvPlayHeaded) != "" {
		nodeArgs = append(nodeArgs, "--headed")
	}
	shared.Announce("", nil, node, nodeArgs)
	c := exec.CommandContext(runCtx, node, nodeArgs...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}
