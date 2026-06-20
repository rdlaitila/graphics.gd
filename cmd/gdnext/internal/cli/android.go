package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"graphics.gd/cmd/gdnext/internal/tooling"

	"github.com/urfave/cli/v3"
)

func androidCmd() *cli.Command {
	return &cli.Command{
		Name:  "android",
		Usage: "Android device and APK helpers",
		Commands: []*cli.Command{
			{
				Name:            "adb",
				Usage:           "raw passthrough to the bundled adb binary",
				ArgsUsage:       "[adb-args...]",
				SkipFlagParsing: true,
				Action: func(_ context.Context, cmd *cli.Command) error {
					return tooling.AndroidDebugBridge.Exec(cmd.Args().Slice()...)
				},
			},
			{
				Name:      "install",
				Usage:     "adb install the supplied APK",
				ArgsUsage: "<path-to-apk>",
				Action: func(_ context.Context, cmd *cli.Command) error {
					if cmd.NArg() != 1 {
						return fmt.Errorf("usage: gdnext android install <apk>")
					}
					return tooling.AndroidDebugBridge.Exec("install", cmd.Args().First())
				},
			},
			{
				Name:  "logcat",
				Usage: "stream logcat from the connected device",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "package",
						Usage: "filter to the pid of the named package (uses adb shell pidof)",
					},
				},
				Action: func(_ context.Context, cmd *cli.Command) error {
					adb, err := tooling.AndroidDebugBridge.Lookup()
					if err != nil {
						return err
					}
					args := []string{"logcat"}
					if pkg := cmd.String("package"); pkg != "" {
						out, err := exec.Command(adb, "shell", "pidof", pkg).Output()
						if err != nil {
							return fmt.Errorf("could not resolve pid of %s: %w", pkg, err)
						}
						args = append(args, "--pid="+string(out))
					}
					c := exec.Command(adb, args...)
					c.Stdout = os.Stdout
					c.Stderr = os.Stderr
					c.Stdin = os.Stdin
					return c.Run()
				},
			},
			{
				Name:  "apk",
				Usage: "inspect and modify .apk artefacts",
				Commands: []*cli.Command{
					{
						Name:            "sign",
						Usage:           "apksigner v1 + v2 sign of the supplied apk",
						ArgsUsage:       "<apk> [extra apksigner flags]",
						SkipFlagParsing: true,
						Action: func(_ context.Context, cmd *cli.Command) error {
							if cmd.NArg() == 0 {
								return fmt.Errorf("usage: gdnext android apk sign <apk> [flags]")
							}
							args := append([]string{"sign"}, cmd.Args().Slice()...)
							return tooling.AndroidPackageSigner.Exec(args...)
						},
					},
					{
						Name:      "verify",
						Usage:     "apksigner verify",
						ArgsUsage: "<apk>",
						Action: func(_ context.Context, cmd *cli.Command) error {
							if cmd.NArg() != 1 {
								return fmt.Errorf("usage: gdnext android apk verify <apk>")
							}
							return tooling.AndroidPackageSigner.Exec("verify", cmd.Args().First())
						},
					},
					{
						Name:      "packagename",
						Usage:     "print the package name of an apk via aapt2 dump packagename",
						ArgsUsage: "<apk>",
						Action: func(_ context.Context, cmd *cli.Command) error {
							if cmd.NArg() != 1 {
								return fmt.Errorf("usage: gdnext android apk packagename <apk>")
							}
							return tooling.AndroidAssetPackagingTool.Exec("dump", "packagename", cmd.Args().First())
						},
					},
				},
			},
			{
				Name:  "keystore",
				Usage: "manage the debug keystore (TODO: extract from builder.Android)",
				Commands: []*cli.Command{
					{
						Name:  "show",
						Usage: "print the debug.keystore path that gdnext build/run uses",
						Action: func(_ context.Context, _ *cli.Command) error {
							p, err := androidKeystorePath()
							if err != nil {
								return err
							}
							fmt.Println(p)
							return nil
						},
					},
				},
			},
		},
	}
}

// androidKeystorePath returns the platform-specific path the legacy gd
// command uses for the auto-generated debug.keystore (see the matching
// switch in cmd/gdnext/internal/builder/android.go).
func androidKeystorePath() (string, error) {
	switch runtime.GOOS {
	case "linux":
		return filepath.Join(os.Getenv("HOME"), ".local", "share", "godot", "keystores", "debug.keystore"), nil
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "Godot", "keystores", "debug.keystore"), nil
	case "darwin":
		return filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "Godot", "keystores", "debug.keystore"), nil
	default:
		return "", fmt.Errorf("no known keystore path for %s", runtime.GOOS)
	}
}
