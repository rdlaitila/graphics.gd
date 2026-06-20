package cli

import (
	"context"
	"fmt"

	"graphics.gd/cmd/gdnext/internal/project"

	"github.com/urfave/cli/v3"
)

func projectCmd() *cli.Command {
	return &cli.Command{
		Name:  "project",
		Usage: "initialise and inspect a graphics.gd project",
		Commands: []*cli.Command{
			{
				Name:   "init",
				Usage:  "create the graphics/ directory, project.godot, and export presets",
				Action: projectInit,
			},
			{
				Name:   "info",
				Usage:  "print the resolved project metadata",
				Action: projectInfo,
			},
			{
				Name:   "version",
				Usage:  "print the project version (config/version in project.godot)",
				Action: projectVersion,
			},
		},
	}
}

func projectInit(_ context.Context, _ *cli.Command) error {
	return project.Setup(func() error { return nil })
}

func projectInfo(_ context.Context, _ *cli.Command) error {
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
}

func projectVersion(_ context.Context, _ *cli.Command) error {
	project.SetupVersion()
	if project.Version == "" {
		return fmt.Errorf("no project.godot found in working directory tree")
	}
	fmt.Println(project.Version)
	return nil
}
