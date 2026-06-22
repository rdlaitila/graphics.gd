// flags.go owns the global flag set, the Go-style "-foo" → "--foo"
// preprocessor that lets gdnext keep the urfave/cli/v3 parser AND the
// stdlib `flag`-style ergonomics graphics.gd users already have muscle
// memory for, and a Before-hook helper that promotes any explicitly-set
// flag back into the env vars its Sources listed.
package cli

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"graphics.gd/product"

	"github.com/urfave/cli/v3"
)

// flags returns the flag slice attached to the root command. Every flag
// is bound to one or more environment variables so scripts that already
// export GOOS / GOARCH / CC / CGO_ENABLED / GDPATH / RUNNING_INSIDE_GODOT
// continue to work without modification.
func flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:    "goos",
			Usage:   "target operating system (" + strings.Join(product.PlatformGOOSes(), ", ") + ")",
			Sources: cli.EnvVars("GOOS"),
		},
		&cli.StringFlag{
			Name:    "goarch",
			Usage:   "target architecture (" + strings.Join(product.PlatformGOARCHes(), ", ") + ")",
			Sources: cli.EnvVars("GOARCH"),
		},
		&cli.StringFlag{
			Name:    "link",
			Aliases: []string{"linkmode"},
			Usage:   "linking recipe (" + strings.Join(product.LinkModeMatrix, ", ") + "); defaults per-target",
			Sources: cli.EnvVars("GOLINK"),
		},
		&cli.StringFlag{
			Name:    "cc",
			Usage:   "C compiler used by cgo (auto-detected: zig cc / clang)",
			Sources: cli.EnvVars("CC"),
		},
		&cli.StringFlag{
			Name:    "cgo",
			Usage:   "CGO_ENABLED value forwarded to the go toolchain",
			Sources: cli.EnvVars("CGO_ENABLED"),
		},
		&cli.StringFlag{
			Name:    "gdpath",
			Usage:   "directory used to cache downloaded toolchains (default ~/gd)",
			Sources: cli.EnvVars("GDPATH"),
		},
		&cli.BoolFlag{
			Name:    "inside-godot",
			Hidden:  true,
			Usage:   "set when gdnext is launched from inside the Godot editor",
			Sources: cli.EnvVars("RUNNING_INSIDE_GODOT"),
		},
		&cli.BoolFlag{
			Name:    "verbose",
			Aliases: []string{"V"},
			Usage:   "print extra diagnostic output",
			Sources: cli.EnvVars("GD_VERBOSE"),
		},
	}
}

// promoteFlagsToEnv is the recommended Before hook for the root command.
// It walks every flag on the supplied cli.Command and, for any flag the
// user explicitly set on the command line, mirrors the resolved value back
// into the first env-var listed in that flag's Sources.
//
// This bridges the urfave/cli world (where flags shadow env vars in one
// direction only) into the legacy code that reads env vars directly. Once
// every downstream package is migrated to read flag values passed from the
// call chain, it can be safely dropped.
//
// An unrecognised flag type that DOES bind env-var Sources is a hard
// error: silently dropping the env-var update would leave downstream code
// observing stale state. Add the new flag type to envKey / valueOf below.
func promoteFlagsToEnv(_ context.Context, cmd *cli.Command) (context.Context, error) {
	for _, f := range cmd.Flags {
		env, ok, err := envKey(f)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue // no env-var source, nothing to promote.
		}
		name := f.Names()[0]
		if !cmd.IsSet(name) {
			continue
		}
		v, err := valueOf(cmd, f, name)
		if err != nil {
			return nil, err
		}
		os.Setenv(env, v)
	}
	return nil, nil
}

// envKey returns (firstEnvVarName, true, nil) when the flag binds at least
// one env var, (anything, false, nil) when it binds none, and a non-nil
// error when the flag type isn't covered by the switch. The error
// distinguishes "no env sources to promote" (safe to skip) from "we don't
// know how to introspect this flag's env sources" (silent bug risk).
func envKey(f cli.Flag) (string, bool, error) {
	var keys []string
	switch t := f.(type) {
	case *cli.StringFlag:
		keys = t.Sources.EnvKeys()
	case *cli.BoolFlag:
		keys = t.Sources.EnvKeys()
	case *cli.IntFlag:
		keys = t.Sources.EnvKeys()
	default:
		return "", false, fmt.Errorf("cli.PromoteFlagsToEnv: unsupported flag type %T for --%s; add a case to envKey/valueOf in cmd/gdnext/internal/cli/flags.go", f, f.Names()[0])
	}
	if len(keys) == 0 {
		return "", false, nil
	}
	return keys[0], true, nil
}

// valueOf renders the flag's resolved value as a string regardless of the
// underlying urfave flag type. Bool flags become "1" / "" so downstream
// env consumers can do the usual non-empty-truthy check.
//
// An empty string is a *valid* promoted value — `--cgo=0` should clear
// CGO_ENABLED via the legacy `os.Getenv("CGO_ENABLED") != ""` pattern,
// and an explicit `--verbose=false` should likewise clear GD_VERBOSE.
// PromoteFlagsToEnv writes the result unconditionally so both of those
// hold; do not add a `v == ""` short-circuit there.
//
// Returning an error here is unreachable in practice — envKey rejects
// unknown types first — but keeping the same signature makes it impossible
// to add a new case to one helper and forget the other.
func valueOf(cmd *cli.Command, f cli.Flag, name string) (string, error) {
	switch f.(type) {
	case *cli.StringFlag:
		return cmd.String(name), nil
	case *cli.BoolFlag:
		if cmd.Bool(name) {
			return "1", nil
		}
		return "", nil
	case *cli.IntFlag:
		return strconv.Itoa(cmd.Int(name)), nil
	default:
		return "", fmt.Errorf("cli.PromoteFlagsToEnv: unsupported flag type %T for --%s; add a case to valueOf in cmd/gdnext/internal/cli/flags.go", f, name)
	}
}

// RewriteShortFlags walks args and returns a copy where every "-foo" /
// "-foo=bar" token whose name is in the known set is rewritten to "--foo" /
// "--foo=bar". The known set is built by CollectFlagNames over the assembled
// command tree.
//
// Rules:
//   - args[0] (program name) is never touched.
//   - "--" terminates rewriting (standard argv convention) — everything
//     after it is forwarded verbatim.
//   - Tokens already starting with "--" pass through unchanged.
//   - Single-letter "-x" tokens pass through unchanged so urfave's POSIX
//     short-flag handling and aliases keep working.
//   - "-foo" or "-foo=bar" is rewritten only when "foo" is in known.
//     Unknown multi-character single-dash tokens are left alone so go
//     passthrough subcommands ("gdnext mod tidy -mod vendor") forward
//     correctly.
func RewriteShortFlags(args []string, known map[string]struct{}) []string {
	if len(args) == 0 {
		return args
	}
	out := make([]string, 0, len(args))
	out = append(out, args[0])
	for i := 1; i < len(args); i++ {
		tok := args[i]
		if tok == "--" {
			out = append(out, args[i:]...)
			break
		}
		if strings.HasPrefix(tok, "--") {
			out = append(out, tok)
			continue
		}
		if !strings.HasPrefix(tok, "-") || len(tok) < 3 {
			out = append(out, tok)
			continue
		}
		body := tok[1:]
		name := body
		if eq := strings.IndexByte(body, '='); eq >= 0 {
			name = body[:eq]
		}
		if _, ok := known[name]; ok {
			out = append(out, "-"+tok)
			continue
		}
		out = append(out, tok)
	}
	return out
}

// CollectFlagNames walks the cli command tree and returns the set of every
// long-flag name registered on the root command and any subcommand. Single-
// letter aliases are excluded so RewriteShortFlags never accidentally
// converts "-h" into "--h".
func CollectFlagNames(cmd *cli.Command) map[string]struct{} {
	known := make(map[string]struct{})
	var walk func(*cli.Command)
	walk = func(c *cli.Command) {
		for _, f := range c.Flags {
			for _, n := range f.Names() {
				if len(n) > 1 {
					known[n] = struct{}{}
				}
			}
		}
		for _, sub := range c.Commands {
			walk(sub)
		}
	}
	walk(cmd)
	return known
}
