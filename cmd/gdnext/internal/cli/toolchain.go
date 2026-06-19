package cli

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"text/tabwriter"

	"graphics.gd/cmd/gdnext/internal/tooling"

	"github.com/urfave/cli/v3"
)

// toolchainEntry pairs a user-visible name with a snapshot of the toolchain
// metadata. Storing function values rather than receiver-bound expressions
// lets us use the catalog from any goroutine without aliasing concerns.
type toolchainEntry struct {
	Name    string
	Version string
	Purpose string
	Targets []string
	Lookup  func(...tooling.Mode) (string, error)
}

// IsRequiredFor reports whether this entry is needed when targeting any
// of the supplied GOOS values. Mirrors tooling.toolchain.IsRequiredFor;
// the duplication is here because toolchainEntry is the exported surface
// the cli package uses.
func (e toolchainEntry) IsRequiredFor(goos ...string) bool {
	for _, t := range e.Targets {
		if t == "all" {
			return true
		}
		for _, g := range goos {
			if t == g {
				return true
			}
		}
	}
	return false
}

func toolchainCatalog() []toolchainEntry {
	return []toolchainEntry{
		{"godot", tooling.Godot.Version, tooling.Godot.RequiredFor, tooling.Godot.Targets, tooling.Godot.Lookup},
		{"go", tooling.Go.Version, tooling.Go.RequiredFor, tooling.Go.Targets, tooling.Go.Lookup},
		{"zig", tooling.Zig.Version, tooling.Zig.RequiredFor, tooling.Zig.Targets, tooling.Zig.Lookup},
		{"llvm", tooling.LLVM.Version, tooling.LLVM.RequiredFor, tooling.LLVM.Targets, tooling.LLVM.Lookup},
		{"adb", tooling.AndroidDebugBridge.Version, tooling.AndroidDebugBridge.RequiredFor, tooling.AndroidDebugBridge.Targets, tooling.AndroidDebugBridge.Lookup},
		{"apksigner", tooling.AndroidPackageSigner.Version, tooling.AndroidPackageSigner.RequiredFor, tooling.AndroidPackageSigner.Targets, tooling.AndroidPackageSigner.Lookup},
		{"aapt2", tooling.AndroidAssetPackagingTool.Version, tooling.AndroidAssetPackagingTool.RequiredFor, tooling.AndroidAssetPackagingTool.Targets, tooling.AndroidAssetPackagingTool.Lookup},
		{"apktool", tooling.AndroidPackageKitTool.Version, tooling.AndroidPackageKitTool.RequiredFor, tooling.AndroidPackageKitTool.Targets, tooling.AndroidPackageKitTool.Lookup},
		{"bundletool", tooling.BundleTool.Version, tooling.BundleTool.RequiredFor, tooling.BundleTool.Targets, tooling.BundleTool.Lookup},
		{"android.jar", tooling.Android.Version, tooling.Android.RequiredFor, tooling.Android.Targets, tooling.Android.Lookup},
		{"upx", tooling.UltimatePackerForExecutables.Version, tooling.UltimatePackerForExecutables.RequiredFor, tooling.UltimatePackerForExecutables.Targets, tooling.UltimatePackerForExecutables.Lookup},
		{"vpk", tooling.Velopack.Version, tooling.Velopack.RequiredFor, tooling.Velopack.Targets, tooling.Velopack.Lookup},
		{"libgodot", tooling.LibGodot.Version, tooling.LibGodot.RequiredFor, tooling.LibGodot.Targets, tooling.LibGodot.Lookup},
		{"libgodot-editor", tooling.LibGodotEditor.Version, tooling.LibGodotEditor.RequiredFor, tooling.LibGodotEditor.Targets, tooling.LibGodotEditor.Lookup},
		{"ldd", tooling.ListDynamicDependencies.Version, tooling.ListDynamicDependencies.RequiredFor, tooling.ListDynamicDependencies.Targets, tooling.ListDynamicDependencies.Lookup},
	}
}

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
					for _, e := range toolchainCatalog() {
						v := e.Version
						if v == "" {
							v = "-"
						}
						fmt.Fprintf(tw, "%s\t%s\t%s\n", e.Name, v, e.Purpose)
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
					name := cmd.Args().First()
					for _, e := range toolchainCatalog() {
						if e.Name == name {
							path, err := e.Lookup()
							if err != nil {
								return err
							}
							fmt.Println(path)
							return nil
						}
					}
					return fmt.Errorf("unknown toolchain %q (try: gdnext toolchain list)", name)
				},
			},
			{
				Name:      "install",
				Usage:     "force-install one or every toolchain by triggering its Lookup hook",
				ArgsUsage: "[name]",
				Action: func(_ context.Context, cmd *cli.Command) error {
					if cmd.NArg() == 0 {
						for _, e := range toolchainCatalog() {
							fmt.Printf("→ %s ... ", e.Name)
							path, err := e.Lookup()
							if err != nil {
								fmt.Println("failed:", err)
								continue
							}
							fmt.Println(path)
						}
						return nil
					}
					name := cmd.Args().First()
					for _, e := range toolchainCatalog() {
						if e.Name == name {
							path, err := e.Lookup()
							if err != nil {
								return err
							}
							fmt.Println(path)
							return nil
						}
					}
					return fmt.Errorf("unknown toolchain %q", name)
				},
			},
			{
				Name:  "doctor",
				Usage: "verify every toolchain reachable; non-zero exit only on REQUIRED misses",
				Description: "Lists every toolchain in the catalog and marks each as REQUIRED or OPTIONAL\n" +
					"for the current target GOOS (taken from --goos or $GOOS, defaulting to the\n" +
					"host's runtime.GOOS).\n" +
					"Examples:\n" +
					"  gdnext toolchain doctor                       # what's needed for this host\n" +
					"  gdnext --goos android toolchain doctor        # what's needed to build for android\n" +
					"  GOOS=ios gdnext toolchain doctor              # same, via the env var",
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

					// Target GOOS comes from --goos / $GOOS (both handled
					// by PromoteFlagsToEnv on the root command's Before
					// hook, so by the time we get here os.Getenv is
					// authoritative). Falls back to the host runtime.GOOS.
					target := os.Getenv("GOOS")
					if target == "" {
						target = runtime.GOOS
					}

					fmt.Fprintf(os.Stdout, "checking toolchains for target: %s\n\n", target)
					tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
					fmt.Fprintln(tw, "NAME\tFOR\tSTATUS\tDETAIL")
					var requiredFail int
					var missing []string
					for _, e := range toolchainCatalog() {
						required := e.IsRequiredFor(target)
						for_ := strings.Join(e.Targets, ",")
						if for_ == "" {
							for_ = "-"
						}
						path, err := e.Lookup(mode)
						switch {
						case err == nil:
							fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", e.Name, for_, "OK", path)
						case required:
							fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", e.Name, for_, "FAIL", err)
							requiredFail++
							missing = append(missing, e.Name)
						default:
							fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", e.Name, for_, "SKIP", "not installed (not required for target)")
						}
					}
					tw.Flush()
					fmt.Fprintln(os.Stdout)
					if requiredFail > 0 {
						return fmt.Errorf("%d required toolchain(s) missing for target %s: %s (rerun with --fix to auto-download, or use `gdnext toolchain install`)",
							requiredFail, target, strings.Join(missing, ", "))
					}
					fmt.Fprintf(os.Stdout, "all required toolchains present for target %s\n", target)
					return nil
				},
			},
		},
	}
}
