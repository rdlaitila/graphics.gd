package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"graphics.gd/cmd/gdnext/internal/tooling"
	"graphics.gd/product"

	"github.com/samber/do/v2"
	"graphics.gd/cmd/gdnext/internal/shared"
	"github.com/urfave/cli/v3"
)

// AndroidCommand wires `gdnext android`. Runtime state lives on
// *AndroidActions.
type AndroidCommand struct {
	*cli.Command
	Injector do.Injector `do:""`
}

// AndroidActions carries the runtime state.
type AndroidActions struct {
	ToolCatalog tooling.Catalog  `do:""`
	BuildEnv    product.BuildEnv `do:""`
}

// NewAndroidCommand constructs the android subcommand for gdnext
func NewAndroidCommand(di do.Injector) (*AndroidCommand, error) {
	t := do.MustInvokeStruct[*AndroidCommand](di)
	t.Command = &cli.Command{
		Name:  "android",
		Usage: "Android device and APK helpers",
		Commands: []*cli.Command{
			{
				Name:            "adb",
				Usage:           "raw passthrough to the bundled adb binary",
				ArgsUsage:       "[adb-args...]",
				SkipFlagParsing: true,
				Action:          shared.BindAction(t.Injector, (*AndroidActions).androidAdb),
			},
			{
				Name:      "install",
				Usage:     "adb install the supplied APK",
				ArgsUsage: "<path-to-apk>",
				Action:    shared.BindAction(t.Injector, (*AndroidActions).androidInstall),
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
				Action: shared.BindAction(t.Injector, (*AndroidActions).androidLogcat),
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
						Action:          shared.BindAction(t.Injector, (*AndroidActions).androidApkSign),
					},
					{
						Name:      "verify",
						Usage:     "apksigner verify",
						ArgsUsage: "<apk>",
						Action:    shared.BindAction(t.Injector, (*AndroidActions).androidApkVerify),
					},
					{
						Name:      "packagename",
						Usage:     "print the package name of an apk via aapt2 dump packagename",
						ArgsUsage: "<apk>",
						Action:    shared.BindAction(t.Injector, (*AndroidActions).androidApkPackagename),
					},
				},
			},
			{
				Name:  "keystore",
				Usage: "manage the debug keystore (TODO: extract from builder.Android)",
				Commands: []*cli.Command{
					{
						Name:   "show",
						Usage:  "print the debug.keystore path that gdnext build/run uses",
						Action: shared.BindAction(t.Injector, (*AndroidActions).androidKeystoreShow),
					},
				},
			},
		},
	}
	return t, nil
}

// NewAndroidActions resolves the runtime state for android.
func NewAndroidActions(di do.Injector) (*AndroidActions, error) {
	return do.InvokeStruct[*AndroidActions](di)
}

// androidAdb is the action handler for the `gdnext android adb` subcommand
func (t *AndroidActions) androidAdb(_ context.Context, cmd *cli.Command) error {
	return t.ToolCatalog.AndroidDebugBridge.Exec(cmd.Args().Slice()...)
}

// androidInstall is the action handler for the `gdnext android install` subcommand
func (t *AndroidActions) androidInstall(_ context.Context, cmd *cli.Command) error {
	if cmd.NArg() != 1 {
		return fmt.Errorf("usage: gdnext android install <apk>")
	}
	return t.ToolCatalog.AndroidDebugBridge.Exec("install", cmd.Args().First())
}

// androidLogcat is the action handler for the `gdnext android logcat` subcommand
func (t *AndroidActions) androidLogcat(_ context.Context, cmd *cli.Command) error {
	adb, err := t.ToolCatalog.AndroidDebugBridge.Lookup()
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
}

// androidApkSign is the action handler for the `gdnext android apk sign` subcommand
func (t *AndroidActions) androidApkSign(_ context.Context, cmd *cli.Command) error {
	if cmd.NArg() == 0 {
		return fmt.Errorf("usage: gdnext android apk sign <apk> [flags]")
	}
	args := append([]string{"sign"}, cmd.Args().Slice()...)
	return t.ToolCatalog.AndroidPackageSigner.Exec(args...)
}

// androidApkVerify is the action handler for the `gdnext android apk verify` subcommand
func (t *AndroidActions) androidApkVerify(_ context.Context, cmd *cli.Command) error {
	if cmd.NArg() != 1 {
		return fmt.Errorf("usage: gdnext android apk verify <apk>")
	}
	return t.ToolCatalog.AndroidPackageSigner.Exec("verify", cmd.Args().First())
}

// androidApkPackagename is the action handler for the `gdnext android apk packagename` subcommand
func (t *AndroidActions) androidApkPackagename(_ context.Context, cmd *cli.Command) error {
	if cmd.NArg() != 1 {
		return fmt.Errorf("usage: gdnext android apk packagename <apk>")
	}
	return t.ToolCatalog.AndroidAssetPackagingTool.Exec("dump", "packagename", cmd.Args().First())
}

// androidKeystoreShow is the action handler for the `gdnext android keystore show` subcommand
func (t *AndroidActions) androidKeystoreShow(_ context.Context, _ *cli.Command) error {
	p, err := androidKeystorePath(t.BuildEnv.Host)
	if err != nil {
		return err
	}
	fmt.Println(p)
	return nil
}

// androidKeystorePath returns the platform-specific path the legacy gd
// command uses for the auto-generated debug.keystore (see the matching
// switch in cmd/gdnext/internal/builder/android.go).
func androidKeystorePath(host product.BuildHost) (string, error) {
	var godot string
	switch host.GOOS {
	case product.GOOSLinux:
		godot = "godot"
	case product.GOOSWindows, product.GOOSDarwin:
		godot = "Godot"
	default:
		return "", fmt.Errorf("no known keystore path for %s", host.GOOS)
	}
	return filepath.Join(host.UserAppdataRoot, godot, "keystores", "debug.keystore"), nil
}
