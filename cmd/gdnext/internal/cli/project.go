package cli

import (
	"context"
	"fmt"

	"graphics.gd/cmd/gdnext/internal/project"
	"graphics.gd/cmd/gdnext/internal/tooling"

	"github.com/samber/do/v2"
	"graphics.gd/cmd/gdnext/internal/shared"
	"github.com/urfave/cli/v3"
)

// ProjectCommand wires `gdnext project`. Runtime state lives on
// *ProjectActions.
type ProjectCommand struct {
	*cli.Command
	Injector do.Injector `do:""`
}

// ProjectActions carries the runtime state.
type ProjectActions struct {
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
				Action: shared.BindAction(t.Injector, (*ProjectActions).init),
			},
			{
				Name:   "info",
				Usage:  "print the resolved project metadata",
				Action: shared.BindAction(t.Injector, (*ProjectActions).info),
			},
			{
				Name:   "version",
				Usage:  "print the project version (config/version in project.godot)",
				Action: shared.BindAction(t.Injector, (*ProjectActions).version),
			},
		},
	}
	return t, nil
}

// NewProjectActions resolves the runtime state for project.
func NewProjectActions(di do.Injector) (*ProjectActions, error) {
	return do.InvokeStruct[*ProjectActions](di)
}

func (t *ProjectActions) init(_ context.Context, _ *cli.Command) error {
	return project.Setup(t.ToolCatalog, func() error { return nil })
}

func (t *ProjectActions) info(_ context.Context, _ *cli.Command) error {
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

func (t *ProjectActions) version(_ context.Context, _ *cli.Command) error {
	project.SetupVersion()
	if project.Version == "" {
		return fmt.Errorf("no project.godot found in working directory tree")
	}
	fmt.Println(project.Version)
	return nil
}
