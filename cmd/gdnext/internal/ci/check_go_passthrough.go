package ci

import (
	"context"
	"fmt"
	"strings"

	"graphics.gd/cmd/gdnext/internal/shared"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
)

// GoPassthroughCommand wires `gdnext ci check-go-passthrough`. Runtime
// state lives on *GoPassthroughActions.
type GoPassthroughCommand struct {
	*cli.Command
	Injector do.Injector `do:""`
}

// GoPassthroughActions carries the runtime state.
type GoPassthroughActions struct{}

// NewGoPassthroughCommand constructs the go-passthrough subcommand.
func NewGoPassthroughCommand(di do.Injector) (*GoPassthroughCommand, error) {
	t := do.MustInvokeStruct[*GoPassthroughCommand](di)
	t.Command = &cli.Command{
		Name:   "check-go-passthrough",
		Usage:  "confirm gdnext forwards unknown verbs to the underlying go toolchain",
		Action: shared.BindAction(t.Injector, (*GoPassthroughActions).action),
	}
	return t, nil
}

// NewGoPassthroughActions resolves the runtime state.
func NewGoPassthroughActions(di do.Injector) (*GoPassthroughActions, error) {
	return do.InvokeStruct[*GoPassthroughActions](di)
}

func (t *GoPassthroughActions) action(_ context.Context, _ *cli.Command) error {
	if err := run("gdnext", "env", "GOVERSION"); err != nil {
		return err
	}
	std, err := output("gdnext", "list", "std")
	if err != nil {
		return err
	}
	lines := strings.SplitN(std, "\n", 4)
	for i := 0; i < len(lines) && i < 3; i++ {
		fmt.Println(lines[i])
	}
	mod, err := output("gdnext", "mod", "help")
	if err != nil {
		return err
	}
	return mustContain("gdnext mod help", mod, "Go mod")
}
