package ci

import (
	"context"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
)

// helpVerbs is the snapshot of registered gdnext subcommands the
// help-text smoke test asserts each respond to `--help`. Keep in sync
// with the cli.commands() list in cmd/gdnext/internal/cli/cli.go —
// a rename there should fail this list and prompt an update.
var helpVerbs = []string{
	"build", "run", "test", "export", "doc", "fix", "version",
	"project", "toolchain", "platform",
	"android", "ios", "macos", "web", "musl", "ci",
}

// HelpTextCommand exposes `gdnext ci help-text`: assert every
// registered gdnext verb resolves --help.
type HelpTextCommand struct {
	*cli.Command
}

// NewHelpTextCommand constructs the help-text subcommand.
func NewHelpTextCommand(di do.Injector) (*HelpTextCommand, error) {
	t := do.MustInvokeStruct[*HelpTextCommand](di)
	t.Command = &cli.Command{
		Name:   "help-text",
		Usage:  "assert every registered gdnext verb resolves --help",
		Action: t.action,
	}
	return t, nil
}

func (t *HelpTextCommand) action(_ context.Context, _ *cli.Command) error {
	out, err := output("gdnext", "--help")
	if err != nil {
		return err
	}
	if err := mustContain("gdnext --help", out, "gdnext"); err != nil {
		return err
	}
	for _, v := range helpVerbs {
		out, err := output("gdnext", v, "--help")
		if err != nil {
			return err
		}
		if err := mustContain("gdnext "+v+" --help", out, v); err != nil {
			return err
		}
	}
	return nil
}
