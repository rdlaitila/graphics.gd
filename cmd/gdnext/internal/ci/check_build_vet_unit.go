package ci

import (
	"context"

	"graphics.gd/cmd/gdnext/internal/shared"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
)

// BuildVetTestCommand wires `gdnext ci check-build-vet-test`.
type BuildVetTestCommand struct {
	*cli.Command
	Injector do.Injector `do:""`
}

type BuildVetTestActions struct{}

func NewBuildVetTestCommand(di do.Injector) (*BuildVetTestCommand, error) {
	t := do.MustInvokeStruct[*BuildVetTestCommand](di)
	t.Command = &cli.Command{
		Name:   "check-build-vet-test",
		Usage:  "compile, vet, and unit-test the gdnext CLI tree",
		Action: shared.BindAction(t.Injector, (*BuildVetTestActions).action),
	}
	return t, nil
}

func NewBuildVetTestActions(di do.Injector) (*BuildVetTestActions, error) {
	return do.InvokeStruct[*BuildVetTestActions](di)
}

func (t *BuildVetTestActions) action(_ context.Context, _ *cli.Command) error {
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
