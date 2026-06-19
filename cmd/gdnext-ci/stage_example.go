package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"
)

// stageExampleCmd ports cmd/gdnext-ci/stage-example.sh.
func stageExampleCmd() *cli.Command {
	return &cli.Command{
		Name:  "stage-example",
		Usage: "stage examples/<name>/ into an empty scratch dir, rewriting graphics.gd to the local checkout",
		Description: "Copies examples/<name>/ recursively into --scratch,\n" +
			"then `go mod edit -replace=graphics.gd=<root>` + `go mod tidy`\n" +
			"so the staged copy resolves against the actual checkout rather\n" +
			"than the example's committed `replace ../..` line.\n" +
			"\n" +
			"<root> is resolved in this priority order:\n" +
			"  --root flag, $GRAPHICS_GD_ROOT, $GITHUB_WORKSPACE,\n" +
			"  walk-up from cwd for `module graphics.gd`.",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "example", Usage: "example name under examples/", Required: true},
			&cli.StringFlag{Name: "scratch", Usage: "empty target directory to stage into", Required: true},
			&cli.StringFlag{Name: "root", Usage: "path to the graphics.gd checkout (overrides env)"},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			name := cmd.String("example")
			target := cmd.String("scratch")

			root, err := graphicsGDRoot(cmd.String("root"))
			if err != nil {
				return err
			}
			src := filepath.Join(root, "examples", name)
			if st, err := os.Stat(src); err != nil || !st.IsDir() {
				return fmt.Errorf("no such example: %s", src)
			}
			for _, must := range []string{"go.mod", "graphics/project.godot"} {
				if _, err := os.Stat(filepath.Join(src, must)); err != nil {
					return fmt.Errorf("%s is not a valid example (missing %s)", src, must)
				}
			}

			if err := copyTree(src, target); err != nil {
				return fmt.Errorf("copy %s -> %s: %w", src, target, err)
			}

			// Rewrite the example's go.mod so its graphics.gd require
			// resolves against the actual checkout, then tidy.
			modEdit := fmt.Sprintf("-replace=graphics.gd=%s", root)
			if err := runIn(target, "go", "mod", "edit", modEdit); err != nil {
				return err
			}
			if err := runIn(target, "go", "mod", "tidy"); err != nil {
				return err
			}

			// Sanity: builders downstream assume both files are present.
			for _, must := range []string{"graphics/project.godot", "graphics/main.tscn"} {
				if _, err := os.Stat(filepath.Join(target, must)); err != nil {
					return fmt.Errorf("staged example missing %s: %w", must, err)
				}
			}
			return nil
		},
	}
}
