package cli

import (
	"context"
	"fmt"
	"runtime/debug"

	"graphics.gd/cmd/gdnext/internal/tooling"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
	"graphics.gd/cmd/gdnext/internal/shared"
)

// VersionCommand wires `gdnext version`.
type VersionCommand struct {
	*cli.Command
	Injector do.Injector `do:""`
}

type VersionActions struct {
	ToolCatalog tooling.Catalog `do:""`
}

func NewVersionCommand(di do.Injector) (*VersionCommand, error) {
	t := do.MustInvokeStruct[*VersionCommand](di)
	t.Command = &cli.Command{
		Name:   "version",
		Usage:  "print gdnext, go, and godot versions",
		Action: shared.BindAction(t.Injector, (*VersionActions).versionAction),
	}
	return t, nil
}

func NewVersionActions(di do.Injector) (*VersionActions, error) {
	return do.InvokeStruct[*VersionActions](di)
}

func (t *VersionActions) versionAction(_ context.Context, _ *cli.Command) error {
	fmt.Println("gdnext version", version())
	if err := t.ToolCatalog.Go.Exec("version"); err != nil {
		return err
	}
	fmt.Println("godot expected version", t.ToolCatalog.Godot.Version)
	if path, err := t.ToolCatalog.Godot.Lookup(tooling.ModeFind); err == nil {
		if out, err := t.ToolCatalog.Godot.Output(t.ToolCatalog.Godot.VersionFlags...); err == nil {
			fmt.Println("godot installed", out, "at", path)
		}
	}
	return nil
}

// version returns the gdnext binary's own module version, falling back
// to "(devel)" when invoked from a non-vendored build. Kept package-
// scope so RootCommand can read it without resolving VersionCommand.
func version() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "(devel)"
}
