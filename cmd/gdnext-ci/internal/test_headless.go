package internal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"
)

// TestHeadlessCmd ports cmd/gdnext-ci/test-headless.sh.
func TestHeadlessCmd() *cli.Command {
	return &cli.Command{
		Name:  "test-headless",
		Usage: "run `gdnext test` against the staged example",
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
