// The 'gdnext' command is a next-generation drop-in replacement for the 'gd'
// command, built on urfave/cli/v3. It exposes every operation the legacy 'gd'
// command performs (build, run, test, export, editor launch, go passthrough)
// as a self-documenting subcommand tree, plus first-class verbs for the
// previously buried subsystems (toolchain management, keystore, APK ops,
// macOS lipo/codesign, web serve, musl setup, project init, fix).
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"graphics.gd/cmd/gdnext/internal/builder"
	"graphics.gd/cmd/gdnext/internal/ci"
	gcli "graphics.gd/cmd/gdnext/internal/cli"
	"graphics.gd/cmd/gdnext/internal/setup"
	"graphics.gd/cmd/gdnext/internal/tooling"

	"github.com/samber/do/v2"
	ucli "github.com/urfave/cli/v3"
)

func main() {
	di := do.New()
	// Register the injector itself so commands can declare an
	// `Injector do.Injector` DI field (needed by build / run / test
	// to lazily resolve builders via setup.ForBuild).
	do.ProvideValue(di, di)
	gcli.Provides(di)
	setup.Provides(di)
	tooling.Provides(di)
	builder.Provides(di)
	ci.Provides(di)
	root := do.MustInvoke[*gcli.RootCommand](di)
	args := gcli.RewriteShortFlags(os.Args, gcli.CollectFlagNames(root.Command))
	if goArgs, ok := goPassthrough(args, root.Command); ok {
		// ---- BEGIN go-compat passthrough (delete to remove) -----------------
		// Forward unknown subcommands like `gdnext get pkg` or `gdnext mod tidy`
		// straight to the underlying `go` toolchain so gdnext stays a drop-in
		// replacement for the `gd` command. Force-populate the tool catalog
		// before exec'ing; urfave's Before hook only fires inside cmd.Run.
		tools := do.MustInvoke[tooling.Catalog](di)
		if err := tools.Go.Exec(goArgs...); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
		// ---- END go-compat passthrough --------------------------------------
	}
	if err := root.Run(context.Background(), args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, "\nis this error unexpected? open an issue! https://github.com/quaadgras/graphics.gd/issues/new/choose")
		os.Exit(1)
	}
}

// goPassthrough decides whether args looks like a `go` invocation
// gdnext should forward rather than try to handle itself. Returns the
// argv to pass to `go` (verb + remaining tokens, no program name) and
// true when the answer is yes
func goPassthrough(args []string, cmd *ucli.Command) ([]string, bool) {
	if len(args) < 2 {
		return nil, false
	}
	for i := 1; i < len(args); i++ {
		tok := args[i]
		if tok == "--" {
			// Everything after `--` is positional; if we got here without
			// finding a verb, there's nothing to passthrough.
			return nil, false
		}
		if strings.HasPrefix(tok, "-") {
			// Global flag. Skip its value if it's space-separated (the
			// rewriter already turned `-goos linux` into `--goos linux`,
			// so the next token may be the value).
			if !strings.Contains(tok, "=") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				// Only consume the next token as a value when this flag
				// actually takes one. urfave knows; the simplest signal
				// available without parsing is "global flag name we
				// registered". If it's a bool flag, we'd over-consume —
				// but `gdnext` has only string/bool globals and the
				// rewriter normalises bools to `--name=true|false`.
				name := strings.TrimLeft(tok, "-")
				if eq := strings.IndexByte(name, '='); eq >= 0 {
					name = name[:eq]
				}
				if isStringFlag(cmd, name) {
					i++
				}
			}
			continue
		}
		if isKnownCommand(cmd, tok) {
			return nil, false
		}
		return args[i:], true
	}
	return nil, false
}

// isKnownCommand reports whether tok matches a registered subcommand
// (or one of its aliases) on the supplied root. urfave keeps the
// canonical list on cmd.Commands and doesn't expose a Lookup-by-name;
// the slice is tiny so a linear scan is fine.
func isKnownCommand(cmd *ucli.Command, tok string) bool {
	for _, c := range cmd.Commands {
		for _, n := range c.Names() {
			if n == tok {
				return true
			}
		}
	}
	return false
}

// isStringFlag reports whether the named global flag carries a string
// (or int) value — i.e. consumes the next argv token when given without
// `=value`. Helper for goPassthrough's flag-skipping loop.
func isStringFlag(cmd *ucli.Command, name string) bool {
	for _, f := range cmd.Flags {
		matched := false
		for _, n := range f.Names() {
			if n == name {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		switch f.(type) {
		case *ucli.StringFlag, *ucli.IntFlag:
			return true
		}
		return false
	}
	return false
}
