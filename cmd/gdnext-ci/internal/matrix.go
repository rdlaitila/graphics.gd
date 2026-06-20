package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"graphics.gd/product"

	"github.com/urfave/cli/v3"
)

// gha is the GHA host-OS table: maps each canonical product BuildHost we
// run CI on to the matching actions/runner-images label. Order is
// preserved in the emitted matrix so the GHA UI lays jobs out
// predictably. The (GOOS, GOARCH) pair is the real shape of the runner —
// ubuntu-latest and windows-latest are amd64; macos-latest is arm64 (M1
// pool since 2024). Keep this in sync with whichever runner labels the
// workflow `runs-on` is willing to schedule.
var gha = []struct {
	Host   product.BuildHost
	Runner string
}{
	{product.HostLinuxAmd64, "ubuntu-latest"},
	{product.HostWindowsAmd64, "windows-latest"},
	{product.HostDarwinArm64, "macos-latest"},
}

// excludedTargets are CI-policy exclusions: rows that exist in
// product.PlatformMatrix but can't be exercised meaningfully in CI.
// Keyed by "goos/goarch".
var excludedTargets = map[string]string{}

// MatrixCmd emits a GHA strategy.matrix JSON document with one
// include: entry per (host, target) pair that the workflow should
// actually run. The build matrix consumes it via fromJSON, so adding
// a row to product.PlatformMatrix lands in CI with zero workflow
// edits.
func MatrixCmd() *cli.Command {
	return &cli.Command{
		Name:  "matrix",
		Usage: "emit the GHA build matrix derived from product.PlatformMatrix",
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name:  "example",
				Value: []string{"canarybird"},
				Usage: "example name (repeatable; cross-products with the target list)",
			},
			&cli.BoolFlag{
				Name:  "summary",
				Usage: "also print a human-readable matrix to stderr",
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			examples := cmd.StringSlice("example")
			rows := buildMatrix(examples)
			doc := struct {
				Include []matrixRow `json:"include"`
			}{Include: rows}
			out, err := json.Marshal(doc)
			if err != nil {
				return err
			}
			fmt.Println(string(out))
			if cmd.Bool("summary") {
				fmt.Fprintf(os.Stderr, "Build matrix (%d cells):\n", len(rows))
				for _, r := range rows {
					tag := ""
					if r.Experimental {
						tag = "  (experimental)"
					}
					link := r.Link
					if link == "" {
						link = "-"
					}
					fmt.Fprintf(os.Stderr, "  %-14s × %-14s × %-16s × %s%s\n", r.OS, r.Example, r.Target, link, tag)
				}
			}
			return nil
		},
	}
}

type matrixRow struct {
	OS           string `json:"os"`
	Example      string `json:"example"`
	Target       string `json:"target"`
	Link         string `json:"link,omitempty"`
	Experimental bool   `json:"experimental"`
}

// buildMatrix runs the same selection rules `gdnext-ci matrix` describes
// and returns the resulting include: entries in deterministic order
// (host axis outer, then matrix axis, then example axis). Each
// (platform, linkMode) combination becomes its own row so the CI surface
// stays correctly tagged when a target supports multiple link recipes.
func buildMatrix(examples []string) []matrixRow {
	var out []matrixRow
	for _, host := range gha {
		for _, platform := range product.PlatformMatrix {
			if !platform.Kind.Has(product.Target) {
				continue
			}
			if !platform.Status.Has(product.Supported) && !platform.Status.Has(product.Experimental) {
				continue
			}
			if platform.Status.Has(product.Broken) {
				continue
			}
			if _, excluded := excludedTargets[platform.Tuple()]; excluded {
				continue
			}
			if !platform.CanBuildOn(host.Host.GOOS, host.Host.GOARCH) {
				continue
			}
			experimental := platform.Status.Has(product.Experimental)
			// One row per supported LinkMode. If LinkModes is empty
			// (legacy / not yet annotated) fall back to a single row
			// without the link axis so the CI keeps emitting it.
			modes := []product.LinkMode{0}
			if platform.LinkModes != 0 {
				modes = modes[:0]
				if platform.LinkModes.Has(product.GDExtension) {
					modes = append(modes, product.GDExtension)
				}
				if platform.LinkModes.Has(product.LibGodot) {
					modes = append(modes, product.LibGodot)
				}
			}
			for _, mode := range modes {
				// LibGodot is experimental everywhere today even
				// when the host platform isn't.
				rowExp := experimental || mode == product.LibGodot
				link := ""
				if mode != 0 {
					link = mode.String()
				}
				for _, ex := range examples {
					ex = strings.TrimSpace(ex)
					if ex == "" {
						continue
					}
					out = append(out, matrixRow{
						OS:           host.Runner,
						Example:      ex,
						Target:       platform.Tuple(),
						Link:         link,
						Experimental: rowExp,
					})
				}
			}
		}
	}
	return out
}
