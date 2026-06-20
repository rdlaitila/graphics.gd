package internal

import (
	"context"
	"fmt"
	"strings"

	"github.com/urfave/cli/v3"
)

func GoPassthroughCmd() *cli.Command {
	return &cli.Command{
		Name:  "go-passthrough",
		Usage: "confirm gdnext forwards unknown verbs to the underlying go toolchain",
		Action: func(_ context.Context, _ *cli.Command) error {
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
		},
	}
}
