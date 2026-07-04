package cli

import (
	"context"
	"fmt"
	"os"

	"graphics.gd/cmd/gdnext/internal/shared"

	lipo "github.com/konoui/lipo/cmd"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
)

// MacosCommand wires `gdnext macos`.
type MacosCommand struct {
	*cli.Command
	Injector do.Injector `do:""`
}

type MacosActions struct{}

func NewMacosCommand(di do.Injector) (*MacosCommand, error) {
	t := do.MustInvokeStruct[*MacosCommand](di)
	t.Command = &cli.Command{
		Name:  "macos",
		Usage: "macOS-specific helpers (lipo, codesign)",
		Commands: []*cli.Command{
			{
				Name:            "lipo",
				Usage:           "merge per-arch dylibs into a universal one",
				ArgsUsage:       "-output <out.dylib> -create <arch1.dylib> <arch2.dylib>",
				SkipFlagParsing: true,
				Action:          shared.BindAction(t.Injector, (*MacosActions).lipo),
			},
			{
				Name:   "codesign",
				Usage:  "TODO: extract codesign --deep from builder.MacOS",
				Action: shared.BindAction(t.Injector, (*MacosActions).codesign),
			},
		},
	}
	return t, nil
}

func NewMacosActions(di do.Injector) (*MacosActions, error) {
	return do.InvokeStruct[*MacosActions](di)
}

func (t *MacosActions) lipo(_ context.Context, cmd *cli.Command) error {
	rc := lipo.Execute(os.Stdout, os.Stderr, cmd.Args().Slice())
	if rc != 0 {
		return fmt.Errorf("lipo exited with status %d", rc)
	}
	return nil
}

func (t *MacosActions) codesign(_ context.Context, _ *cli.Command) error {
	fmt.Println("`gdnext macos codesign` is queued for a future builder/macos.go refactor.")
	fmt.Println("Today, run `GOOS=macos gdnext build` to drive codesigning via the existing pipeline.")
	return nil
}
