package internal

import (
	"context"

	"github.com/urfave/cli/v3"
)

// BuildVetTestCmd ports cmd/gdnext-ci/build-vet-test.sh.
func BuildVetTestCmd() *cli.Command {
	return &cli.Command{
		Name:  "build-vet-test",
		Usage: "compile, vet, and unit-test the gdnext CLI tree",
		Action: func(_ context.Context, _ *cli.Command) error {
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
		},
	}
}
