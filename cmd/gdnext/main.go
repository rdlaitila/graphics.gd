// The 'gdnext' command is a next-generation drop-in replacement for the 'gd'
// command, built on urfave/cli/v3. It exposes every operation the legacy 'gd'
// command performs (build, run, test, export, editor launch, go passthrough)
// as a self-documenting subcommand tree, plus first-class verbs for the
// previously buried subsystems (toolchain management, keystore, APK ops,
// macOS lipo/codesign, web serve, musl setup, project init, fix).
//
// See docs/plans/2026-gdnext-cli.md for the design rationale. Every
// subcommand and the global flag set live in cmd/gdnext/internal/cli;
// this file only assembles the root command tree and runs the urfave
// dispatcher with a Go-style flag rewrite preprocessor.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	gdcli "graphics.gd/cmd/gdnext/internal/cli"
	"graphics.gd/cmd/gdnext/internal/tooling"

	"github.com/urfave/cli/v3"
)

func main() {
	cmd := &cli.Command{
		Name:                  "gdnext",
		Usage:                 "Drop-in replacement for the go command for Godot-based projects",
		Version:               gdcli.Version(),
		Suggest:               true,
		EnableShellCompletion: true,
		Flags:                 gdcli.Global(),
		Before:                gdcli.PromoteFlagsToEnv,
		Action:                gdcli.LaunchEditor,
		CommandNotFound:       passthroughToGo,
		Commands:              gdcli.Commands(),
	}
	args := gdcli.RewriteShortFlags(os.Args, gdcli.CollectFlagNames(cmd))
	if goArgs, ok := goPassthrough(args, cmd); ok {
		// ---- BEGIN go-compat passthrough (delete to remove) -----------------
		// Forward unknown subcommands like `gdnext get pkg` or `gdnext mod tidy`
		// straight to the underlying `go` toolchain so gdnext stays a drop-in
		// replacement for the `gd` command. urfave's own CommandNotFound hook
		// can't help here because the root command has a default Action (the
		// editor launcher), so any unmatched first positional falls into that
		// action instead. We sniff for the case up front and short-circuit.
		if err := tooling.Go.Exec(goArgs...); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
		// ---- END go-compat passthrough --------------------------------------
	}
	if err := cmd.Run(context.Background(), args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, "\nis this error unexpected? open an issue! https://github.com/quaadgras/graphics.gd/issues/new/choose")
		os.Exit(1)
	}
}

// goPassthrough decides whether args looks like a `go` invocation
// gdnext should forward rather than try to handle itself. Returns the
// argv to pass to `go` (verb + remaining tokens, no program name) and
// true when the answer is yes.
//
// The heuristic: walk args after the program name skipping any leading
// global flags. The first non-flag token is the candidate verb. If it
// matches a registered gdnext command we hand off to urfave; otherwise
// the user typed something like `gdnext get pkg` and we treat the whole
// suffix as a `go` invocation.
//
// Wholly cosmetic: leaving this in `main.go` (not the cli package) makes
// the entire go-compat shim a single self-contained function that's
// trivial to delete.
func goPassthrough(args []string, cmd *cli.Command) ([]string, bool) {
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
//
// urfave's auto-injected `help` / `h` verb is NOT in cmd.Commands at
// argv-parse time, so it isn't recognised here. That's deliberate:
// `gdnext help vet` should forward to `go help vet` rather than print
// "no such gdnext subcommand". The price is that bare `gdnext help`
// also forwards to `go help`; users wanting our top-level help should
// use `gdnext --help` (and per-verb help is `gdnext <verb> --help`).
func isKnownCommand(cmd *cli.Command, tok string) bool {
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
func isStringFlag(cmd *cli.Command, name string) bool {
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
		case *cli.StringFlag, *cli.IntFlag:
			return true
		}
		return false
	}
	return false
}

// passthroughToGo remains the urfave CommandNotFound handler for the rare
// case where execution reaches urfave with a verb we didn't catch in
// goPassthrough — defensive backup, not the primary path.
func passthroughToGo(_ context.Context, cmd *cli.Command, name string) {
	if name == "" {
		return
	}
	args := append([]string{name}, cmd.Args().Slice()...)
	if err := tooling.Go.Exec(args...); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
