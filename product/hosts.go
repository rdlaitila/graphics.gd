package product

import (
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/samber/lo"
)

// BuildHost is the machine the CLI is running on: OS, arch, and the
// resolved gd/godot directory paths.
type BuildHost struct {
	GOOS   string
	GOARCH string
	// GD* paths come from the CLI's prepareBuildEnv ($GDPATH, default ~/gd).
	GDRootPath string
	GDLibPath  string
	GDBinPath  string
	// UserHomeRoot is os.UserHomeDir; UserAppdataRoot is $APPDATA on
	// Windows and equal to UserHomeRoot elsewhere.
	UserHomeRoot    string
	UserAppdataRoot string
}

// TargetHost is the (GOOS, GOARCH, LibC, LinkMode) a build is producing for.
// LibC is populated only for linux targets; empty elsewhere.
type TargetHost struct {
	GOOS     string
	GOARCH   string
	LibC     string
	LinkMode LinkMode
}

// PlayHost is a machine that runs a built artefact as a playable
// session (`gdnext-play` in CI). Carries identity plus the headless
// display and compat-layer the matrix should drive the artefact
// through. One PlayHost = one matrix cell shape; combinations like
// wine vs proton are modelled as separate PlayHost variants (see
// PlayLinuxAmd64Wine, PlayLinuxAmd64Proton in matrix.go) so the play
// matrix enumerates them naturally.
//
// VirtualDisplay is empty when the host has a real display or runs
// without one (e.g. "xvfb" on linux CI). CompatLayer is empty when
// the host runs the target natively; otherwise it names the wrapper
// ("wine", "proton", "proton-9", ...) the driver should set up and
// invoke.
type PlayHost struct {
	GOOS           string
	GOARCH         string
	VirtualDisplay string
	CompatLayer    string
}

// Tuple returns the play host as "goos/goarch".
func (t PlayHost) Tuple() string { return Tuple(t.GOOS, t.GOARCH) }

// BuildEnv pairs the host the CLI is running on with the target a
// build is producing for.
type BuildEnv struct {
	Host   BuildHost
	Target TargetHost
}

// ManageType labels who owns a toolchain on disk: GDManaged (the CLI
// downloaded it under GDRootPath and can be trusted to keep it up to
// date), or UserManaged (the user supplied it via $PATH or an
// existing install). Resolved at Lookup time by inspecting Tool.Path.
type ManageType uint8

const (
	UserManaged ManageType = iota
	GDManaged
)

// Tuple returns the host as "goos/goarch".
func (t BuildHost) Tuple() string { return Tuple(t.GOOS, t.GOARCH) }

// GDChecksumsPath returns <GDRootPath>/checksums, the directory holding
// per-artefact sha256 sidecar files written by `gdnext toolchain install`
// and consulted by the verifier alongside catalog KnownChecksums.
func (t BuildHost) GDChecksumsPath() string {
	if t.GDRootPath == "" {
		return ""
	}
	return filepath.Join(t.GDRootPath, "checksums")
}

// Tuple returns the target as "goos/goarch".
func (t TargetHost) Tuple() string { return Tuple(t.GOOS, t.GOARCH) }

// FindBuildEnv resolves the BuildEnv for the current runtime host and the given target
// (GOOS, GOARCH, LibC, LinkMode) tokens, applying defaults for empty fields. libc defaults to glibc
// on linux and stays empty elsewhere.
func FindBuildEnv(targetGOOS, targetGOARCH, targetLibC, targetLinkMode string) (BuildEnv, error) {
	var env BuildEnv
	for _, host := range HostMatrix {
		if host.GOOS == runtime.GOOS && host.GOARCH == runtime.GOARCH {
			env.Host = host
			break
		}
	}
	if env.Host.GOOS == "" {
		return env, fmt.Errorf(
			"host '%s' is not a supported build host (see `gdnext platform`)",
			Tuple(runtime.GOOS, runtime.GOARCH),
		)
	}
	if targetGOOS != "" && !lo.Contains(GOOSMatrix, targetGOOS) {
		return env, fmt.Errorf(
			"target GOOS '%s' is not recognized",
			targetGOOS,
		)
	}
	if targetGOARCH != "" && !lo.Contains(GOARCHMatrix, targetGOARCH) {
		return env, fmt.Errorf(
			"target GOARCH '%s' is not recognized",
			targetGOARCH,
		)
	}
	if targetLibC != "" && targetLibC != LibCGlibc && targetLibC != LibCMusl {
		return env, fmt.Errorf(
			"target LibC '%s' is not recognized (want %q or %q)",
			targetLibC, LibCGlibc, LibCMusl,
		)
	}
	// Capture GOOS-alias implied LinkMode / LibC before remapping the GOOS,
	// otherwise GOOS=musl loses its LibGodot + musl implication.
	impliedMode, hasImpliedMode := GOOSAliasLinkMode[targetGOOS]
	impliedLibC, hasImpliedLibC := GOOSAliasLibC[targetGOOS]
	if remap, ok := GOOSRemaps[targetGOOS]; ok {
		targetGOOS = remap
	}
	if targetGOOS != "" {
		env.Target.GOOS = targetGOOS
	}
	if targetGOARCH != "" {
		env.Target.GOARCH = targetGOARCH
	}
	if env.Target.GOOS != "" && env.Target.GOARCH == "" {
		if def, ok := GOOSArchDefaults[env.Target.GOOS]; ok {
			env.Target.GOARCH = def
		}
	}
	if env.Target.GOOS == "" {
		env.Target.GOOS = env.Host.GOOS
	}
	if env.Target.GOARCH == "" {
		env.Target.GOARCH = env.Host.GOARCH
	}
	// LinkMode precedence: --link > GOOS alias > GOOSLinkModeDefaults > GDExtension.
	mode, err := ParseLinkMode(targetLinkMode)
	if err != nil {
		return env, err
	}
	switch {
	case mode != 0:
		env.Target.LinkMode = mode
	case hasImpliedMode:
		env.Target.LinkMode = impliedMode
	default:
		if def, ok := GOOSLinkModeDefaults[env.Target.GOOS]; ok {
			env.Target.LinkMode = def
		} else {
			env.Target.LinkMode = GDExtension
		}
	}
	switch {
	case targetLibC != "":
		env.Target.LibC = targetLibC
	case hasImpliedLibC:
		env.Target.LibC = impliedLibC
	case env.Target.GOOS == GOOSLinux:
		env.Target.LibC = LibCGlibc
	}
	if plat, ok := FindPlatformByTargetEnv(env.Target.GOOS, env.Target.GOARCH); ok {
		if plat.LinkModes != 0 && !plat.LinkModes.Has(env.Target.LinkMode) {
			return env, fmt.Errorf(
				"target %s does not support --link=%s (supports: %s)",
				env.Target.Tuple(), env.Target.LinkMode, plat.LinkModes,
			)
		}
	}
	return env, nil
}

// Validate checks every field downstream code (builders, tooling,
// setup) reads without defensive fallbacks. A non-nil return is always
// a bug in the CLI assembly path, not user input.
func (t BuildEnv) Validate() error {
	if t.Host.GOOS == "" || t.Host.GOARCH == "" {
		return fmt.Errorf("product.BuildEnv: Host (GOOS, GOARCH) is not populated")
	}
	if t.Target.GOOS == "" || t.Target.GOARCH == "" {
		return fmt.Errorf("product.BuildEnv: Target (GOOS, GOARCH) is not populated")
	}
	if t.Target.LinkMode == 0 {
		return fmt.Errorf("product.BuildEnv: Target.LinkMode is not populated")
	}
	if t.Host.GDRootPath == "" || t.Host.GDBinPath == "" || t.Host.GDLibPath == "" {
		return fmt.Errorf("product.BuildEnv: Host GD*Path values are not populated")
	}
	if t.Host.UserHomeRoot == "" || t.Host.UserAppdataRoot == "" {
		return fmt.Errorf("product.BuildEnv: Host User*Root values are not populated")
	}
	return nil
}

// String returns the short token used in audit output: "user" or "gd".
func (t ManageType) String() string {
	switch t {
	case GDManaged:
		return "gd"
	case UserManaged:
		return "user"
	}
	return ""
}

// MarshalText so JSON / YAML / XML render the short token.
func (t ManageType) MarshalText() ([]byte, error) { return []byte(t.String()), nil }
