package ci

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"graphics.gd/cmd/gdnext/internal/shared"
	"graphics.gd/product"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
)

// gha maps each product BuildHost we run CI on to its actions/runner
// label. macos-latest is the M1 (arm64) pool.
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

// MatrixCommand wires `gdnext ci build-matrix`.
type MatrixCommand struct {
	*cli.Command
	Injector do.Injector `do:""`
}

type MatrixActions struct{}

type matrixRow struct {
	OS        string `json:"os"`
	Example   string `json:"example"`
	Target    string `json:"target"`
	Link      string `json:"link,omitempty"`
	AllowFail bool   `json:"allow_fail"`
	Playable  bool   `json:"playable"`
	Artifact  string `json:"artifact,omitempty"`
}

func NewMatrixCommand(di do.Injector) (*MatrixCommand, error) {
	t := do.MustInvokeStruct[*MatrixCommand](di)
	t.Command = &cli.Command{
		Name:  "build-matrix",
		Usage: "emit the GHA build matrix derived from product.PlatformMatrix",
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name:  "example",
				Value: []string{"canarybird"},
				Usage: "example name (repeatable; cross-products with the target list)",
			},
			&cli.StringFlag{
				Name:  "filter-host",
				Usage: "comma-separated <goos>/<goarch> host tuples; empty allows every host",
			},
			&cli.StringFlag{
				Name:  "filter-target",
				Usage: "comma-separated <goos>/<goarch> target tuples; empty allows every target",
			},
			&cli.StringFlag{
				Name:  "filter-link",
				Usage: "comma-separated link modes (gdextension|libgodot); empty allows every mode",
			},
			&cli.BoolFlag{
				Name:  "summary",
				Usage: "also print a human-readable matrix to stderr",
			},
		},
		Action: shared.BindAction(t.Injector, (*MatrixActions).action),
	}
	return t, nil
}

func NewMatrixActions(di do.Injector) (*MatrixActions, error) {
	return do.InvokeStruct[*MatrixActions](di)
}

func (t *MatrixActions) action(_ context.Context, cmd *cli.Command) error {
	examples := cmd.StringSlice("example")
	filter := parseMatrixFilter(cmd.String("filter-host"), cmd.String("filter-target"), cmd.String("filter-link"))
	rows := buildMatrix(examples, filter)
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
			if r.AllowFail {
				tag = "  (allow-fail)"
			}
			link := r.Link
			if link == "" {
				link = "-"
			}
			fmt.Fprintf(os.Stderr, "  %-14s × %-14s × %-16s × %s%s\n", r.OS, r.Example, r.Target, link, tag)
		}
	}
	return nil
}

// buildMatrix emits one include: row per (host, platform, linkMode,
// example) the workflow should run. Host axis outermost.
func buildMatrix(examples []string, filter matrixFilter) []matrixRow {
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
			if !filter.allows(host.Host.Tuple(), platform.Tuple()) {
				continue
			}
			allowFail := platform.Status.Has(product.Experimental)
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
				if !filter.allowsLink(link) {
					continue
				}
				// QuirkCIBuildBroken (e.g. windows host + darwin target,
				// or darwin host + linux libgodot) omits the cell outright;
				// the Quirk row in the summary is the contract that explains
				// why. Scoped per-mode so a quirk that only affects libgodot
				// doesn't gate the gdextension cell.
				if platform.CIBlockedFor(host.Host.GOOS, host.Host.GOARCH, mode) {
					continue
				}
				for _, ex := range examples {
					ex = strings.TrimSpace(ex)
					if ex == "" {
						continue
					}
					playable := len(platform.PlayHosts) > 0
					artifact := ""
					if playable {
						artifact = ArtifactName(host.Runner, ex, platform.Tuple(), link)
					}
					out = append(out, matrixRow{
						OS:        host.Runner,
						Example:   ex,
						Target:    platform.Tuple(),
						Link:      link,
						AllowFail: allowFail,
						Playable:  playable,
						Artifact:  artifact,
					})
				}
			}
		}
	}
	return out
}
