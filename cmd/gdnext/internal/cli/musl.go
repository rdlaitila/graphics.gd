package cli

import (
	"context"
	"fmt"

	"graphics.gd/cmd/gdnext/internal/builder"

	"github.com/samber/do/v2"
	"graphics.gd/cmd/gdnext/internal/shared"
	"github.com/urfave/cli/v3"
)

// MuslCommand wires `gdnext musl`. Runtime state lives on *MuslActions.
type MuslCommand struct {
	*cli.Command
	Injector do.Injector `do:""`
}

// MuslActions carries the runtime state (the *builder.Musl is resolved
// post-Before so we get a fresh BuildEnv).
type MuslActions struct {
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
				Action: shared.BindAction(t.Injector, (*MuslActions).setup),
			},
			{
				Name:   "patch-malloc",
				Usage:  "TODO: extract the deterministic malloc.c patch from builder.Musl",
				Action: shared.BindAction(t.Injector, (*MuslActions).patchMalloc),
			},
		},
	}
	return t, nil
}

// NewMuslActions resolves the runtime state for musl.
func NewMuslActions(di do.Injector) (*MuslActions, error) {
	return do.InvokeStruct[*MuslActions](di)
}

func (t *MuslActions) setup(_ context.Context, _ *cli.Command) error {
	return t.Musl.Build()
}

func (t *MuslActions) patchMalloc(_ context.Context, _ *cli.Command) error {
	fmt.Println("`gdnext musl patch-malloc` is queued for a future builder/musl.go refactor.")
	return nil
}
