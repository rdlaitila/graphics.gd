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

// gha is the GHA host-OS table: maps each product GOOS we run CI on to
// the matching actions/runner-images label. Order is preserved in the
// emitted matrix so the GHA UI lays jobs out predictably.
var gha = []struct {
	GOOS, Runner string
}{
	{"linux", "ubuntu-latest"},
	{"windows", "windows-latest"},
	{"darwin", "macos-latest"},
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
					fmt.Fprintf(os.Stderr, "  %-14s × %-14s × %s%s\n", r.OS, r.Example, r.Target, tag)
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
	Experimental bool   `json:"experimental"`
}

// buildMatrix runs the same selection rules `gdnext-ci matrix` describes
// and returns the resulting include: entries in deterministic order
// (host axis outer, then matrix axis, then example axis).
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
			if !platform.CanBuildOn(host.GOOS, "") {
				continue
			}
			experimental := platform.Status.Has(product.Experimental)
			for _, ex := range examples {
				ex = strings.TrimSpace(ex)
				if ex == "" {
					continue
				}
				out = append(out, matrixRow{
					OS:           host.Runner,
					Example:      ex,
					Target:       platform.Tuple(),
					Experimental: experimental,
				})
			}
		}
	}
	return out
}
