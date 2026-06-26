package ci

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"graphics.gd/cmd/gdnext/internal/shared"
	"graphics.gd/product"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
)

// BuildTargetCommand wires `gdnext ci build-target`. Runtime state
// lives on *BuildTargetActions.
type BuildTargetCommand struct {
	*cli.Command
	Injector do.Injector `do:""`
}

// BuildTargetActions carries the runtime state.
type BuildTargetActions struct{}

// NewBuildTargetCommand constructs the build-target subcommand.
func NewBuildTargetCommand(di do.Injector) (*BuildTargetCommand, error) {
	t := do.MustInvokeStruct[*BuildTargetCommand](di)
	t.Command = &cli.Command{
		Name:  "build-target",
		Usage: "run `gdnext build` for one (GOOS, GOARCH) and assert artefacts",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "goos", Usage: "target GOOS (or alias)", Required: true},
			&cli.StringFlag{Name: "goarch", Usage: "target GOARCH", Required: true},
			&cli.StringFlag{Name: "link", Usage: "link mode (gdextension|libgodot); blank = platform default"},
			&cli.StringFlag{Name: "scratch", Usage: "staged example directory to build in", Required: true},
		},
		Action: shared.BindAction(t.Injector, (*BuildTargetActions).action),
	}
	return t, nil
}

// NewBuildTargetActions resolves the runtime state.
func NewBuildTargetActions(di do.Injector) (*BuildTargetActions, error) {
	return do.InvokeStruct[*BuildTargetActions](di)
}

func (t *BuildTargetActions) action(ctx context.Context, cmd *cli.Command) error {
	goos := cmd.String("goos")
	goarch := cmd.String("goarch")
	link := cmd.String("link")
	scratchArg := cmd.String("scratch")
	platform, ok := product.FindPlatformByTargetEnv(goos, goarch)
	if !ok {
		if p, ok := product.FindPlatformByName(goos); ok && (goarch == "" || p.GOARCH == goarch) {
			platform = p
			goos, goarch = p.GOOS, p.GOARCH
		} else {
			return fmt.Errorf("unknown platform %s (try `gdnext platforms` for the matrix)",
				product.Tuple(goos, goarch))
		}
	}
	if !platform.Kind.Has(product.Target) {
		return fmt.Errorf("%s is registered but not a build target", platform.Tuple())
	}
	scratch, err := filepath.Abs(scratchArg)
	if err != nil {
		return err
	}
	if st, err := os.Stat(scratch); err != nil || !st.IsDir() {
		return fmt.Errorf("scratch dir does not exist: %s", scratch)
	}
	mode, err := product.ParseLinkMode(link)
	if err != nil {
		return err
	}
	if mode == 0 {
		mode = product.GDExtension
	}
	// Actual build. Stdin closed so the optional AAB-signing
	// `Provide passphrase:` prompt reads EOF immediately rather
	// than consuming the next CI step's output.
	args := []string{"-goos", goos, "-goarch", goarch}
	if link != "" {
		args = append(args, "-link", link)
	}
	args = append(args, "build")
	// Android: bake the play-bot in for CI artefacts only. The
	// `playbot` build tag flips canarybird/playenv_android_*.go
	// from the user variant (no bot, runs the game on install) to
	// the bot variant (auto-attaches and pipes the report to
	// logcat). Local `gdnext build` deliberately omits the tag so
	// `gdnext android install` produces a normal user APK.
	if platform.GOOS == product.GOOSAndroid {
		args = append(args, "--", "-tags", "playbot")
	}
	if err := runIn(scratch, "gdnext", args...); err != nil {
		return err
	}
	if err := assertSharedLibrary(scratch, platform, mode); err != nil {
		return err
	}
	if err := assertDistributable(scratch, platform, mode); err != nil {
		return err
	}
	if platform.GOOS == product.GOOSAndroid {
		if err := signAndVerifyApk(scratch, platform); err != nil {
			return err
		}
	}
	return nil
}

// assertSharedLibrary verifies the per-target shared library `gdnext
// build` writes under graphics/. Skipped for libgodot, which produces
// a single statically-linked executable instead (checked by
// assertDistributable).
func assertSharedLibrary(scratch string, plat product.Platform, mode product.LinkMode) error {
	if mode.Has(product.LibGodot) {
		return nil
	}
	graphics := filepath.Join(scratch, "graphics")
	switch plat.GOOS {
	case product.GOOSLinux:
		return mustExist(filepath.Join(graphics, "linux_"+plat.GOARCH+".so"))
	case product.GOOSWindows:
		return mustExist(filepath.Join(graphics, "windows_"+plat.GOARCH+".dll"))
	case product.GOOSDarwin:
		if err := mustExist(filepath.Join(graphics, "darwin_"+plat.GOARCH+".dylib")); err != nil {
			return err
		}
		return mustExist(filepath.Join(graphics, "darwin_universal.dylib"))
	case product.GOOSJS:
		return mustExist(filepath.Join(scratch, "releases", "js", "wasm", "library.wasm"))
	case product.GOOSAndroid:
		return nil // apk landed under releases/android/<arch>/; checked below
	default:
		return fmt.Errorf("no shared-library assertion for %s", plat.Tuple())
	}
}

// assertDistributable verifies the export bundle landed at the path
// gdnext build writes it to. libgodot collapses every linux target
// into a single static binary under releases/linux/<arch>/.
func assertDistributable(scratch string, plat product.Platform, mode product.LinkMode) error {
	releases := filepath.Join(scratch, "releases")
	if mode.Has(product.LibGodot) {
		return assertDirNonEmpty(filepath.Join(releases, "linux", plat.GOARCH))
	}
	switch plat.GOOS {
	case product.GOOSLinux, product.GOOSWindows, product.GOOSAndroid:
		return assertDirNonEmpty(filepath.Join(releases, plat.GOOS, plat.GOARCH))
	case product.GOOSDarwin:
		// macOS exports a universal .app regardless of -goarch.
		return assertDirNonEmpty(filepath.Join(releases, "darwin", "universal"))
	case product.GOOSJS:
		return nil // already covered by the wasm shared-library check
	default:
		return fmt.Errorf("no distributable assertion for %s", plat.Tuple())
	}
}

// signAndVerifyApk runs the android-specific keystore + apksigner
// pipeline after `gdnext build` produces an unsigned apk.
func signAndVerifyApk(scratch string, plat product.Platform) error {
	pattern := filepath.Join(scratch, "releases", "android", plat.GOARCH, "*.apk")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		return fmt.Errorf("android build produced no apk under %s", pattern)
	}
	apk := matches[0]

	keystore, err := output("gdnext", "android", "keystore", "show")
	if err != nil {
		return err
	}
	if err := mustExist(keystore); err != nil {
		return fmt.Errorf("expected keystore at %s but it's missing: %w", keystore, err)
	}

	// apksigner is positional-last: --ks / --ks-pass / --ks-key-alias
	// MUST precede the apk path or apksigner's parser emits
	// "At least one signer must be specified".
	if err := run("gdnext", "android", "apk", "sign",
		"--ks", keystore,
		"--ks-pass", "pass:android",
		"--ks-key-alias", "androiddebugkey",
		apk,
	); err != nil {
		return err
	}
	return run("gdnext", "android", "apk", "verify", apk)
}

func mustExist(p string) error {
	if _, err := os.Stat(p); err != nil {
		return fmt.Errorf("expected file missing: %s (%w)", p, err)
	}
	return nil
}

func assertDirNonEmpty(p string) error {
	entries, err := os.ReadDir(p)
	if err != nil {
		return fmt.Errorf("expected directory missing: %s (%w)", p, err)
	}
	if len(entries) == 0 {
		return fmt.Errorf("expected directory empty: %s", p)
	}
	for _, e := range entries {
		fmt.Println(filepath.Join(p, e.Name()))
	}
	return nil
}
