package main

import (
	"context"

	"github.com/urfave/cli/v3"
)

// buildVetTestCmd ports cmd/gdnext-ci/build-vet-test.sh.
func buildVetTestCmd() *cli.Command {
	return &cli.Command{
		Name:  "build-vet-test",
		Usage: "compile, vet, and unit-test the gdnext CLI tree",
		Description: "Runs `go build`, `go vet`, and `go test` against ./cmd/gdnext/...\n" +
			"in sequence. Any failure aborts. Equivalent to the legacy\n" +
			"cmd/gdnext-ci/build-vet-test.sh shell script.",
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
