// fix.go implements "gdnext fix" — rewrite Go source code in the current
// module to migrate uses of deprecated graphics.gd symbols to their
// replacements via golang.org/x/tools/refactor/eg example-based transforms.
//
// This verb was advertised by the legacy "gd"'s stderr help text but never
// wired into the dispatcher (see cmd/gd/deprecated.go:46) — gdnext fixes that.
package cli

import (
	"context"
	_ "embed"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"runtime"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"
	"graphics.gd/cmd/gdnext/internal/refactor/eg"
	"graphics.gd/variant/String"
	"runtime.link/api/xray"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
)

//go:embed deprecated.txt
var fixes string

// FixCommand exposes `gdnext fix`: rewrite Go source code in the
// current module to migrate uses of deprecated graphics.gd symbols
// to their replacements via golang.org/x/tools/refactor/eg example-
// based transforms.
type FixCommand struct {
	*cli.Command
}

// NewFixCommand constructs the `gdnext fix` subcommand
func NewFixCommand(di do.Injector) (*FixCommand, error) {
	t := do.MustInvokeStruct[*FixCommand](di)
	t.Command = &cli.Command{
		Name:   "fix",
		Usage:  "rewrite code to migrate from deprecated graphics.gd APIs",
		Action: t.fix,
	}
	return t, nil
}

// fix runs the eg transformer over every package in the current module.
func (t *FixCommand) fix(_ context.Context, _ *cli.Command) error {
	cfg := &packages.Config{
		Fset:  token.NewFileSet(),
		Mode:  packages.NeedName | packages.NeedTypes | packages.NeedSyntax | packages.NeedImports | packages.NeedDeps | packages.NeedCompiledGoFiles,
		Tests: true,
	}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return xray.New(err)
	}
	var transformers []*eg.Transformer
	for example := range String.Splits(fixes, "\n\n") {
		f, err := parser.ParseFile(cfg.Fset, "/tmp/fixes.go", strings.NewReader(example), parser.ParseComments)
		if err != nil {
			return xray.New(err)
		}
		tInfo := types.Info{
			Types:      make(map[ast.Expr]types.TypeAndValue),
			Defs:       make(map[*ast.Ident]types.Object),
			Uses:       make(map[*ast.Ident]types.Object),
			Implicits:  make(map[ast.Node]types.Object),
			Selections: make(map[*ast.SelectorExpr]*types.Selection),
			Scopes:     make(map[ast.Node]*types.Scope),
		}
		conf := types.Config{Importer: pkgsImporter(pkgs), Sizes: types.SizesFor("gc", runtime.GOARCH)}
		tPkg, _ := conf.Check("egtemplate", cfg.Fset, []*ast.File{f}, &tInfo)
		xform, err := eg.NewTransformer(cfg.Fset, tPkg, f, &tInfo, false)
		if err != nil {
			return xray.New(err)
		}
		transformers = append(transformers, xform)
	}
	var hadErrors bool
	for _, pkg := range pkgs {
		for i, filename := range pkg.CompiledGoFiles {
			if filename == "/tmp/fixes.go" {
				continue
			}
			file := pkg.Syntax[i]
			var n int
			for _, xform := range transformers {
				n += xform.Transform(pkg.TypesInfo, pkg.Types, file)
			}
			if n == 0 {
				continue
			}
			fmt.Fprintf(os.Stderr, "=== %s (%d matches)\n", filename, n)
			if err := eg.WriteAST(cfg.Fset, filename, file); err != nil {
				fmt.Fprintf(os.Stderr, "eg: %s\n", err)
				hadErrors = true
			}
		}
	}
	if hadErrors {
		os.Exit(1)
	}
	return nil
}

// fixHint scans the supplied list of unresolved-symbol error names and, if
// any of them match a deprecated symbol declared in deprecated.txt, prints a
// hint to stderr telling the user to run "gdnext fix". This was orphaned in
// the legacy cmd/gd; we expose it for future re-wiring (e.g. from a build
// failure hook).
func fixHint(undefined []string) {
	for example := range String.Splits(fixes, "\n\n") {
		_, before, _ := strings.Cut(example, "func before(")
		_, name, _ := strings.Cut(before, "{")
		_, nameAfterReturn, ok := strings.Cut(name, "return")
		if ok {
			name = nameAfterReturn
		}
		name, _, _ = strings.Cut(name, "(")
		name = strings.TrimSpace(name)
		if slices.Contains(undefined, name) {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "NOTE it looks like some of your compilation errors may be fixed by running `gdnext fix`")
			fmt.Fprintln(os.Stderr, "this will rewrite your project to refactor deprecated functions to use the new API.")
			fmt.Fprintln(os.Stderr, "(you should back up your code or use version control before running this command).")
			fmt.Fprintln(os.Stderr)
			return
		}
	}
}

type pkgsImporter []*packages.Package

func (p pkgsImporter) Import(path string) (tpkg *types.Package, err error) {
	packages.Visit([]*packages.Package(p), func(pkg *packages.Package) bool {
		if pkg.PkgPath == path {
			tpkg = pkg.Types
			return false
		}
		return true
	}, nil)
	if tpkg != nil {
		return tpkg, nil
	}
	return nil, fmt.Errorf("package %q not found", path)
}
