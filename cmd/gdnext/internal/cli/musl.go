package cli

import (
	"context"
	"fmt"

	"graphics.gd/cmd/gdnext/internal/builder"

	"github.com/urfave/cli/v3"
)

func muslCmd() *cli.Command {
	return &cli.Command{
		Name:  "musl",
		Usage: "static-musl Linux build helpers",
		Commands: []*cli.Command{
			{
				Name:   "setup",
				Usage:  "stage the musl build environment by invoking builder.Musl.Build",
				Action: muslSetup,
			},
			{
				Name:   "patch-malloc",
				Usage:  "TODO: extract the deterministic malloc.c patch from builder.Musl",
				Action: muslPatchMalloc,
			},
		},
	}
}

func muslSetup(_ context.Context, _ *cli.Command) error {
	return builder.Musl{}.Build(buildEnv)
}

func muslPatchMalloc(_ context.Context, _ *cli.Command) error {
	fmt.Println("`gdnext musl patch-malloc` is queued for a future builder/musl.go refactor.")
	return nil
}
