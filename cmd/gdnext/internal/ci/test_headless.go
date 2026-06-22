package ci

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
)

// TestHeadlessCommand exposes `gdnext ci test-headless`: run
// `gdnext test` against the staged example.
type TestHeadlessCommand struct {
	*cli.Command
}

// NewTestHeadlessCommand constructs the test-headless subcommand.
func NewTestHeadlessCommand(di do.Injector) (*TestHeadlessCommand, error) {
	t := do.MustInvokeStruct[*TestHeadlessCommand](di)
	t.Command = &cli.Command{
		Name:  "test-headless",
		Usage: "run `gdnext test` against the staged example",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "scratch", Usage: "staged example directory to test in", Required: true},
		},
		Action: t.action,
	}
	return t, nil
}

func (t *TestHeadlessCommand) action(_ context.Context, cmd *cli.Command) error {
	scratch, err := filepath.Abs(cmd.String("scratch"))
	if err != nil {
		return err
	}
	if st, err := os.Stat(scratch); err != nil || !st.IsDir() {
		return fmt.Errorf("scratch dir does not exist: %s", scratch)
	}
	return runIn(scratch, "gdnext", "test")
}
