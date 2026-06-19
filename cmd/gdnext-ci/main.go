// Command gdnext-ci is the CI driver for the gdnext workflow. Each
// subcommand is a one-for-one port of a former bash script under
// cmd/gdnext-ci/; the urfave/cli help text on each is the canonical
// usage documentation that used to live in shebang-line comments.
//
// All platform-shaped logic uses graphics.gd/product instead of
// hand-rolled magic strings so a typo in a renamed GOOS surfaces as a
// compile error or a Lookup miss rather than a silent test pass.
//
// The workflow installs this binary alongside gdnext itself
// (`go install ./cmd/gdnext-ci`) and invokes verbs by name —
// `gdnext-ci build-vet-test`, `gdnext-ci build-target linux amd64 $tmp`,
// and so on.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"
)

func main() {
	cmd := &cli.Command{
		Name:    "gdnext-ci",
		Usage:   "CI helpers for the gdnext-ci workflow",
		Version: "(devel)",
		Description: "One subcommand per phase of .github/workflows/gdnext-ci.yml.\n" +
			"Each verb is a one-for-one port of a former cmd/gdnext-ci/*.sh\n" +
			"script; the urfave/cli help text below is the canonical reference\n" +
			"for how the workflow invokes it.",
		Commands: []*cli.Command{
			buildVetTestCmd(),
			helpTextCmd(),
			diagnosticVerbsCmd(),
			goPassthroughCmd(),
			shortFlagRewriteCmd(),
			toolchainInstallCmd(),
			stageExampleCmd(),
			buildTargetCmd(),
			testHeadlessCmd(),
		},
	}
	if err := cmd.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
