package ci

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"graphics.gd/cmd/gdnext/internal/shared"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
)

// TestHeadlessCommand wires `gdnext ci build-test-headless`. Runtime state
// lives on *TestHeadlessActions.
type TestHeadlessCommand struct {
	*cli.Command
	Injector do.Injector `do:""`
}

// TestHeadlessActions carries the runtime state.
type TestHeadlessActions struct{}

// NewTestHeadlessCommand constructs the test-headless subcommand.
func NewTestHeadlessCommand(di do.Injector) (*TestHeadlessCommand, error) {
	t := do.MustInvokeStruct[*TestHeadlessCommand](di)
	t.Command = &cli.Command{
		Name:  "build-test-headless",
		Usage: "run `gdnext test` against the staged example",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "scratch", Usage: "staged example directory to test in", Required: true},
			&cli.StringFlag{Name: "link", Usage: "link mode (gdextension|libgodot); empty = builder default"},
		},
		Action: shared.BindAction(t.Injector, (*TestHeadlessActions).action),
	}
	return t, nil
}

// NewTestHeadlessActions resolves the runtime state.
func NewTestHeadlessActions(di do.Injector) (*TestHeadlessActions, error) {
	return do.InvokeStruct[*TestHeadlessActions](di)
}

func (t *TestHeadlessActions) action(_ context.Context, cmd *cli.Command) error {
	scratch, err := filepath.Abs(cmd.String("scratch"))
	if err != nil {
		return err
	}
	if st, err := os.Stat(scratch); err != nil || !st.IsDir() {
		return fmt.Errorf("scratch dir does not exist: %s", scratch)
	}
	// Pass -v so per-test names + timings reach the CI log instead
	// of a bare PASS/FAIL. gdnext test's SkipFlagParsing means -v is
	// a positional arg, not a gdnext flag; no `--` separator needed
	// (a leading `--` ends up in Godot's argv and stops it parsing
	// the converted -test.v).
	args := []string{}
	if link := cmd.String("link"); link != "" {
		args = append(args, "--link", link)
	}
	args = append(args, "test", "-v")
	return runIn(scratch, "gdnext", args...)
}
