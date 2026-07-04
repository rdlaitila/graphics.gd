package cli

import (
	"context"
	"fmt"

	"graphics.gd/cmd/gdnext/internal/shared"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
)

// IosCommand wires `gdnext ios`.
type IosCommand struct {
	*cli.Command
	Injector do.Injector `do:""`
}

type IosActions struct{}

func NewIosCommand(di do.Injector) (*IosCommand, error) {
	t := do.MustInvokeStruct[*IosCommand](di)
	t.Command = &cli.Command{
		Name:  "ios",
		Usage: "iOS-specific helpers",
		Commands: []*cli.Command{
			{
				Name:   "xcode-gen",
				Usage:  "generate releases/ios/<arch>/<project>.xcodeproj (runs gdnext build --goos ios)",
				Action: shared.BindAction(t.Injector, (*IosActions).xcodeGen),
			},
		},
	}
	return t, nil
}

func NewIosActions(di do.Injector) (*IosActions, error) {
	return do.InvokeStruct[*IosActions](di)
}

func (t *IosActions) xcodeGen(_ context.Context, _ *cli.Command) error {
	fmt.Println("Run `GOOS=ios gdnext build` to drive xcode-gen via the existing builder.IOS.BuildMain flow.")
	fmt.Println("(Standalone Xcode-project generation is queued for a future builder/ios.go refactor.)")
	return nil
}
