// Command gdnext-ci is the CI driver for the gdnext workflow. Every
// verb is defined in cmd/gdnext-ci/internal; this file only wires them
// into the root command tree so `go run ./cmd/gdnext-ci <verb>` works
// for one-shot local use.
//
// All platform-shaped logic uses graphics.gd/product instead of
// hand-rolled magic strings so a typo in a renamed GOOS surfaces as a
// compile error or a Lookup miss rather than a silent test pass.
//
// The workflow installs this binary alongside gdnext itself
// (`go install ./cmd/gdnext-ci`) and invokes verbs by name —
// `gdnext-ci build-vet-test`,
// `gdnext-ci build-target --goos … --goarch … --scratch $tmp`, ...
package main

import (
	"context"
	"fmt"
	"os"

	"graphics.gd/cmd/gdnext-ci/internal"

	"github.com/urfave/cli/v3"
)

func main() {
	cmd := &cli.Command{
		Name:    "gdnext-ci",
		Usage:   "CI helpers for the gdnext-ci workflow",
		Version: "(devel)",
		Commands: []*cli.Command{
			internal.BuildVetTestCmd(),
			internal.HelpTextCmd(),
			internal.DiagnosticVerbsCmd(),
			internal.GoPassthroughCmd(),
			internal.ShortFlagRewriteCmd(),
			internal.ToolchainInstallCmd(),
			internal.StageExampleCmd(),
			internal.BuildTargetCmd(),
			internal.TestHeadlessCmd(),
			internal.MatrixCmd(),
			internal.WorkflowSummaryCmd(),
		},
	}
	if err := cmd.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
