package ci

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"graphics.gd/cmd/gdnext/internal/shared"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
)

// StageExampleCommand wires `gdnext ci build-stage-example`. Runtime state
// lives on *StageExampleActions.
type StageExampleCommand struct {
	*cli.Command
	Injector do.Injector `do:""`
}

// StageExampleActions carries the runtime state.
type StageExampleActions struct{}

// NewStageExampleCommand constructs the stage-example subcommand.
func NewStageExampleCommand(di do.Injector) (*StageExampleCommand, error) {
	t := do.MustInvokeStruct[*StageExampleCommand](di)
	t.Command = &cli.Command{
		Name:  "build-stage-example",
		Usage: "stage examples/<name>/ into an empty scratch dir, rewriting graphics.gd to the local checkout",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "example", Usage: "example name under examples/", Required: true},
			&cli.StringFlag{Name: "scratch", Usage: "empty target directory to stage into", Required: true},
			&cli.StringFlag{Name: "root", Usage: "path to the graphics.gd checkout (overrides env)"},
		},
		Action: shared.BindAction(t.Injector, (*StageExampleActions).action),
	}
	return t, nil
}

// NewStageExampleActions resolves the runtime state.
func NewStageExampleActions(di do.Injector) (*StageExampleActions, error) {
	return do.InvokeStruct[*StageExampleActions](di)
}

func (t *StageExampleActions) action(_ context.Context, cmd *cli.Command) error {
	name := cmd.String("example")
	target := cmd.String("scratch")

	root, err := graphicsGDRoot(cmd.String("root"))
	if err != nil {
		return err
	}
	src := filepath.Join(root, "examples", name)
	if st, err := os.Stat(src); err != nil || !st.IsDir() {
		return fmt.Errorf("no such example: %s", src)
	}
	for _, must := range []string{"go.mod", "graphics/project.godot"} {
		if _, err := os.Stat(filepath.Join(src, must)); err != nil {
			return fmt.Errorf("%s is not a valid example (missing %s)", src, must)
		}
	}
	if err := copyTree(src, target); err != nil {
		return fmt.Errorf("copy %s -> %s: %w", src, target, err)
	}
	// Rewrite the example's go.mod so its graphics.gd require
	// resolves against the actual checkout, then tidy.
	modEdit := fmt.Sprintf("-replace=graphics.gd=%s", root)
	if err := runIn(target, "go", "mod", "edit", modEdit); err != nil {
		return err
	}
	if err := runIn(target, "go", "mod", "tidy"); err != nil {
		return err
	}
	// Sanity: builders downstream assume both files are present.
	for _, must := range []string{"graphics/project.godot", "graphics/main.tscn"} {
		if _, err := os.Stat(filepath.Join(target, must)); err != nil {
			return fmt.Errorf("staged example missing %s: %w", must, err)
		}
	}
	return nil
}
