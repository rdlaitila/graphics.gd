package cli

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"

	"graphics.gd/cmd/gdnext/internal/builder"
	"graphics.gd/cmd/gdnext/internal/shared"
	"graphics.gd/product"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
)

// LibGodotCommand wires `gdnext libgodot`.
type LibGodotCommand struct {
	*cli.Command
	Injector do.Injector `do:""`
}

// LibGodotActions carries the runtime state resolved after the root
// Before hook has finalised the BuildEnv.
type LibGodotActions struct {
	LibGodot *builder.LibGodot `do:""`
	BuildEnv product.BuildEnv  `do:""`
}

func NewLibGodotCommand(di do.Injector) (*LibGodotCommand, error) {
	t := do.MustInvokeStruct[*LibGodotCommand](di)
	t.Command = &cli.Command{
		Name:  "libgodot",
		Usage: "build and install libgodot.<goos>.<goarch>.a static libraries",
		Commands: []*cli.Command{
			{
				Name:  "build",
				Usage: "fetch pinned godot source (via toolchain), scons library_type=static_library, drop the .a into $GDPATH/godot-src/<ref>/bin",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "goos", Usage: "target GOOS (default: host)"},
					&cli.StringFlag{Name: "goarch", Usage: "target GOARCH (default: host)"},
					&cli.BoolFlag{Name: "editor", Usage: "build the editor variant (target=editor) instead of template_release"},
					&cli.StringFlag{Name: "libc", Usage: "linux libc variant: glibc (default, zig-cc pinned to glibc 2.28) or musl (opt-in, zig + dlopen shim)"},
				},
				Action: shared.BindAction(t.Injector, (*LibGodotActions).build),
			},
			{
				Name:  "install",
				Usage: "sanity-check the artefact from a prior `libgodot build` (or --from) and copy it to $GDPATH/lib with a sha256 sidecar. Does NOT build.",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "goos", Usage: "target GOOS (default: host)"},
					&cli.StringFlag{Name: "goarch", Usage: "target GOARCH (default: host)"},
					&cli.BoolFlag{Name: "editor", Usage: "install the editor variant"},
					&cli.StringFlag{Name: "libc", Usage: "linux libc variant to install: glibc (default) or musl"},
					&cli.StringFlag{Name: "from", Usage: "path to a pre-built .a to install (default: look under $GDPATH/godot-src/<ref>/bin for the artefact `libgodot build` produced)"},
				},
				Action: shared.BindAction(t.Injector, (*LibGodotActions).install),
			},
			{
				Name:   "version",
				Usage:  "print the pinned godot ref libgodot builds check out",
				Action: shared.BindAction(t.Injector, (*LibGodotActions).version),
			},
			{
				Name:   "list",
				Usage:  "list every (goos, goarch, editor) recipe in product.LibGodotMatrix",
				Action: shared.BindAction(t.Injector, (*LibGodotActions).list),
			},
			{
				Name:  "clean",
				Usage: "remove SCons object/archive files under the godot-src cache so the next build recompiles from scratch",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "goos", Usage: "target GOOS (default: host)"},
					&cli.StringFlag{Name: "goarch", Usage: "target GOARCH (default: host)"},
					&cli.BoolFlag{Name: "editor", Usage: "target the editor recipe instead of template_release"},
					&cli.StringFlag{Name: "libc", Usage: "linux libc variant to clean: glibc (default) or musl"},
					&cli.BoolFlag{Name: "all-recipes", Usage: "ignore the recipe filter and wipe every .o/.a in the source tree"},
					&cli.BoolFlag{Name: "source-tree", Usage: "delete the whole godot-src/<ref>/ directory (forces a re-download on next build)"},
					&cli.BoolFlag{Name: "shims", Usage: "also wipe $GDPATH/libgodot-shim/"},
					&cli.BoolFlag{Name: "installed", Usage: "also delete the installed .a + checksum sidecar under $GDPATH/lib"},
					&cli.BoolFlag{Name: "dry-run", Aliases: []string{"n"}, Usage: "list what would be removed without touching anything"},
				},
				Action: shared.BindAction(t.Injector, (*LibGodotActions).clean),
			},
		},
	}
	return t, nil
}

func NewLibGodotActions(di do.Injector) (*LibGodotActions, error) {
	return do.InvokeStruct[*LibGodotActions](di)
}

func (t *LibGodotActions) build(_ context.Context, cmd *cli.Command) error {
	recipe, err := t.resolveRecipe(cmd)
	if err != nil {
		return err
	}
	_, err = t.LibGodot.Build(recipe)
	return err
}

func (t *LibGodotActions) install(_ context.Context, cmd *cli.Command) error {
	recipe, err := t.resolveRecipe(cmd)
	if err != nil {
		return err
	}
	from := cmd.String("from")
	if from == "" {
		// Locate the artefact a prior `gdnext libgodot build` left
		// on disk. install never triggers a build — it's a plain
		// sanity-check + copy + sidecar step.
		artefact, err := t.LibGodot.LocateArtefact(recipe)
		if err != nil {
			return err
		}
		from = artefact
	}
	installed, _, err := t.LibGodot.Install(recipe, from)
	if err != nil {
		return err
	}
	fmt.Println(installed)
	return nil
}

func (t *LibGodotActions) version(_ context.Context, _ *cli.Command) error {
	fmt.Println(product.LibGodotRef)
	return nil
}

func (t *LibGodotActions) clean(_ context.Context, cmd *cli.Command) error {
	recipe, err := t.resolveRecipe(cmd)
	if err != nil {
		return err
	}
	return t.LibGodot.Clean(recipe, builder.CleanOptions{
		AllRecipes: cmd.Bool("all-recipes"),
		SourceTree: cmd.Bool("source-tree"),
		Shims:      cmd.Bool("shims"),
		Installed:  cmd.Bool("installed"),
		DryRun:     cmd.Bool("dry-run"),
	})
}

func (t *LibGodotActions) list(_ context.Context, _ *cli.Command) error {
	table := make([][]string, 0, len(product.LibGodotMatrix))
	for _, r := range product.LibGodotMatrix {
		libc := r.LibC
		if libc == "" {
			libc = "-"
		}
		table = append(table, []string{
			r.GOOS + "/" + r.GOARCH,
			libc,
			strconv.FormatBool(r.Editor),
			r.GodotPlatform + "/" + r.GodotArch,
			r.ArtefactName,
			r.InstallName,
		})
	}
	renderTable(os.Stdout,
		[]string{"GOOS/GOARCH", "LIBC", "EDITOR", "PLATFORM", "ARTEFACT", "INSTALLED AS"},
		table)
	return nil
}

// resolveRecipe reads --goos / --goarch / --editor / --libc with
// sensible defaults (host GOOS/GOARCH, non-editor, glibc on linux)
// and returns the matching recipe from product.LibGodotMatrix.
func (t *LibGodotActions) resolveRecipe(cmd *cli.Command) (product.LibGodotRecipe, error) {
	goos := cmd.String("goos")
	if goos == "" {
		goos = runtime.GOOS
	}
	goarch := cmd.String("goarch")
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	editor := cmd.Bool("editor")
	libc := cmd.String("libc")
	if libc != "" && libc != product.LibCGlibc && libc != product.LibCMusl {
		return product.LibGodotRecipe{}, fmt.Errorf("--libc must be %q or %q, got %q", product.LibCGlibc, product.LibCMusl, libc)
	}
	recipe, ok := product.FindLibGodotRecipeLibC(goos, goarch, editor, libc)
	if !ok {
		return product.LibGodotRecipe{}, fmt.Errorf("no libgodot recipe for %s/%s editor=%v libc=%q (see `gdnext libgodot list`)", goos, goarch, editor, libc)
	}
	return recipe, nil
}
