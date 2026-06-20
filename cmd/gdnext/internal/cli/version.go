package cli

import (
	"context"
	"fmt"

	"graphics.gd/cmd/gdnext/internal/tooling"

	"github.com/urfave/cli/v3"
)

func versionCmd() *cli.Command {
	return &cli.Command{
		Name:  "version",
		Usage: "print gdnext, go, and godot versions",
		Action: func(_ context.Context, _ *cli.Command) error {
			fmt.Println("gdnext version", Version())
			if err := tooling.Go.Exec("version"); err != nil {
				return err
			}
			fmt.Println("godot expected version", tooling.Godot.Version)
			if path, err := tooling.Godot.Lookup(tooling.ModeFind); err == nil {
				if out, err := tooling.Godot.Output(tooling.Godot.VersionFlags...); err == nil {
					fmt.Println("godot installed", out, "at", path)
				}
			}
			return nil
		},
	}
}
