package cli

import (
	"context"
	"fmt"
	"os"

	lipo "github.com/konoui/lipo/cmd"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
)

// MacosCommand exposes `gdnext macos`: macOS-specific helpers (lipo,
// codesign).
type MacosCommand struct {
	*cli.Command
}

// NewMacosCommand constructs the `gdnext macos` subcommand
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
				Action:          t.lipo,
			},
			{
				Name:   "codesign",
				Usage:  "TODO: extract codesign --deep from builder.MacOS",
				Action: t.codesign,
			},
		},
	}
	return t, nil
}

func (t *MacosCommand) lipo(_ context.Context, cmd *cli.Command) error {
	rc := lipo.Execute(os.Stdout, os.Stderr, cmd.Args().Slice())
	if rc != 0 {
		return fmt.Errorf("lipo exited with status %d", rc)
	}
	return nil
}

func (t *MacosCommand) codesign(_ context.Context, _ *cli.Command) error {
	fmt.Println("`gdnext macos codesign` is queued for a future builder/macos.go refactor.")
	fmt.Println("Today, run `GOOS=macos gdnext build` to drive codesigning via the existing pipeline.")
	return nil
}
