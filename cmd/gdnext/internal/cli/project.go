package cli

import (
	"context"
	"fmt"
	"os"

	"graphics.gd/cmd/gdnext/internal/project"

	"github.com/urfave/cli/v3"
)

func projectCmd() *cli.Command {
	return &cli.Command{
		Name:  "project",
		Usage: "initialise and inspect a graphics.gd project",
		Commands: []*cli.Command{
			{
				Name:  "init",
				Usage: "create the graphics/ directory, project.godot, and export presets",
				Action: func(_ context.Context, _ *cli.Command) error {
					return project.Setup(func() error { return nil })
				},
			},
			{
				Name:  "info",
				Usage: "print the resolved project metadata",
				Action: func(_ context.Context, _ *cli.Command) error {
					if err := project.Setup(func() error { return nil }); err != nil {
						return err
					}
					fmt.Println("name:    ", project.Name)
					fmt.Println("module:  ", project.Directory)
					fmt.Println("graphics:", project.GraphicsDirectory)
					fmt.Println("releases:", project.ReleasesDirectory)
					fmt.Println("version: ", project.Version)
					fmt.Println("go:      ", project.IncludesGo)
					return nil
				},
			},
			{
				Name:  "version",
				Usage: "print the project version (config/version in project.godot)",
				Action: func(_ context.Context, _ *cli.Command) error {
					project.SetupVersion()
					if project.Version == "" {
						fmt.Fprintln(os.Stderr, "(no project.godot found in working directory tree)")
						return nil
					}
					fmt.Println(project.Version)
					return nil
				},
			},
		},
	}
}
