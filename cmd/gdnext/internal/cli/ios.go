package cli

import (
	"context"
	"fmt"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
)

// IosCommand exposes `gdnext ios`: iOS-specific helpers.
type IosCommand struct {
	*cli.Command
}

// NewIosCommand constructs the `gdnext ios` subcommand
func NewIosCommand(di do.Injector) (*IosCommand, error) {
	t := do.MustInvokeStruct[*IosCommand](di)
	t.Command = &cli.Command{
		Name:  "ios",
		Usage: "iOS-specific helpers",
		Commands: []*cli.Command{
			{
				Name:   "xcode-gen",
				Usage:  "generate releases/ios/<arch>/<project>.xcodeproj (runs gdnext build --goos ios)",
				Action: t.xcodeGen,
			},
		},
	}
	return t, nil
}

func (t *IosCommand) xcodeGen(_ context.Context, _ *cli.Command) error {
	fmt.Println("Run `GOOS=ios gdnext build` to drive xcode-gen via the existing builder.IOS.BuildMain flow.")
	fmt.Println("(Standalone Xcode-project generation is queued for a future builder/ios.go refactor.)")
	return nil
}
