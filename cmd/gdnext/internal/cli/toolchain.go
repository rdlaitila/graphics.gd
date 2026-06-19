package cli

import (
	"context"
	"fmt"
	"os"
	"runtime"
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
				Name:  "list",
				Usage: "list every toolchain gdnext can manage",
				Action: func(_ context.Context, _ *cli.Command) error {
					tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
					defer tw.Flush()
					fmt.Fprintln(tw, "NAME\tVERSION\tPURPOSE")
					for _, e := range tooling.Entries() {
						v := e.Version
						if v == "" {
							v = "-"
						}
						fmt.Fprintf(tw, "%s\t%s\t%s\n", e.Slug, v, e.Purpose)
					}
					return nil
				},
			},
			{
				Name:      "path",
				Usage:     "print the absolute install path of a toolchain (downloads if missing)",
				ArgsUsage: "<name>",
				Action: func(_ context.Context, cmd *cli.Command) error {
					if cmd.NArg() != 1 {
						return fmt.Errorf("usage: gdnext toolchain path <name>")
					}
					e, ok := tooling.LookupBySlug(cmd.Args().First())
					if !ok {
						return fmt.Errorf("unknown toolchain %q (try: gdnext toolchain list)", cmd.Args().First())
					}
					path, err := e.Lookup()
					if err != nil {
						return err
					}
					fmt.Println(path)
					return nil
				},
			},
			{
				Name:      "install",
				Usage:     "force-install one or every toolchain by triggering its Lookup hook",
				ArgsUsage: "[name]",
				Action: func(_ context.Context, cmd *cli.Command) error {
					if cmd.NArg() == 0 {
						for _, e := range tooling.Entries() {
							fmt.Printf("→ %s ... ", e.Slug)
							path, err := e.Lookup()
							if err != nil {
								fmt.Println("failed:", err)
								continue
							}
							fmt.Println(path)
						}
						return nil
					}
					e, ok := tooling.LookupBySlug(cmd.Args().First())
					if !ok {
						return fmt.Errorf("unknown toolchain %q", cmd.Args().First())
					}
					path, err := e.Lookup()
					if err != nil {
						return err
					}
					fmt.Println(path)
					return nil
				},
			},
			{
				Name:  "doctor",
				Usage: "verify every toolchain reachable; non-zero exit only on REQUIRED misses",
				Description: "Lists every toolchain in the catalog and classifies each by whether it's\n" +
					"REQUIRED for the build target (taken from --goos/--goarch or $GOOS/$GOARCH,\n" +
					"falling back to runtime.GOOS/GOARCH) and AVAILABLE on the host.\n" +
					"\n" +
					"Statuses:\n" +
					"  OK     present and resolvable\n" +
					"  FAIL   required for this target but missing on this host (job fails)\n" +
					"  N/A    required for this target but unobtainable on this host (job fails)\n" +
					"  SKIP   not required for this target (or not available on this host)\n" +
					"\n" +
					"Examples:\n" +
					"  gdnext toolchain doctor                              # host\n" +
					"  gdnext --goos android --goarch arm64 toolchain doctor\n" +
					"  GOOS=linux GOARCH=arm64 gdnext toolchain doctor      # same, via env",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "fix",
						Usage: "auto-download any toolchain that's missing (defaults to diagnostic-only)",
					},
				},
				Action: func(_ context.Context, cmd *cli.Command) error {
					// Doctor defaults to diagnostic-only — Lookup(ModeFind)
					// skips the auto-download path entirely so a drive-by
					// `toolchain doctor` never silently fetches 100s of MB.
					// `--fix` opts back in to the install behaviour.
					mode := tooling.ModeFind
					if cmd.Bool("fix") {
						mode = tooling.ModeInstall
					}

					// (target GOOS/GOARCH, host GOOS/GOARCH) all come
					// from env vars by the time we reach the action:
					// PromoteFlagsToEnv on the root command's Before
					// hook has already mirrored --goos/--goarch into
					// $GOOS/$GOARCH. Host always reflects the binary.
					targetGOOS := os.Getenv("GOOS")
					if targetGOOS == "" {
						targetGOOS = runtime.GOOS
					}
					targetGOARCH := os.Getenv("GOARCH")
					if targetGOARCH == "" {
						targetGOARCH = runtime.GOARCH
					}
					hostGOOS, hostGOARCH := runtime.GOOS, runtime.GOARCH

					fmt.Fprintf(os.Stdout, "host:   %s\ntarget: %s\n\n",
						product.Tuple(hostGOOS, hostGOARCH),
						product.Tuple(targetGOOS, targetGOARCH))
					tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
					fmt.Fprintln(tw, "NAME\tNEEDED FOR\tRUNS ON\tSTATUS\tDETAIL")
					var requiredFail int
					var missing []string
					for _, e := range tooling.Entries() {
						required := e.IsRequiredFor(targetGOOS, targetGOARCH)
						available := e.IsAvailableOn(hostGOOS, hostGOARCH)
						neededFor := platformsString(e.Required)
						runsOn := platformsString(e.Available)
						if len(e.Available.GOOS) == 0 {
							runsOn = "any"
						}

						// Tool is required by the target but the host
						// can't run it: hard error, no point trying to
						// install (downloads won't exist).
						if required && !available {
							fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", e.Slug, neededFor, runsOn,
								"N/A", fmt.Sprintf("required for %s but cannot run on %s host",
									product.Tuple(targetGOOS, targetGOARCH),
									product.Tuple(hostGOOS, hostGOARCH)))
							requiredFail++
							missing = append(missing, e.Slug+"(unavailable)")
							continue
						}

						path, err := e.Lookup(mode)
						switch {
						case err == nil:
							fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", e.Slug, neededFor, runsOn, "OK", path)
						case required:
							fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", e.Slug, neededFor, runsOn, "FAIL", err)
							requiredFail++
							missing = append(missing, e.Slug)
						default:
							reason := "not installed (not required for target)"
							if !available {
								reason = "not installed (not available on host)"
							}
							fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", e.Slug, neededFor, runsOn, "SKIP", reason)
						}
					}
					tw.Flush()
					fmt.Fprintln(os.Stdout)
					if requiredFail > 0 {
						return fmt.Errorf("%d required toolchain(s) missing for %s on %s host: %s (rerun with --fix to auto-download, or use `gdnext toolchain install`)",
							requiredFail,
							product.Tuple(targetGOOS, targetGOARCH),
							product.Tuple(hostGOOS, hostGOARCH),
							strings.Join(missing, ", "))
					}
					fmt.Fprintf(os.Stdout, "all required toolchains present for %s\n", product.Tuple(targetGOOS, targetGOARCH))
					return nil
				},
			},
		},
	}
}

// platformsString renders a Platforms set as "goos[/goarch,goarch]" with
// multiple GOOS values comma-separated. Used by doctor's table.
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
