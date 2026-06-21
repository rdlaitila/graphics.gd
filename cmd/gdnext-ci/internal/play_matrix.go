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

// PlayMatrixCmd emits a GHA strategy.matrix JSON document for the
// `gdnext-ci-run` job, one row per (play-host, build-host, target,
// link, example) where Platform.PlayHosts contains the play host.
func PlayMatrixCmd() *cli.Command {
	return &cli.Command{
		Name:  "play-matrix",
		Usage: "emit the GHA play matrix derived from product.PlayHosts",
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
			rows := buildPlayMatrix(examples)
			doc := struct {
				Include []playMatrixRow `json:"include"`
			}{Include: rows}
			out, err := json.Marshal(doc)
			if err != nil {
				return err
			}
			fmt.Println(string(out))
			if cmd.Bool("summary") {
				fmt.Fprintf(os.Stderr, "Play matrix (%d cells):\n", len(rows))
				for _, r := range rows {
					link := r.Link
					if link == "" {
						link = "-"
					}
					fmt.Fprintf(os.Stderr, "  play=%-14s build=%-14s × %-14s × %-16s × %s\n",
						r.OS, r.BuildOS, r.Example, r.Target, link)
				}
			}
			return nil
		},
	}
}

type playMatrixRow struct {
	OS           string `json:"os"`
	BuildOS      string `json:"build_os"`
	Example      string `json:"example"`
	Target       string `json:"target"`
	Link         string `json:"link,omitempty"`
	Experimental bool   `json:"experimental"`
	Artifact     string `json:"artifact"`
}

// buildPlayMatrix emits one row per (build-host, platform, mode,
// example, play-host) the build matrix would produce × the
// platform's PlayHosts entries. Artefact name mirrors the build
// job's upload key (see ArtifactName).
func buildPlayMatrix(examples []string) []playMatrixRow {
	var out []playMatrixRow
	for _, buildHost := range gha {
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
			if !platform.CanBuildOn(buildHost.Host.GOOS, buildHost.Host.GOARCH) {
				continue
			}
			if len(platform.PlayHosts) == 0 {
				continue
			}
			experimental := platform.Status.Has(product.Experimental)
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
				link := ""
				if mode != 0 {
					link = mode.String()
				}
				for _, ex := range examples {
					ex = strings.TrimSpace(ex)
					if ex == "" {
						continue
					}
					for _, playHost := range platform.PlayHosts {
						playRunner, ok := runnerFor(playHost)
						if !ok {
							continue
						}
						out = append(out, playMatrixRow{
							OS:           playRunner,
							BuildOS:      buildHost.Runner,
							Example:      ex,
							Target:       platform.Tuple(),
							Link:         link,
							Experimental: experimental,
							Artifact:     ArtifactName(buildHost.Runner, ex, platform.Tuple(), link),
						})
					}
				}
			}
		}
	}
	return out
}

// runnerFor maps a product.BuildHost to its GHA runner label.
// Returns ok=false when no runner is registered for the host.
func runnerFor(host product.BuildHost) (string, bool) {
	for _, g := range gha {
		if g.Host.GOOS == host.GOOS && g.Host.GOARCH == host.GOARCH {
			return g.Runner, true
		}
	}
	return "", false
}

// ArtifactName is the canonical upload/download name for a build
// cell's staged scratch dir. Build YAML and play matrix call this
// so the key is defined in one place.
func ArtifactName(buildRunner, example, target, link string) string {
	safeTarget := strings.ReplaceAll(target, "/", "-")
	parts := []string{"play", buildRunner, example, safeTarget}
	if link != "" {
		parts = append(parts, link)
	}
	return strings.Join(parts, "-")
}
