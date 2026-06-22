package ci

import (
	"context"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
)

// BuildVetTestCommand exposes `gdnext ci build-vet-test`: compile,
// vet, and unit-test the gdnext CLI tree.
type BuildVetTestCommand struct {
	*cli.Command
}

// NewBuildVetTestCommand constructs the build-vet-test subcommand.
func NewBuildVetTestCommand(di do.Injector) (*BuildVetTestCommand, error) {
	t := do.MustInvokeStruct[*BuildVetTestCommand](di)
	t.Command = &cli.Command{
		Name:   "build-vet-test",
		Usage:  "compile, vet, and unit-test the gdnext CLI tree",
		Action: t.action,
	}
	return t, nil
}

func (t *BuildVetTestCommand) action(_ context.Context, _ *cli.Command) error {
	for _, step := range [][]string{
		{"build", "./cmd/gdnext/..."},
		{"vet", "./cmd/gdnext/..."},
		{"test", "./cmd/gdnext/..."},
	} {
		if err := run("go", step...); err != nil {
			return err
		}
	}
	return nil
}
