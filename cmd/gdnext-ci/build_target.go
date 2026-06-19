package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"graphics.gd/product"

	"github.com/urfave/cli/v3"
)

// buildTargetCmd is the heaviest of the verbs. Runs `gdnext build`
// for one (GOOS, GOARCH) pair against the staged example, asserts
// the produced shared library + the distributable bundle landed on
// disk, and on android additionally exercises apksigner.
func buildTargetCmd() *cli.Command {
	return &cli.Command{
		Name:  "build-target",
		Usage: "run `gdnext build` for one (GOOS, GOARCH) and assert artefacts",
		Description: "Pipeline:\n" +
			"  1. validate (goos, goarch) via product.Lookup\n" +
			"  2. `gdnext toolchain doctor --fix` for that target (warms cache)\n" +
			"  3. `gdnext -goos … -goarch … build` (stdin closed)\n" +
			"  4. assert shared library landed under graphics/\n" +
			"  5. assert distributable landed under releases/\n" +
			"  6. android-only: sign + verify the produced apk\n" +
			"\n" +
			"--goos / --goarch must form a row in product.PlatformMatrix.\n" +
			"Aliases (`iphone`, `win`, `wasm`, ...) are accepted via\n" +
			"product.Resolve on --goos.",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "goos", Usage: "target GOOS (or alias)", Required: true},
			&cli.StringFlag{Name: "goarch", Usage: "target GOARCH", Required: true},
			&cli.StringFlag{Name: "scratch", Usage: "staged example directory to build in", Required: true},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			goos := cmd.String("goos")
			goarch := cmd.String("goarch")
			scratchArg := cmd.String("scratch")
			plat, ok := product.Lookup(goos, goarch)
			if !ok {
				if p, ok := product.Resolve(goos); ok && (goarch == "" || p.GOARCH == goarch) {
					plat = p
					goos, goarch = p.GOOS, p.GOARCH
				} else {
					return fmt.Errorf("unknown platform %s (try `gdnext platforms` for the matrix)",
						product.Tuple(goos, goarch))
				}
			}
			if !plat.Kind.Has(product.Target) {
				return fmt.Errorf("%s is registered but not a build target", plat.Tuple())
			}
			scratch, err := filepath.Abs(scratchArg)
			if err != nil {
				return err
			}
			if st, err := os.Stat(scratch); err != nil || !st.IsDir() {
				return fmt.Errorf("scratch dir does not exist: %s", scratch)
			}
			// Doctor warms the toolchain cache for this target. A missing
			// REQUIRED tool surfaces here with a clear message instead of
			// as a cryptic build failure deeper in the pipeline.
			env := []string{"GOOS=" + goos, "GOARCH=" + goarch}
			if err := runInEnv(scratch, env, "gdnext", "toolchain", "doctor", "--fix"); err != nil {
				return err
			}
			// Actual build. Stdin closed so the optional AAB-signing
			// `Provide passphrase:` prompt reads EOF immediately rather
			// than consuming the next CI step's output.
			if err := runIn(scratch, "gdnext", "-goos", goos, "-goarch", goarch, "build"); err != nil {
				return err
			}
			if err := assertSharedLibrary(scratch, plat); err != nil {
				return err
			}
			if err := assertDistributable(scratch, plat); err != nil {
				return err
			}
			if plat.GOOS == "android" {
				if err := signAndVerifyApk(scratch, plat); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

// assertSharedLibrary verifies the shared library `gdnext build` is
// expected to produce for plat is present under graphics/. Paths are
// gdnext-builder conventions, not platform metadata, so the mapping
// lives here rather than in product/.
func assertSharedLibrary(scratch string, plat product.Platform) error {
	graphics := filepath.Join(scratch, "graphics")
	switch plat.GOOS {
	case "linux", "musl":
		// musl builds piggyback on the linux_<arch>.so name.
		return mustExist(filepath.Join(graphics, "linux_"+plat.GOARCH+".so"))
	case "windows":
		return mustExist(filepath.Join(graphics, "windows_"+plat.GOARCH+".dll"))
	case "darwin":
		if err := mustExist(filepath.Join(graphics, "darwin_"+plat.GOARCH+".dylib")); err != nil {
			return err
		}
		return mustExist(filepath.Join(graphics, "darwin_universal.dylib"))
	case "js":
		return mustExist(filepath.Join(scratch, "releases", "js", "wasm", "library.wasm"))
	case "android":
		return nil // apk landed under releases/android/<arch>/; checked below
	default:
		return fmt.Errorf("no shared-library assertion for %s", plat.Tuple())
	}
}

// assertDistributable verifies the export bundle landed at the path
// gdnext build writes it to. Each builder writes to a fixed location
// driven by the Godot export preset.
func assertDistributable(scratch string, plat product.Platform) error {
	releases := filepath.Join(scratch, "releases")
	switch plat.GOOS {
	case "linux", "windows", "android", "musl":
		return assertDirNonEmpty(filepath.Join(releases, plat.GOOS, plat.GOARCH))
	case "darwin":
		// macOS exports a universal .app regardless of -goarch.
		return assertDirNonEmpty(filepath.Join(releases, "darwin", "universal"))
	case "js":
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
