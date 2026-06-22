package cli

import (
	"context"
	"fmt"

	"graphics.gd/cmd/gdnext/internal/builder"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
)

// MuslCommand exposes `gdnext musl`: static-musl Linux build helpers.
type MuslCommand struct {
	*cli.Command
	Musl *builder.Musl `do:""`
}

// NewMuslCommand constructs the `gdnext musl` subcommand
func NewMuslCommand(di do.Injector) (*MuslCommand, error) {
	t := do.MustInvokeStruct[*MuslCommand](di)
	t.Command = &cli.Command{
		Name:  "musl",
		Usage: "static-musl Linux build helpers",
		Commands: []*cli.Command{
			{
				Name:   "setup",
				Usage:  "stage the musl build environment by invoking builder.Musl.Build",
				Action: t.setup,
			},
			{
				Name:   "patch-malloc",
				Usage:  "TODO: extract the deterministic malloc.c patch from builder.Musl",
				Action: t.patchMalloc,
			},
		},
	}
	return t, nil
}

func (t *MuslCommand) setup(_ context.Context, _ *cli.Command) error {
	return t.Musl.Build()
}

func (t *MuslCommand) patchMalloc(_ context.Context, _ *cli.Command) error {
	fmt.Println("`gdnext musl patch-malloc` is queued for a future builder/musl.go refactor.")
	return nil
}
