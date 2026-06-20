package internal

import (
	"context"

	"github.com/urfave/cli/v3"
)

// helpVerbs is the snapshot of registered gdnext subcommands the
// help-text smoke test asserts each respond to `--help`. Keep in sync
// with gdcli.Commands() in cmd/gdnext/internal/cli/commands.go — a
// rename there should fail this list and prompt an update.
var helpVerbs = []string{
	"build", "run", "test", "export", "doc", "fix", "version",
	"project", "toolchain", "platforms",
	"android", "ios", "macos", "web", "musl",
}

// HelpTextCmd ports cmd/gdnext-ci/help-text.sh.
func HelpTextCmd() *cli.Command {
	return &cli.Command{
		Name:  "help-text",
		Usage: "assert every registered gdnext verb resolves --help",
		Action: func(_ context.Context, _ *cli.Command) error {
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
		},
	}
}
