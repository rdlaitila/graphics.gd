package ci

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"graphics.gd/cmd/gdnext/internal/shared"
	"graphics.gd/product"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
)

// LibGodotMatrixCommand wires `gdnext ci libgodot-matrix`. Emits one
// row per buildable product.LibGodotRecipe with the GHA runner label
// derived from the recipe's build-host requirements (linuxbsd cross
// runs on ubuntu, windows via zig-mingw also on ubuntu, darwin on
// macos-latest). Rows are consumed by the libgodot workflow.
type LibGodotMatrixCommand struct {
	*cli.Command
	Injector do.Injector `do:""`
}

// LibGodotMatrixActions carries the runtime state.
type LibGodotMatrixActions struct{}

type libgodotMatrixRow struct {
	Runner   string `json:"runner"`
	GOOS     string `json:"goos"`
	GOARCH   string `json:"goarch"`
	LibC     string `json:"libc,omitempty"`
	Editor   bool   `json:"editor"`
	Variant  string `json:"variant"`
	Install  string `json:"install"`
	Artefact string `json:"artefact"`
}

// NewLibGodotMatrixCommand constructs `gdnext ci libgodot-matrix`.
func NewLibGodotMatrixCommand(di do.Injector) (*LibGodotMatrixCommand, error) {
	t := do.MustInvokeStruct[*LibGodotMatrixCommand](di)
	t.Command = &cli.Command{
		Name:  "libgodot-matrix",
		Usage: "emit the GHA matrix derived from product.LibGodotMatrix",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "filter-target",
				Usage: "comma-separated <goos>/<goarch> tuples; empty allows every recipe",
			},
			&cli.StringFlag{
				Name:  "filter-libc",
				Usage: "comma-separated libc variants (glibc|musl); empty allows both",
			},
			&cli.BoolFlag{
				Name:  "editor-only",
				Usage: "restrict to editor variants",
			},
			&cli.BoolFlag{
				Name:  "template-only",
				Usage: "restrict to template_release variants",
			},
			&cli.BoolFlag{
				Name:  "summary",
				Usage: "also print a human-readable matrix to stderr",
			},
		},
		Action: shared.BindAction(t.Injector, (*LibGodotMatrixActions).action),
	}
	return t, nil
}

// NewLibGodotMatrixActions resolves the runtime state.
func NewLibGodotMatrixActions(di do.Injector) (*LibGodotMatrixActions, error) {
	return do.InvokeStruct[*LibGodotMatrixActions](di)
}

func (t *LibGodotMatrixActions) action(_ context.Context, cmd *cli.Command) error {
	targetFilter := splitTuples(cmd.String("filter-target"))
	libcFilter := splitTuples(cmd.String("filter-libc"))
	rows := libgodotMatrix(targetFilter, libcFilter, cmd.Bool("editor-only"), cmd.Bool("template-only"))
	doc := struct {
		Include []libgodotMatrixRow `json:"include"`
	}{Include: rows}
	out, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	if cmd.Bool("summary") {
		fmt.Fprintf(os.Stderr, "libgodot matrix (%d cells):\n", len(rows))
		for _, r := range rows {
			libc := r.LibC
			if libc == "" {
				libc = "-"
			}
			fmt.Fprintf(os.Stderr, "  %-14s  %s/%s libc=%-5s editor=%-5v -> %s\n",
				r.Runner, r.GOOS, r.GOARCH, libc, r.Editor, r.Install)
		}
	}
	return nil
}

// libgodotMatrix walks product.LibGodotMatrix and emits one row per
// recipe that has a known runner + upstream Godot supports the target.
// ios/android recipes are declared in the matrix for parity with the
// CLI but upstream Godot's platform drivers don't accept
// library_type=static_library for them, so they're omitted here.
func libgodotMatrix(targetFilter, libcFilter []string, editorOnly, templateOnly bool) []libgodotMatrixRow {
	var out []libgodotMatrixRow
	for _, r := range product.LibGodotMatrix {
		runner, ok := libgodotRunnerFor(r)
		if !ok {
			continue
		}
		tuple := r.GOOS + "/" + r.GOARCH
		if !matchesAny(tuple, targetFilter) {
			continue
		}
		if r.LibC != "" && !matchesAny(r.LibC, libcFilter) {
			continue
		}
		if editorOnly && !r.Editor {
			continue
		}
		if templateOnly && r.Editor {
			continue
		}
		variant := "release"
		if r.Editor {
			variant = "editor"
		}
		out = append(out, libgodotMatrixRow{
			Runner:   runner,
			GOOS:     r.GOOS,
			GOARCH:   r.GOARCH,
			LibC:     r.LibC,
			Editor:   r.Editor,
			Variant:  variant,
			Install:  r.InstallName,
			Artefact: r.ArtefactName,
		})
	}
	return out
}

// libgodotRunnerFor returns the GHA runner label a recipe should build
// on, and false when the recipe has no CI-runnable host (ios/android).
// Rules mirror LibGodot.hostCanBuild in cmd/gdnext/internal/builder.
func libgodotRunnerFor(r product.LibGodotRecipe) (string, bool) {
	switch r.GOOS {
	case product.GOOSLinux:
		if r.GOARCH == product.GOARCHArm64 {
			return "ubuntu-24.04-arm", true
		}
		return "ubuntu-latest", true
	case product.GOOSWindows:
		return "ubuntu-latest", true
	case product.GOOSDarwin:
		return "macos-latest", true
	case product.GOOSIOS, product.GOOSAndroid:
		// Upstream Godot's platform drivers don't currently accept
		// library_type=static_library for these targets; see
		// LibGodot.hostCanBuild for the details. Skip in CI.
		return "", false
	default:
		return "", false
	}
}
