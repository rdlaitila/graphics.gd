package cli

import (
	"context"
	"fmt"
	"os"
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
	Lookup  func(...tooling.Mode) (string, error)
}

func toolchainCatalog() []toolchainEntry {
	return []toolchainEntry{
		{"godot", tooling.Godot.Version, tooling.Godot.RequiredFor, tooling.Godot.Lookup},
		{"go", tooling.Go.Version, tooling.Go.RequiredFor, tooling.Go.Lookup},
		{"zig", tooling.Zig.Version, tooling.Zig.RequiredFor, tooling.Zig.Lookup},
		{"llvm", tooling.LLVM.Version, tooling.LLVM.RequiredFor, tooling.LLVM.Lookup},
		{"adb", tooling.AndroidDebugBridge.Version, tooling.AndroidDebugBridge.RequiredFor, tooling.AndroidDebugBridge.Lookup},
		{"apksigner", tooling.AndroidPackageSigner.Version, tooling.AndroidPackageSigner.RequiredFor, tooling.AndroidPackageSigner.Lookup},
		{"aapt2", tooling.AndroidAssetPackagingTool.Version, tooling.AndroidAssetPackagingTool.RequiredFor, tooling.AndroidAssetPackagingTool.Lookup},
		{"apktool", tooling.AndroidPackageKitTool.Version, tooling.AndroidPackageKitTool.RequiredFor, tooling.AndroidPackageKitTool.Lookup},
		{"bundletool", tooling.BundleTool.Version, tooling.BundleTool.RequiredFor, tooling.BundleTool.Lookup},
		{"android.jar", tooling.Android.Version, tooling.Android.RequiredFor, tooling.Android.Lookup},
		{"upx", tooling.UltimatePackerForExecutables.Version, tooling.UltimatePackerForExecutables.RequiredFor, tooling.UltimatePackerForExecutables.Lookup},
		{"vpk", tooling.Velopack.Version, tooling.Velopack.RequiredFor, tooling.Velopack.Lookup},
		{"libgodot", tooling.LibGodot.Version, tooling.LibGodot.RequiredFor, tooling.LibGodot.Lookup},
		{"libgodot-editor", tooling.LibGodotEditor.Version, tooling.LibGodotEditor.RequiredFor, tooling.LibGodotEditor.Lookup},
		{"ldd", tooling.ListDynamicDependencies.Version, tooling.ListDynamicDependencies.RequiredFor, tooling.ListDynamicDependencies.Lookup},
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
				Usage: "verify every toolchain is reachable; non-zero exit on any failure",
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
					tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
					defer tw.Flush()
					fmt.Fprintln(tw, "NAME\tSTATUS\tDETAIL")
					var failed bool
					for _, e := range toolchainCatalog() {
						path, err := e.Lookup(mode)
						if err != nil {
							fmt.Fprintf(tw, "%s\tFAIL\t%s\n", e.Name, err)
							failed = true
							continue
						}
						fmt.Fprintf(tw, "%s\tOK\t%s\n", e.Name, path)
					}
					if failed {
						return fmt.Errorf("one or more toolchains could not be resolved (rerun with --fix to auto-download, or use `gdnext toolchain install`)")
					}
					return nil
				},
			},
		},
	}
}
