package cli

import (
	"context"
	"fmt"
	"os"

	"graphics.gd/cmd/gdnext/internal/builder"
	"graphics.gd/product"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
	"graphics.gd/cmd/gdnext/internal/shared"
)

// MuslCommand wires `gdnext musl`. Kept as a thin shim over the Linux
// builder with GDNEXT_LIBGODOT_LIBC=musl forced, since musl-static
// libgodot linking is now folded into the Linux builder. Prefer
// `gdnext libgodot build --libc=musl` for the modern path.
type MuslCommand struct {
	*cli.Command
	Injector do.Injector `do:""`
}

// MuslActions carries the runtime state.
type MuslActions struct {
	Linux *builder.Linux `do:""`
}

// NewMuslCommand constructs the `gdnext musl` subcommand
func NewMuslCommand(di do.Injector) (*MuslCommand, error) {
	t := do.MustInvokeStruct[*MuslCommand](di)
	t.Command = &cli.Command{
		Name:  "musl",
		Usage: "static-musl Linux build helpers (thin shim over `gdnext libgodot build --libc=musl`)",
		Commands: []*cli.Command{
			{
				Name:   "setup",
				Usage:  "stage the musl build environment by invoking the Linux builder with GDNEXT_LIBGODOT_LIBC=musl",
				Action: shared.BindAction(t.Injector, (*MuslActions).setup),
			},
			{
				Name:   "patch-malloc",
				Usage:  "TODO: extract the deterministic malloc.c patch from builder/linux.go",
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
	prev := os.Getenv(product.EnvLibGodotLibC)
	os.Setenv(product.EnvLibGodotLibC, product.LibCMusl)
	defer os.Setenv(product.EnvLibGodotLibC, prev)
	return t.Linux.Build()
}

func (t *MuslActions) patchMalloc(_ context.Context, _ *cli.Command) error {
	fmt.Println("`gdnext musl patch-malloc` is queued for a future refactor.")
	return nil
}
