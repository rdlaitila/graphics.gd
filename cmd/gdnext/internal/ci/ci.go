// Package ci hosts the CI driver wired into the main gdnext CLI as
// `gdnext ci <verb>`. Each verb is a thin orchestration step the
// GitHub Actions workflow at .github/workflows/gdnext.yml calls;
// folding them into gdnext lets the driver share the same dependency
// injection plumbing (BuildEnv, ToolCatalog, builders) the rest of
// the CLI uses.
//
// Verbs that need to drive `gdnext` itself still shell out via the
// helpers in helpers.go — that keeps each step independent and lets
// failures surface with the same diagnostics a human would see.
package ci

import (
	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
)

// CICommand wraps the urfave Command for `gdnext ci`. The subcommand
// list is assembled in NewCICommand from the per-verb *XCommand DI
// structs registered in Provides.
type CICommand struct {
	*cli.Command
	Injector do.Injector `do:""`
}

// Provides is the package-level provider set for every CI verb plus
// the CICommand root. Register on the root injector before resolving
// the root command.
var Provides = do.Package(
	do.Lazy(NewCICommand),
	do.Lazy(NewBuildTargetCommand),
	do.Lazy(NewBuildTargetActions),
	do.Lazy(NewBuildVetTestCommand),
	do.Lazy(NewBuildVetTestActions),
	do.Lazy(NewDiagnosticVerbsCommand),
	do.Lazy(NewDiagnosticVerbsActions),
	do.Lazy(NewGoPassthroughCommand),
	do.Lazy(NewGoPassthroughActions),
	do.Lazy(NewHelpTextCommand),
	do.Lazy(NewHelpTextActions),
	do.Lazy(NewMatrixCommand),
	do.Lazy(NewMatrixActions),
	do.Lazy(NewLibGodotMatrixCommand),
	do.Lazy(NewLibGodotMatrixActions),
	do.Lazy(NewPlayCellCommand),
	do.Lazy(NewPlayCellActions),
	do.Lazy(NewPlayMatrixCommand),
	do.Lazy(NewPlayMatrixActions),
	do.Lazy(NewShortFlagRewriteCommand),
	do.Lazy(NewShortFlagRewriteActions),
	do.Lazy(NewStageExampleCommand),
	do.Lazy(NewStageExampleActions),
	do.Lazy(NewTestHeadlessCommand),
	do.Lazy(NewTestHeadlessActions),
	do.Lazy(NewTestMatrixCommand),
	do.Lazy(NewTestMatrixActions),
	do.Lazy(NewToolchainInstallCommand),
	do.Lazy(NewToolchainInstallActions),
	do.Lazy(NewToolchainChecksumsCommand),
	do.Lazy(NewToolchainChecksumsActions),
	do.Lazy(NewWorkflowSummaryCommand),
	do.Lazy(NewWorkflowSummaryActions),
)

// NewCICommand constructs the `gdnext ci` subcommand tree.
func NewCICommand(di do.Injector) (*CICommand, error) {
	t := do.MustInvokeStruct[*CICommand](di)
	t.Command = &cli.Command{
		Name:  "ci",
		Usage: "CI helpers for the gdnext GitHub Actions workflow",
		Commands: []*cli.Command{
			do.MustInvoke[*BuildVetTestCommand](di).Command,
			do.MustInvoke[*HelpTextCommand](di).Command,
			do.MustInvoke[*DiagnosticVerbsCommand](di).Command,
			do.MustInvoke[*GoPassthroughCommand](di).Command,
			do.MustInvoke[*ShortFlagRewriteCommand](di).Command,
			do.MustInvoke[*ToolchainInstallCommand](di).Command,
			do.MustInvoke[*ToolchainChecksumsCommand](di).Command,
			do.MustInvoke[*StageExampleCommand](di).Command,
			do.MustInvoke[*BuildTargetCommand](di).Command,
			do.MustInvoke[*TestHeadlessCommand](di).Command,
			do.MustInvoke[*MatrixCommand](di).Command,
			do.MustInvoke[*LibGodotMatrixCommand](di).Command,
			do.MustInvoke[*PlayMatrixCommand](di).Command,
			do.MustInvoke[*TestMatrixCommand](di).Command,
			do.MustInvoke[*PlayCellCommand](di).Command,
			do.MustInvoke[*WorkflowSummaryCommand](di).Command,
		},
	}
	return t, nil
}
