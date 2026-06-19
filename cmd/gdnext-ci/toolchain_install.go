package main

import (
	"context"
	"fmt"
	"os"

	"graphics.gd/product"

	"github.com/urfave/cli/v3"
)

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

// toolchainInstallCmd ports cmd/gdnext-ci/toolchain-install.sh.
func toolchainInstallCmd() *cli.Command {
	return &cli.Command{
		Name:  "toolchain-install",
		Usage: "smoke-test `gdnext toolchain install` and its path round-trip",
		Description: "Two contracts:\n" +
			"  1. The argless walk (`gdnext toolchain install`) logs a result\n" +
			"     per entry and exits 0 even when individual entries are\n" +
			"     unreachable (ldd on macos/windows, libgodot 404s, ...).\n" +
			"  2. For every installable entry in product.ToolchainMatrix,\n" +
			"     `install <slug>` followed by `path <slug>` round-trips to\n" +
			"     an on-disk file. Libraries (IsLibrary) and a small skip\n" +
			"     set (go, ldd) are excluded; optional entries (upx, vpk)\n" +
			"     are best-effort.\n" +
			"\n" +
			"Per-target REQUIRED/OPTIONAL checks happen later inside\n" +
			"`gdnext-ci build-target` via `gdnext toolchain doctor --fix`,\n" +
			"so this verb does NOT call doctor — it stays focused on the\n" +
			"install verb's own contract.",
		Action: func(_ context.Context, _ *cli.Command) error {
			fmt.Println("=== gdnext toolchain install (full walk) ===")
			if err := run("gdnext", "toolchain", "install"); err != nil {
				return err
			}

			mandatory, optional := classifyInstallables()
			for _, slug := range mandatory {
				fmt.Println()
				fmt.Printf("=== %s ===\n", slug)
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
				fmt.Println()
				fmt.Printf("=== %s (best-effort) ===\n", slug)
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
		},
	}
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
