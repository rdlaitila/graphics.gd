package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"
)

// testHeadlessCmd ports cmd/gdnext-ci/test-headless.sh.
func testHeadlessCmd() *cli.Command {
	return &cli.Command{
		Name:  "test-headless",
		Usage: "run `gdnext test` against the staged example",
		Description: "Exercises the full setup-and-dispatch path of `gdnext test`\n" +
			"under `godot --headless`. The staged example has no Go tests,\n" +
			"so the run is a no-op success; the value is proving the pipe.",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "scratch", Usage: "staged example directory to test in", Required: true},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			scratch, err := filepath.Abs(cmd.String("scratch"))
			if err != nil {
				return err
			}
			if st, err := os.Stat(scratch); err != nil || !st.IsDir() {
				return fmt.Errorf("scratch dir does not exist: %s", scratch)
			}
			return runIn(scratch, "gdnext", "test")
		},
	}
}
