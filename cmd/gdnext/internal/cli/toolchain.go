package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"graphics.gd/cmd/gdnext/internal/tooling"
	"graphics.gd/product"

	"github.com/urfave/cli/v3"
)

func toolchainCmd() *cli.Command {
	return &cli.Command{
		Name:  "toolchain",
		Usage: "manage the external programs gdnext drives",
		Commands: []*cli.Command{
			{
				Name:   "list",
				Usage:  "list every toolchain gdnext can manage",
				Action: toolchainList,
			},
			{
				Name:      "path",
				Usage:     "print the absolute install path of a toolchain (lookup only, no download)",
				ArgsUsage: "<name>",
				Action:    toolchainPath,
			},
			{
				Name:      "install",
				Usage:     "install one named toolchain, or every tool required for the current target",
				ArgsUsage: "[name]",
				Action:    toolchainInstall,
			},
			{
				Name:  "doctor",
				Usage: "report toolchain status for the current host + target; --fix auto-installs missing requireds",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "fix",
						Usage: "after reporting, run `toolchain install` for the current target and re-report",
					},
				},
				Action: toolchainDoctor,
			},
		},
	}
}

func toolchainList(_ context.Context, _ *cli.Command) error {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer tw.Flush()
	fmt.Fprintln(tw, "NAME\tVERSION\tPURPOSE")
	for _, t := range tooling.Catalog {
		v := t.Version
		if v == "" {
			v = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", t.Slug, v, t.RequiredFor)
	}
	return nil
}

func toolchainPath(_ context.Context, cmd *cli.Command) error {
	if cmd.NArg() != 1 {
		return fmt.Errorf("usage: gdnext toolchain path <name>")
	}
	t := tooling.BySlug(cmd.Args().First())
	if t == nil {
		return fmt.Errorf("unknown toolchain %q (try: gdnext toolchain list)", cmd.Args().First())
	}
	path, err := t.Lookup(tooling.ModeFind)
	if err != nil {
		return err
	}
	fmt.Println(path)
	return nil
}

// toolchainInstall installs one named toolchain (when given a positional
// arg) or every tool required by the current host/target (when bare).
// Target-blind installs are intentionally not offered: the previous
// "install everything regardless" walk was a footgun that pulled
// hundreds of MB the user didn't need.
func toolchainInstall(_ context.Context, cmd *cli.Command) error {
	if cmd.NArg() == 0 {
		return installForTarget(product.ResolveEnv(os.Getenv("GOOS"), os.Getenv("GOARCH")))
	}
	t := tooling.BySlug(cmd.Args().First())
	if t == nil {
		return fmt.Errorf("unknown toolchain %q", cmd.Args().First())
	}
	path, err := t.Lookup()
	if err != nil {
		return err
	}
	fmt.Println(path)
	return nil
}

// toolchainDoctor renders the host/target report. With --fix, any
// REQUIRED tool that's missing is fed through `toolchain install` and
// the report is reprinted so the caller sees the final state.
func toolchainDoctor(_ context.Context, cmd *cli.Command) error {
	env := product.ResolveEnv(os.Getenv("GOOS"), os.Getenv("GOARCH"))
	fail := reportToolchainStatus(env)

	if cmd.Bool("fix") && fail > 0 {
		fmt.Fprintln(os.Stdout, "\n→ installing missing requirements...")
		if err := installForTarget(env); err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout)
		fail = reportToolchainStatus(env)
	}

	if fail > 0 {
		return fmt.Errorf("%d required toolchain(s) missing for %s on %s host (rerun with --fix to auto-download)",
			fail, env.TargetTuple(), env.HostTuple())
	}
	fmt.Fprintf(os.Stdout, "all required toolchains present for %s\n", env.TargetTuple())
	return nil
}

// reportToolchainStatus prints the host/target header + per-tool status
// table to stdout (ModeFind, no downloads) and returns the count of
// REQUIRED tools that are missing or unavailable. Used by doctor; also
// re-called after `--fix` runs to show the final state.
func reportToolchainStatus(env product.BuildEnv) (requiredFail int) {
	fmt.Fprintf(os.Stdout, "host:   %s\ntarget: %s\n\n", env.HostTuple(), env.TargetTuple())
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer tw.Flush()
	fmt.Fprintln(tw, "NAME\tNEEDED FOR\tRUNS ON\tSTATUS\tDETAIL")
	for _, t := range tooling.Catalog {
		required := t.IsRequiredFor(env.TargetGOOS, env.TargetGOARCH)
		available := t.IsAvailableOn(env.HostGOOS, env.HostGOARCH)
		neededFor := platformsString(t.Required)
		runsOn := platformsString(t.Available)
		if len(t.Available.GOOS) == 0 {
			runsOn = "any"
		}
		// Tool required by the target but host can't run it: hard
		// error, no point trying to install (downloads won't exist).
		if required && !available {
			fmt.Fprintf(tw, "%s\t%s\t%s\tN/A\trequired for %s but cannot run on %s host\n",
				t.Slug, neededFor, runsOn, env.TargetTuple(), env.HostTuple())
			requiredFail++
			continue
		}
		path, err := t.Lookup(tooling.ModeFind)
		switch {
		case err == nil:
			fmt.Fprintf(tw, "%s\t%s\t%s\tOK\t%s\n", t.Slug, neededFor, runsOn, path)
		case required:
			fmt.Fprintf(tw, "%s\t%s\t%s\tFAIL\t%s\n", t.Slug, neededFor, runsOn, err)
			requiredFail++
		default:
			reason := "not installed (not required for target)"
			if !available {
				reason = "not installed (not available on host)"
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\tSKIP\t%s\n", t.Slug, neededFor, runsOn, reason)
		}
	}
	return requiredFail
}

// installForTarget walks Catalog and installs every tool that is
// required by env's target AND obtainable on env's host. Prints
// `→ slug ... path` per tool. Install attempts are independent so one
// tool failing (e.g. upstream 404) doesn't abort the rest.
func installForTarget(env product.BuildEnv) error {
	for _, t := range tooling.Catalog {
		if !t.IsRequiredFor(env.TargetGOOS, env.TargetGOARCH) {
			continue
		}
		if !t.IsAvailableOn(env.HostGOOS, env.HostGOARCH) {
			fmt.Printf("→ %s ... skipped (not available on %s host)\n", t.Slug, env.HostTuple())
			continue
		}
		fmt.Printf("→ %s ... ", t.Slug)
		path, err := t.Lookup(tooling.ModeInstall)
		if err != nil {
			fmt.Println("failed:", err)
			continue
		}
		fmt.Println(path)
	}
	return nil
}

// platformsString renders a Platforms set as "goos[/goarch,goarch]" with
// multiple GOOS values comma-separated. Used by the doctor table.
func platformsString(p product.Platforms) string {
	if len(p.GOOS) == 0 {
		return "-"
	}
	os := strings.Join(p.GOOS, ",")
	if len(p.GOARCH) == 0 {
		return os
	}
	return os + "/" + strings.Join(p.GOARCH, ",")
}
