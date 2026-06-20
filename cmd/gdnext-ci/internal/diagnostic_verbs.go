package internal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"graphics.gd/product"

	"github.com/urfave/cli/v3"
)

// DiagnosticVerbsCmd ports cmd/gdnext-ci/diagnostic-verbs.sh. Must run
// BEFORE toolchain-install so the no-download invariant on
// `gdnext toolchain doctor` is meaningful (the $GDPATH is still empty).
func DiagnosticVerbsCmd() *cli.Command {
	return &cli.Command{
		Name:  "diagnostic-verbs",
		Usage: "verify the diagnostic verbs that must work without any toolchain installed",
		Action: func(_ context.Context, _ *cli.Command) error {
			if err := run("gdnext", "version"); err != nil {
				return err
			}
			listing, err := output("gdnext", "toolchain", "list")
			if err != nil {
				return err
			}
			fmt.Println(listing)
			for _, t := range product.ToolchainMatrix {
				if !listingContainsSlug(listing, t.Slug) {
					return fmt.Errorf("`gdnext toolchain list` missing entry: %s", t.Slug)
				}
			}
			gdpath := os.Getenv("GDPATH")
			if gdpath == "" {
				if home, err := os.UserHomeDir(); err == nil {
					gdpath = filepath.Join(home, "gd")
				}
			}
			before, err := dirSize(gdpath)
			if err != nil {
				return err
			}
			// Doctor is expected to exit non-zero on a fresh runner (no
			// tools yet); we tolerate that and only enforce the no-mutate
			// invariant on $GDPATH.
			_ = run("gdnext", "toolchain", "doctor")
			after, err := dirSize(gdpath)
			if err != nil {
				return err
			}
			if before != after {
				return fmt.Errorf("`gdnext toolchain doctor` mutated %s: %d -> %d bytes", gdpath, before, after)
			}
			return nil
		},
	}
}

// listingContainsSlug looks for slug as the first whitespace-delimited
// token on some line of the `gdnext toolchain list` table. Equivalent
// to `grep -qE "^[[:space:]]*$slug([[:space:]]|$)"` in the shell.
func listingContainsSlug(listing, slug string) bool {
	for _, line := range strings.Split(listing, "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == slug {
			return true
		}
		if strings.HasPrefix(trimmed, slug) {
			rest := trimmed[len(slug):]
			if rest == "" || rest[0] == ' ' || rest[0] == '\t' {
				return true
			}
		}
	}
	return false
}
