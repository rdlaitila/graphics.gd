package cli

import (
	"context"
	"fmt"

	"graphics.gd/cmd/gdnext/internal/project"
	"graphics.gd/cmd/gdnext/internal/tooling"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
)

// ProjectCommand exposes `gdnext project`: initialise and inspect a
// graphics.gd project.
type ProjectCommand struct {
	*cli.Command
	ToolCatalog tooling.Catalog `do:""`
}

// NewProjectCommand constructs the `gdnext project` subcommand
func NewProjectCommand(di do.Injector) (*ProjectCommand, error) {
	t := do.MustInvokeStruct[*ProjectCommand](di)
	t.Command = &cli.Command{
		Name:  "project",
		Usage: "initialise and inspect a graphics.gd project",
		Commands: []*cli.Command{
			{
				Name:   "init",
				Usage:  "create the graphics/ directory, project.godot, and export presets",
				Action: t.init,
			},
			{
				Name:   "info",
				Usage:  "print the resolved project metadata",
				Action: t.info,
			},
			{
				Name:   "version",
				Usage:  "print the project version (config/version in project.godot)",
				Action: t.version,
			},
		},
	}
	return t, nil
}

func (t *ProjectCommand) init(_ context.Context, _ *cli.Command) error {
	return project.Setup(t.ToolCatalog, func() error { return nil })
}

func (t *ProjectCommand) info(_ context.Context, _ *cli.Command) error {
	if err := project.Setup(t.ToolCatalog, func() error { return nil }); err != nil {
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

func (t *ProjectCommand) version(_ context.Context, _ *cli.Command) error {
	project.SetupVersion()
	if project.Version == "" {
		return fmt.Errorf("no project.godot found in working directory tree")
	}
	fmt.Println(project.Version)
	return nil
}
