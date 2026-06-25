package ci

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"graphics.gd/cmd/gdnext/internal/shared"
	"graphics.gd/product"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
)

// PlayMatrixCommand wires `gdnext ci play-matrix`. Runtime state
// lives on *PlayMatrixActions. Emits one row per (play-host,
// build-host, target, link, example) where Platform.PlayHosts
// contains the play host.
type PlayMatrixCommand struct {
	*cli.Command
	Injector do.Injector `do:""`
}

// PlayMatrixActions carries the runtime state.
type PlayMatrixActions struct{}

type playMatrixRow struct {
	OS        string `json:"os"`
	BuildOS   string `json:"build_os"`
	Example   string `json:"example"`
	Target    string `json:"target"`
	Link      string `json:"link,omitempty"`
	Compat    string `json:"compat,omitempty"`
	ProtonTag string `json:"proton_tag,omitempty"`
	AllowFail bool   `json:"allow_fail"`
	Artifact  string `json:"artifact"`
}

// NewPlayMatrixCommand constructs the play-matrix subcommand.
func NewPlayMatrixCommand(di do.Injector) (*PlayMatrixCommand, error) {
	t := do.MustInvokeStruct[*PlayMatrixCommand](di)
	t.Command = &cli.Command{
		Name:  "play-matrix",
		Usage: "emit the GHA play matrix derived from product.PlayHosts",
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name:  "example",
				Value: []string{"canarybird"},
				Usage: "example name (repeatable; cross-products with the target list)",
			},
			&cli.StringFlag{
				Name:  "filter-host",
				Usage: "comma-separated <goos>/<goarch> build-host tuples; empty allows every host",
			},
			&cli.StringFlag{
				Name:  "filter-target",
				Usage: "comma-separated <goos>/<goarch> target tuples; empty allows every target",
			},
			&cli.BoolFlag{
				Name:  "summary",
				Usage: "also print a human-readable matrix to stderr",
			},
		},
		Action: shared.BindAction(t.Injector, (*PlayMatrixActions).action),
	}
	return t, nil
}

// NewPlayMatrixActions resolves the runtime state.
func NewPlayMatrixActions(di do.Injector) (*PlayMatrixActions, error) {
	return do.InvokeStruct[*PlayMatrixActions](di)
}

func (t *PlayMatrixActions) action(_ context.Context, cmd *cli.Command) error {
	examples := cmd.StringSlice("example")
	filter := parseMatrixFilter(cmd.String("filter-host"), cmd.String("filter-target"))
	rows := buildPlayMatrix(examples, filter)
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
			compat := r.Compat
			if compat == "" {
				compat = "native"
			}
			fmt.Fprintf(os.Stderr, "  %-16s [%s] compat %-10s built on %-14s played on %-14s × %s\n",
				r.Target, link, compat, r.BuildOS, r.OS, r.Example)
		}
	}
	return nil
}

// buildPlayMatrix emits one row per (build-host, platform, mode,
// example, play-host) the build matrix would produce × the
// platform's PlayHosts entries. Artefact name mirrors the build
// job's upload key (see ArtifactName).
func buildPlayMatrix(examples []string, filter matrixFilter) []playMatrixRow {
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
			if !filter.allows(buildHost.Host.Tuple(), platform.Tuple()) {
				continue
			}
			if len(platform.PlayHosts) == 0 {
				continue
			}
			allowFail := platform.Status.Has(product.Experimental) ||
				platform.CIBlockedFor(buildHost.Host.GOOS, buildHost.Host.GOARCH)
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
						if platform.PlayBlockedFor(playHost.GOOS, playHost.GOARCH, playHost.CompatLayer) {
							continue
						}
						out = append(out, playMatrixRow{
							OS:        playRunner,
							BuildOS:   buildHost.Runner,
							Example:   ex,
							Target:    platform.Tuple(),
							Link:      link,
							Compat:    playHost.CompatLayer,
							ProtonTag: protonRelease(playHost.CompatLayer),
							AllowFail: allowFail,
							Artifact:  ArtifactName(buildHost.Runner, ex, platform.Tuple(), link),
						})
					}
				}
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return playMatrixLess(out[i], out[j]) })
	return out
}

// playMatrixLess orders rows by target → link → build host → play
// host → compat layer → example so reading the human summary scans
// target-major.
func playMatrixLess(a, b playMatrixRow) bool {
	if ar, br := targetRank(a.Target), targetRank(b.Target); ar != br {
		return ar < br
	}
	if ar, br := linkSubrank(a.Link), linkSubrank(b.Link); ar != br {
		return ar < br
	}
	if ar, br := hostRank(a.BuildOS), hostRank(b.BuildOS); ar != br {
		return ar < br
	}
	if ar, br := hostRank(a.OS), hostRank(b.OS); ar != br {
		return ar < br
	}
	if a.Compat != b.Compat {
		return a.Compat < b.Compat
	}
	return a.Example < b.Example
}

// runnerFor maps a product.PlayHost to its GHA runner label.
// Returns ok=false when no runner is registered for the host.
func runnerFor(host product.PlayHost) (string, bool) {
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
