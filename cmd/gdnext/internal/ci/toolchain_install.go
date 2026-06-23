package ci

import (
	"context"
	"fmt"
	"os"

	"graphics.gd/cmd/gdnext/internal/shared"
	"graphics.gd/product"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
)

// ToolchainInstallCommand wires `gdnext ci toolchain-install`.
// Runtime state lives on *ToolchainInstallActions.
type ToolchainInstallCommand struct {
	*cli.Command
	Injector do.Injector `do:""`
}

// ToolchainInstallActions carries the runtime state.
type ToolchainInstallActions struct{}

// installSkip is the small set of slugs the install round-trip can't
// or shouldn't exercise:
//   - "go"   gdnext is already running on it
//   - "ldd"  system tool, never downloaded
//
// IsLibrary entries (android.jar, libgodot, libgodot-editor) are skipped
// automatically by inspecting product.Toolchain.IsLibrary — no executable
// to round-trip through `toolchain path`.
var installSkip = map[string]bool{
	"go":  true,
	"ldd": true,
}

// optionalInstall lists slugs whose upstreams are flaky enough that
// install failure is logged but not fatal. Kept as an opt-in list
// (rather than e.g. "any tool not Required by any platform") so the
// human intent stays explicit at the CI layer.
var optionalInstall = map[string]bool{
	"upx": true,
	"vpk": true,
}

// NewToolchainInstallCommand constructs the toolchain-install subcommand.
func NewToolchainInstallCommand(di do.Injector) (*ToolchainInstallCommand, error) {
	t := do.MustInvokeStruct[*ToolchainInstallCommand](di)
	t.Command = &cli.Command{
		Name:   "toolchain-install",
		Usage:  "smoke-test `gdnext toolchain install` and its path round-trip",
		Action: shared.BindAction(t.Injector, (*ToolchainInstallActions).action),
	}
	return t, nil
}

// NewToolchainInstallActions resolves the runtime state.
func NewToolchainInstallActions(di do.Injector) (*ToolchainInstallActions, error) {
	return do.InvokeStruct[*ToolchainInstallActions](di)
}

func (t *ToolchainInstallActions) action(_ context.Context, _ *cli.Command) error {
	// Phase 1 — argless walk. The announce banner from run()
	// labels the command itself; no extra header needed.
	if err := run("gdnext", "toolchain", "install"); err != nil {
		return err
	}
	mandatory, optional := classifyInstallables()
	for _, slug := range mandatory {
		if err := run("gdnext", "toolchain", "install", slug); err != nil {
			return fmt.Errorf("toolchain install %s: %w", slug, err)
		}
		path, err := output("gdnext", "toolchain", "path", slug)
		if err != nil {
			return fmt.Errorf("toolchain path %s: %w", slug, err)
		}
		if err := pathExists(path); err != nil {
			return fmt.Errorf("%s: path %s does not exist (%w)", slug, path, err)
		}
		fmt.Println(path)
	}
	for _, slug := range optional {
		out, err := outputCombined("gdnext", "toolchain", "install", slug)
		fmt.Print(out)
		if !endsWithNewline(out) {
			fmt.Println()
		}
		if err != nil {
			fmt.Printf("(optional install for %s failed; continuing)\n", slug)
		}
	}
	return nil
}

// classifyInstallables walks product.ToolchainMatrix and partitions
// the slugs the install verb should exercise into (mandatory, optional)
// according to the installSkip / optionalInstall maps above.
func classifyInstallables() (mandatory, optional []string) {
	for _, t := range product.ToolchainMatrix {
		if t.IsLibrary || installSkip[t.Slug] {
			continue
		}
		if optionalInstall[t.Slug] {
			optional = append(optional, t.Slug)
			continue
		}
		mandatory = append(mandatory, t.Slug)
	}
	return mandatory, optional
}

func endsWithNewline(s string) bool {
	if s == "" {
		return true
	}
	return s[len(s)-1] == '\n'
}

func pathExists(p string) error {
	_, err := os.Stat(p)
	return err
}
