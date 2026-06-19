package cli

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
)

func iosCmd() *cli.Command {
	return &cli.Command{
		Name:  "ios",
		Usage: "iOS-specific helpers",
		Commands: []*cli.Command{
			{
				Name:  "xcode-gen",
				Usage: "generate releases/ios/<arch>/<project>.xcodeproj (runs gdnext build --goos ios)",
				Action: func(_ context.Context, _ *cli.Command) error {
					fmt.Println("Run `GOOS=ios gdnext build` to drive xcode-gen via the existing builder.IOS.BuildMain flow.")
					fmt.Println("(Standalone Xcode-project generation is queued for a future builder/ios.go refactor.)")
					return nil
				},
			},
		},
	}
}
