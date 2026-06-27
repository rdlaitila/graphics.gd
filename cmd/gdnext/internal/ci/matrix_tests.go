package ci

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"graphics.gd/cmd/gdnext/internal/shared"
	"graphics.gd/product"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
)

// TestMatrixCommand wires `gdnext ci test-matrix`. Emits one
// (host runner, link mode) row per cell `gdnext-test` should run,
// honouring each host platform's LinkModes (today only linux/amd64
// supports both gdextension + libgodot; the others omit libgodot
// rather than hard-fail on a missing toolchain).
type TestMatrixCommand struct {
	*cli.Command
	Injector do.Injector `do:""`
}

type TestMatrixActions struct{}

type testMatrixRow struct {
	OS   string `json:"os"`
	Link string `json:"link"`
}

func NewTestMatrixCommand(di do.Injector) (*TestMatrixCommand, error) {
	t := do.MustInvokeStruct[*TestMatrixCommand](di)
	t.Command = &cli.Command{
		Name:  "test-matrix",
		Usage: "emit the GHA test matrix derived from each host platform's LinkModes",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "filter-host",
				Usage: "comma-separated <goos>/<goarch> host tuples; empty allows every host",
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
		Action: shared.BindAction(t.Injector, (*TestMatrixActions).action),
	}
	return t, nil
}

func NewTestMatrixActions(di do.Injector) (*TestMatrixActions, error) {
	return do.InvokeStruct[*TestMatrixActions](di)
}

func (t *TestMatrixActions) action(_ context.Context, cmd *cli.Command) error {
	filter := parseMatrixFilter(cmd.String("filter-host"), "", cmd.String("filter-link"))
	rows := testMatrix(filter)
	doc := struct {
		Include []testMatrixRow `json:"include"`
	}{Include: rows}
	out, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	if cmd.Bool("summary") {
		fmt.Fprintf(os.Stderr, "Test matrix (%d cells):\n", len(rows))
		for _, r := range rows {
			fmt.Fprintf(os.Stderr, "  %-14s × %s\n", r.OS, r.Link)
		}
	}
	return nil
}

// testMatrix walks gha and emits one row per (runner, link mode)
// the host's own platform supports. `gdnext test` always targets
// the runner's GOOS/GOARCH (cross-test is refused upstream), so the
// only axis besides host is link. A host's own platform entry in
// product.PlatformMatrix gates which link modes are valid: today
// only linux/amd64 has both, the rest are gdextension-only.
func testMatrix(filter matrixFilter) []testMatrixRow {
	var out []testMatrixRow
	for _, host := range gha {
		if !filter.allows(host.Host.Tuple(), "") {
			continue
		}
		plat, ok := product.FindPlatformByTargetEnv(host.Host.GOOS, host.Host.GOARCH)
		if !ok {
			continue
		}
		modes := []product.LinkMode{}
		if plat.LinkModes == 0 || plat.LinkModes.Has(product.GDExtension) {
			modes = append(modes, product.GDExtension)
		}
		if plat.LinkModes.Has(product.LibGodot) {
			modes = append(modes, product.LibGodot)
		}
		for _, m := range modes {
			link := m.String()
			if !filter.allowsLink(link) {
				continue
			}
			out = append(out, testMatrixRow{OS: host.Runner, Link: link})
		}
	}
	return out
}
