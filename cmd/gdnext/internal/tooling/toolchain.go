package tooling

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"graphics.gd/product"

	"github.com/schollz/progressbar/v3"
	"runtime.link/api/xray"
)

var debug = os.Getenv(product.EnvDebugCmd) != ""

// Mode controls how Lookup and LookupPlatform resolve a missing toolchain.
// Passing it explicitly at the callsite keeps the behaviour local instead
// of relying on env vars like GDTOOLCHAIN=local.
type Mode int

const (
	// ModeInstall is the default: try $GDPATH/bin, then $PATH, and finally
	// download + extract from the toolchain's DownloadURL. Used by every
	// builder/* implementation that needs a tool to actually run.
	ModeInstall Mode = iota
	// ModeFind only inspects $GDPATH/bin and $PATH. If the binary isn't
	// already present it returns an error rather than fetching it. Used by
	// `gdnext toolchain doctor` so a diagnostic run never silently pulls
	// hundreds of MB.
	ModeFind
	// ModeForceInstall always downloads + extracts into $GDPATH/bin,
	// even when a matching $GDPATH copy already exists or a satisfying
	// version is on $PATH. Used by `gdnext toolchain install --force`
	// to take ownership of a tool the user supplied via their system
	// package manager (gdnext will prefer the GDPATH copy on the next
	// lookup since it's checked first).
	ModeForceInstall
)

// GDTOOLCHAIN=local will disable automatic toolchain downloads.
//
// Deprecated: prefer Mode arg on Lookup / LookupPlatform. The env var is
// still honoured for backwards compatibility with scripts and NixOS users.
//
// Note: GOTOOLCHAIN is reserved by the Go toolchain itself (and is set
// to "local" by actions/setup-go to pin the runtime Go version); we
// must not piggy-back on it for gdnext's download gating because that
// would surprise CI users who only meant to pin go.

// Tool wraps a product.Toolchain record with the mutable runtime
// state gdnext needs to drive it: the resolved BuildHost (so
// LookupPlatform can resolve GD*Path / UserHomeRoot without
// re-deriving them on every call) and the cached install Path.
// The embedded product.Toolchain provides every declarative field
// (Slug / Name / Version / Download* / Required / Available / ...)
// via promotion.
//
// Tools are produced by NewCatalog with Host populated from the
// BuildEnv the CLI's Before hook resolved. Construct one ad-hoc only
// when the resulting Tool will never be looked up.
type Tool struct {
	product.Toolchain
	Host product.BuildHost
	Path string // cached by [toolchain.Lookup]
}

// ManagedByPath classifies who owns the resolved binary at path.
// GDManaged when path is under Host.GDRootPath (gdnext installed
// it, gdnext is responsible for upgrades + the checksum sidecar);
// UserManaged otherwise (came from $PATH or a pre-existing local
// install). Returns UserManaged for an empty path to keep the
// "user owns it until proven otherwise" default. Callers pass the
// resolved path explicitly because library tools' Path field is a
// scalar shared across every target-fanned Lookup.
func (exe Tool) ManagedByPath(path string) product.ManageType {
	if path == "" || exe.Host.GDRootPath == "" {
		return product.UserManaged
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	root, err := filepath.Abs(exe.Host.GDRootPath)
	if err != nil {
		root = exe.Host.GDRootPath
	}
	if rel, err := filepath.Rel(root, abs); err == nil && !strings.HasPrefix(rel, "..") {
		return product.GDManaged
	}
	return product.UserManaged
}

func (exe Tool) PathToCommand() string {
	if exe.Path == "" {
		panic("toolchain.PathToCommand: toolchain not yet looked up")
	}
	if exe.IsApp && runtime.GOOS == product.GOOSDarwin {
		return filepath.Join(exe.Path, "Contents", "MacOS", exe.Name)
	}
	return exe.Path
}

// invocation returns the (command, args) pair to actually exec for
// this tool. For native binaries it's just (path, args). For
// JavaJar tools (apktool, bundletool), the catalog records the path
// to a jar but the runtime invocation is `java -jar <path> <args>`;
// every exec site goes through this helper so the wrapping only
// lives in one place. Lookup is not re-run here; the caller is
// expected to have already resolved the path.
func (exe Tool) invocation(path string, args []string) (string, []string) {
	if exe.JavaJar {
		return "java", append([]string{"-jar", path}, args...)
	}
	return path, args
}

func (exe Tool) Exec(args ...string) error {
	var converted []string
	for _, arg := range args {
		// Only the key portion (before "=") participates in the
		// ConvertArguments lookup; the full original arg is what
		// reaches the command. Truncating unconditionally would
		// strip the value half of long-form flags like
		// "--output=/path/to/file" before exec.
		key := arg
		if i := strings.Index(arg, "="); i >= 0 {
			key = arg[:i]
		}
		if newarg, ok := exe.ConvertArguments[key]; ok {
			if newarg == "" {
				continue
			}
			converted = append(converted, newarg)
		} else {
			converted = append(converted, arg)
		}
	}
	args = converted
	path, err := exe.Lookup()
	if err != nil {
		return xray.New(err)
	}
	name, args := exe.invocation(path, args)
	cmd := exec.Command(name, args...)
	if debug {
		fmt.Println(name, strings.Join(args, " "))
	}
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func (exe Tool) Action(name string, suffix_args []string, args ...string) error {
	var suffix = make([]string, 0, len(suffix_args))
	for _, arg := range suffix_args {
		suffix = append(suffix, arg)
	}
	for i, arg := range args {
		if newarg, ok := exe.ConvertArguments[arg]; ok {
			args[i] = newarg
		}
	}
	for i, arg := range suffix {
		if newarg, ok := exe.ConvertArguments[arg]; ok {
			suffix[i] = newarg
		}
	}
	path, err := exe.Lookup()
	if err != nil {
		return xray.New(err)
	}
	args = append(append([]string{name}, args...), suffix...)
	cmdName, cmdArgs := exe.invocation(path, args)
	cmd := exec.Command(cmdName, cmdArgs...)
	if debug {
		fmt.Println(cmdName, strings.Join(cmdArgs, " "))
	}
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func (exe Tool) Output(args ...string) (string, error) {
	path, err := exe.Lookup()
	if err != nil {
		return "", err
	}
	name, args := exe.invocation(path, args)
	if debug {
		fmt.Println(name, strings.Join(args, " "))
	}
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (exe Tool) CombinedOutput(args ...string) (string, error) {
	path, err := exe.Lookup()
	if err != nil {
		return "", err
	}
	name, args := exe.invocation(path, args)
	if debug {
		fmt.Println(name, strings.Join(args, " "))
	}
	out, err := exec.Command(name, args...).CombinedOutput()
	if debug {
		fmt.Println(string(out))
	}
	return strings.TrimSpace(string(out)), err
}

// Lookup resolves the toolchain to a runnable path. Pass ModeFind to skip
// the download step (diagnostic / dry-run); pass nothing or ModeInstall to
// auto-download when missing.
func (exe *Tool) Lookup(mode ...Mode) (string, error) {
	return exe.LookupPlatform(runtime.GOOS, runtime.GOARCH, "", mode...)
}

// LookupPlatform resolves the toolchain for the given (goos, goarch, libc) target.
// libc is only meaningful for linux libgodot-style artefacts that fan out
// between glibc and musl; pass "" for every other tool and target.
func (exe *Tool) LookupPlatform(GOOS, GOARCH, LibC string, mode ...Mode) (string, error) {
	m := ModeInstall
	if len(mode) > 0 {
		m = mode[0]
	}
	// Library artefacts fan out across (GOOS, GOARCH, LibC) targets
	// but share a single *Tool via Catalog.BySlug. Caching Path on
	// that shared struct would leak the first target's resolution
	// into every subsequent call — one job's linux/amd64 install
	// would trick a later linux/arm64 Lookup into returning the
	// amd64 path. Non-library binaries are host-native (one path
	// per host) and safe to cache.
	if exe.Path != "" && !exe.IsLibrary {
		return exe.Path, nil
	}
	if exe.Host.GDRootPath == "" {
		return "", fmt.Errorf("tooling.Tool.LookupPlatform: %s has no Host populated (construct via tooling.NewCatalog from a resolved BuildEnv)", exe.Slug)
	}
	// Materialise prerequisite bundles before resolving the binary
	// path. Tools that live inside a multi-binary archive (apksigner
	// and aapt2 inside android-build-tools, adb inside
	// android-platform-tools) declare the bundle slug here so the
	// install + extract happens once and every consumer just looks
	// inside the resulting directory. Skipped under ModeFind so
	// `gdnext toolchain doctor` stays read-only. The first bundle's
	// install_dir is captured as bundleInstallDir and becomes the
	// consumer's install_dir when the consumer hasn't set an
	// explicit Installations entry — keeping bundle versions in one
	// place (the bundle's Version field) instead of duplicated
	// across every dependent. Bundles are always resolved against
	// the host (a single physical install per machine), even when
	// the consumer is target-keyed (android.jar fans out over
	// android/metaquest tuples but pulls from one host bundle).
	var bundleInstallDir string
	bundleGOOS, bundleGOARCH := exe.Host.GOOS, exe.Host.GOARCH
	for _, slug := range exe.RequiresBundles {
		bundle, ok := product.FindToolchainBySlug(slug)
		if !ok {
			return "", fmt.Errorf("toolchain %s requires bundle %q, but no such slug is registered in product.ToolchainMatrix", exe.Slug, slug)
		}
		bundleTool := &Tool{Toolchain: bundle, Host: exe.Host}
		if m != ModeFind {
			if _, err := bundleTool.LookupPlatform(bundleGOOS, bundleGOARCH, "", m); err != nil {
				return "", fmt.Errorf("toolchain %s: required bundle %s: %w", exe.Slug, slug, err)
			}
		}
		if bundleInstallDir == "" {
			bundleInstallDir = bundleTool.installDirFor(bundleGOOS, bundleGOARCH)
		}
	}
	HOME := exe.Host.UserHomeRoot
	GDPATH := exe.Host.GDRootPath
	GDBin := exe.Host.GDBinPath
	GDLib := exe.Host.GDLibPath
	ARCH := exe.DownloadARCH[GOARCH]
	if ARCH == "" {
		ARCH = "$(MISSING)"
	}
	OS := strings.ReplaceAll(exe.DownloadOS[GOOS], "$(ARCH)", ARCH)
	if OS == "" {
		OS = "$(MISSING)"
	}
	EXT, ok := exe.DownloadEXT[GOOS]
	if !ok {
		EXT = "$(MISSING)"
	}
	var MaybeUniversal = GOARCH
	if GOOS == product.GOOSDarwin && exe.DarwinUniversal {
		MaybeUniversal = "universal"
	}
	var LIBC = LibC
	var LIBC_DOT string
	if LibC != "" {
		LIBC_DOT = "." + LibC
	}
	var variables = strings.NewReplacer(
		"$(VERSION)", exe.Version,
		"$(ARCH)", ARCH,
		"$(OS)", OS,
		"$(GOARCH)", MaybeUniversal,
		"$(GOOS)", GOOS,
		"$(HOME)", HOME,
		"$(GDPATH)", GDPATH,
		"$(EXT)", EXT,
		"$(LIBC)", LIBC,
		"$(LIBC_DOT)", LIBC_DOT,
	)
	var install_dir = GDBin
	if exe.IsLibrary {
		install_dir = GDLib
	}
	if dir, ok := exe.Installations[GOOS]; ok {
		install_dir = variables.Replace(dir)
	} else if bundleInstallDir != "" {
		install_dir = bundleInstallDir
	}
	// IsBundle: this toolchain is a directory of files (e.g. android
	// build-tools / platform-tools). Lookup returns the install_dir
	// itself; there is no single binary to probe for a version.
	// Existence + non-emptiness of install_dir is the success
	// condition. Install fetches the archive and ExtractArchive
	// strips the upstream top-level dir so contents land directly
	// under install_dir.
	if exe.IsBundle {
		return exe.lookupBundle(install_dir, GOOS, GOARCH, m, variables)
	}
	var name = variables.Replace(exe.Name)
	var install_path = filepath.Join(install_dir, name)
	// .exe is for executables we drop into GDBin on a Windows host;
	// libraries carry their own extension via $(EXT) (e.g. .a, .lib)
	// and must never get a host-driven suffix tacked on.
	if runtime.GOOS == product.GOOSWindows && !exe.IsLibrary {
		install_path += ".exe"
	}
	if exe.IsApp && runtime.GOOS == product.GOOSDarwin {
		install_path += ".app"
	}
	// Bundle-sourced wrappers on Windows: android build-tools ships
	// apksigner / sdkmanager / lint as `.bat`, not `.exe`. When the
	// initial probe misses, try the windows wrapper extensions so the
	// resolver doesn't fall through to "no download URL".
	if runtime.GOOS == product.GOOSWindows && !exe.IsLibrary && bundleInstallDir != "" {
		if _, err := os.Stat(install_path); err != nil {
			base := strings.TrimSuffix(install_path, ".exe")
			for _, ext := range []string{".bat", ".cmd"} {
				if _, err := os.Stat(base + ext); err == nil {
					install_path = base + ext
					break
				}
			}
		}
	}
	// always prefer the GDPATH-installed version if it matches the expected version.
	// ModeForceInstall skips this branch entirely so --force always
	// re-downloads.
	if _, err := os.Stat(install_path); err == nil && m != ModeForceInstall {
		if exe.IsLibrary {
			return install_path, nil
		}
		var exe_path = install_path
		if exe.IsApp && runtime.GOOS == product.GOOSDarwin {
			exe_path = filepath.Join(install_path, "Contents", "MacOS", name)
		}
		if exe.Name == "godot" && os.Getenv(product.EnvRunningInsideGodot) != "" {
			exe.Path = install_path
			return exe.PathToCommand(), nil
		}
		probeName, probeArgs := exe.invocation(exe_path, exe.VersionFlags)
		version, err := exec.Command(probeName, probeArgs...).CombinedOutput()
		version = bytes.TrimSpace(version)
		if err == nil {
			if (exe.Version != "" && string(version) == exe.Version) || (exe.VersionPrefix != "" && strings.HasPrefix(string(version), exe.VersionPrefix)) {
				exe.Path = install_path
				return exe.PathToCommand(), nil
			}
		}
		// Mode==Find is lookup-only: trust the file at install_path
		// even when the version probe disagrees (some tools print
		// per-host wrappers that defeat the prefix check).
		if m == ModeFind {
			exe.Path = install_path
			return exe.PathToCommand(), nil
		}
	}
	// some users (ie. NixOS) don't want things to be automatically installed, they
	// can set their toolchain to local and download/install everything themselves.
	// Mode==Find produces the same effect explicitly at the call site.
	if m == ModeFind || os.Getenv(product.EnvGDToolchain) == "local" {
		path, err := exec.LookPath(name)
		if err != nil {
			return "", fmt.Errorf(
				"'%v' %s not found in $PATH (required for %v) and automatic-downloads are disabled, please install it, ie. %v",
				name, exe.Version, exe.RequiredFor, exe.DownloadHint,
			)
		}
		exe.Path = path
		return exe.PathToCommand(), nil
	}
	if !exe.IsLibrary && m != ModeForceInstall {
		// if the expected version of the tool is already installed in $PATH, then we can
		// just use it. ModeForceInstall skips this branch so --force
		// always downloads into GDPATH rather than adopting the user's
		// system copy. JavaJar tools skip this entirely — the jar is
		// the artefact, never on $PATH directly.
		if !exe.JavaJar {
			if path, err := exec.LookPath(name); err == nil {
				version, _ := exec.Command(path, exe.VersionFlags...).CombinedOutput()
				if (exe.Version != "" && string(version) == exe.Version) || (exe.VersionPrefix != "" && strings.HasPrefix(string(version), exe.VersionPrefix)) || (exe.Version == "" && exe.VersionPrefix == "") {
					exe.Path = path
					if exe.IsApp {
						exe.IsApp = false
					}
					if err := exe.ensureBinSymlink(path, GDBin, EXT); err != nil {
						return "", xray.New(err)
					}
					return exe.PathToCommand(), nil
				}
			}
		}
	}
	// attempt to automatically download and install the toolchain.
	url, ok := exe.Downloads[GOOS][GOARCH]
	if !ok {
		url = variables.Replace(exe.DownloadURL)
	}
	if url == "" || strings.Contains(url, "$(MISSING)") {
		return "", fmt.Errorf(
			"'%v' %s not found in $PATH (required for %v) and no automatic-download is available, please install it, ie. %v",
			name, exe.Version, exe.RequiredFor, exe.DownloadHint,
		)
	}
	if err := os.MkdirAll(install_dir, 0755); err != nil {
		return "", xray.New(err)
	}
	var dest = install_path
	dest += "." + exe.Version + ".download"
	// A leftover .download from a previous run is only useful as a
	// resume cursor for an interrupted in-flight download. When the
	// caller asked for --force or --skip-checksum they're explicitly
	// asking for a fresh fetch — a stale .download here would trigger
	// HTTP 416 on the next Range: request. Drop it so we start clean.
	if m == ModeForceInstall || os.Getenv(product.EnvSkipChecksum) != "" {
		_ = os.Remove(dest)
	}
	if err := func() error {
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY, 0755)
		if err != nil {
			return xray.New(err)
		}
		defer out.Close()
		stat, err := out.Stat()
		if err != nil {
			return xray.New(err)
		}
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return xray.New(err)
		}
		if stat.Size() > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", stat.Size()))
		}
		req.Header.Set("User-Agent", "graphics.gd/cmd/gd")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return xray.New(err)
		}
		defer resp.Body.Close()
		switch resp.StatusCode {
		case 200:
		case 206:
			if _, err := out.Seek(stat.Size(), io.SeekStart); err != nil {
				return xray.New(err)
			}
		case 416:
			contentRange := resp.Header.Get("Content-Range")
			if contentRange != fmt.Sprintf("bytes */%d", stat.Size()) {
				return fmt.Errorf("unable to resume download of '%v' (required for %v), please delete %v and try again\nGET %s HTTP status: %v", name, exe.RequiredFor, dest, url, resp.StatusCode)
			}
		default:
			return fmt.Errorf(
				"unable to download '%v' (required for %v) and not found in $PATH, please install it, ie. %v\nGET %s HTTP status: %v",
				name, exe.RequiredFor, exe.DownloadHint, url, resp.StatusCode,
			)
		}
		if resp.StatusCode != 416 {
			bar := progressbar.DefaultBytes(
				resp.ContentLength,
				fmt.Sprintf("gd: downloading %s v%s", name, exe.Version),
			)
			if _, err := io.Copy(io.MultiWriter(out, bar), resp.Body); err != nil {
				_ = bar.Close()
				return xray.New(err)
			}
			_ = bar.Finish()
			_ = bar.Close()
			fmt.Fprintln(os.Stderr)
		}
		return nil
	}(); err != nil {
		return "", xray.New(err)
	}
	// Hash the downloaded archive (single file, pre-extract) for the
	// supply-chain check. We can't tee during io.Copy because the
	// 416-resume branch writes nothing, and partial writes from a
	// resumed 206 would only hash the new bytes. Re-reading dest is
	// trivial vs the download cost. The sha + size land at
	// <GDChecksumsPath>/<slug>-<goos>-<goarch>.sha256 so the audit
	// can surface them without re-downloading, and so the verifier
	// can union the on-disk sidecar with the catalog's KnownChecksums
	// on the next install.
	dlSize, dlHash, err := sha256File(dest)
	if err != nil {
		return "", xray.New(err)
	}
	downloadHash := "sha256:" + dlHash
	sidecarPath := sidecarPathFor(exe.Host, exe.Slug, GOOS, GOARCH, LibC)
	if err := verifyChecksum(downloadHash, exe.KnownChecksums, sidecarPath); err != nil {
		// Leave dest in place so the user can inspect what was
		// served before deciding whether to retry, allow-list, or
		// rotate the catalog entry.
		return "", xray.New(fmt.Errorf("checksum verification failed for %s (downloaded from %s, kept at %s): %w", name, url, dest, err))
	}
	if err := writeSidecar(sidecarPath, downloadHash, dlSize); err != nil {
		return "", xray.New(err)
	}
	var unzip = variables.Replace(exe.Unzip)
	if exe.IsApp && runtime.GOOS == product.GOOSDarwin {
		unzip = ""
	}
	switch {
	case strings.HasSuffix(url, ".zip"):
		if err := ExtractArchive(dest, install_dir, "zip", unzip, runtime.GOOS != product.GOOSDarwin || !exe.IsApp); err != nil {
			return "", xray.New(err)
		}
		if err := os.Remove(dest); err != nil {
			return "", xray.New(err)
		}
	case strings.HasSuffix(url, ".tar.gz"):
		if err := ExtractArchive(dest, install_dir, "tar.gz", unzip, true); err != nil {
			return "", xray.New(err)
		}
		if err := os.Remove(dest); err != nil {
			return "", xray.New(err)
		}
	case strings.HasSuffix(url, ".tar.xz"):
		if err := ExtractArchive(dest, install_dir, "tar.xz", unzip, true); err != nil {
			return "", xray.New(err)
		}
		if err := os.Remove(dest); err != nil {
			return "", xray.New(err)
		}
	default:
		if err := os.Rename(dest, install_path); err != nil {
			return "", xray.New(err)
		}
	}
	if unzip != "" {
		if err := os.Rename(filepath.Join(install_dir, unzip), install_path); err != nil {
			return "", xray.New(err)
		}
	}
	if exe.IsLibrary {
		return install_path, nil
	}
	exe.Path = install_path
	return exe.PathToCommand(), nil
}

// installDirFor returns the resolved install_dir for this toolchain on the
// given (GOOS, GOARCH) without performing any installation. Used by tools
// that declare RequiresBundles so they can inherit their bundle's path
// (versioned by the bundle's Version field) instead of duplicating it in
// their own Installations map.
func (exe *Tool) installDirFor(GOOS, GOARCH string) string {
	GDPATH := exe.Host.GDRootPath
	GDBin := exe.Host.GDBinPath
	GDLib := exe.Host.GDLibPath
	ARCH := exe.DownloadARCH[GOARCH]
	if ARCH == "" {
		ARCH = "$(MISSING)"
	}
	OS := strings.ReplaceAll(exe.DownloadOS[GOOS], "$(ARCH)", ARCH)
	if OS == "" {
		OS = "$(MISSING)"
	}
	MaybeUniversal := GOARCH
	if GOOS == product.GOOSDarwin && exe.DarwinUniversal {
		MaybeUniversal = "universal"
	}
	variables := strings.NewReplacer(
		"$(VERSION)", exe.Version,
		"$(ARCH)", ARCH,
		"$(OS)", OS,
		"$(GOARCH)", MaybeUniversal,
		"$(GOOS)", GOOS,
		"$(HOME)", exe.Host.UserHomeRoot,
		"$(GDPATH)", GDPATH,
		"$(LIBC)", "",
		"$(LIBC_DOT)", "",
	)
	install_dir := GDBin
	if exe.IsLibrary {
		install_dir = GDLib
	}
	if dir, ok := exe.Installations[GOOS]; ok {
		install_dir = variables.Replace(dir)
	}
	return install_dir
}

// ensureBinSymlink creates $(GDBin)/<name><ext> as a symlink to the
// resolved (user-managed) tool path when the toolchain entry has
// AddBinSymlink set. Used by tools we don't manage ourselves but that
// downstream consumers (e.g. Godot's android exporter probing
// $(GDPATH)/bin/java) expect to find under GDBin. No-op when
// AddBinSymlink is false, when target already points where we want,
// or when target equals source (would create a self-loop).
func (exe *Tool) ensureBinSymlink(realPath, gdBin, ext string) error {
	if !exe.AddBinSymlink || realPath == "" || gdBin == "" {
		return nil
	}
	linkPath := filepath.Join(gdBin, exe.Name+ext)
	if linkPath == realPath {
		return nil
	}
	if existing, err := os.Readlink(linkPath); err == nil && existing == realPath {
		return nil
	}
	if err := os.MkdirAll(gdBin, 0755); err != nil {
		return err
	}
	_ = os.Remove(linkPath)
	return os.Symlink(realPath, linkPath)
}

// lookupBundle handles the IsBundle install path: the toolchain is a
// directory of files (android build-tools / platform-tools) and
// "lookup" means "make sure install_dir exists and isn't empty,
// otherwise fetch + extract the upstream archive into it." Returns
// the install_dir as the resolved Path so callers can build
// per-file paths inside (e.g. install_dir+"/lib/apksigner.jar"). No
// version probe — the sidecar carries the archive hash and the
// install_dir contents are what they are.
func (exe *Tool) lookupBundle(install_dir, GOOS, GOARCH string, m Mode, variables *strings.Replacer) (string, error) {
	if dirNonEmpty(install_dir) && m != ModeForceInstall {
		exe.Path = install_dir
		return install_dir, nil
	}
	if m == ModeFind || os.Getenv(product.EnvGDToolchain) == "local" {
		return "", fmt.Errorf("bundle %q not installed at %s (required for %s) and automatic-downloads are disabled, ie. %s",
			exe.Slug, install_dir, exe.RequiredFor, exe.DownloadHint)
	}
	url, ok := exe.Downloads[GOOS][GOARCH]
	if !ok {
		url = variables.Replace(exe.DownloadURL)
	}
	if url == "" || strings.Contains(url, "$(MISSING)") {
		return "", fmt.Errorf("bundle %q has no download URL for %s/%s (required for %s), ie. %s",
			exe.Slug, GOOS, GOARCH, exe.RequiredFor, exe.DownloadHint)
	}
	if err := os.MkdirAll(install_dir, 0755); err != nil {
		return "", xray.New(err)
	}
	// Stage the archive next to the install dir so we can resume
	// interrupted downloads via Range:, mirroring the single-file
	// path above.
	dest := install_dir + "." + bundleArchiveSuffix(url) + ".download"
	if m == ModeForceInstall || os.Getenv(product.EnvSkipChecksum) != "" {
		_ = os.Remove(dest)
	}
	if err := downloadResumable(url, dest, exe.Name, exe.Version); err != nil {
		return "", xray.New(err)
	}
	dlSize, dlHash, err := sha256File(dest)
	if err != nil {
		return "", xray.New(err)
	}
	downloadHash := "sha256:" + dlHash
	sidecarPath := sidecarPathFor(exe.Host, exe.Slug, GOOS, GOARCH, "")
	if err := verifyChecksum(downloadHash, exe.KnownChecksums, sidecarPath); err != nil {
		return "", xray.New(fmt.Errorf("checksum verification failed for %s (downloaded from %s, kept at %s): %w", exe.Slug, url, dest, err))
	}
	if err := writeSidecar(sidecarPath, downloadHash, dlSize); err != nil {
		return "", xray.New(err)
	}
	if err := ExtractArchive(dest, install_dir, archiveType(url), "", true); err != nil {
		return "", xray.New(err)
	}
	if err := os.Remove(dest); err != nil {
		return "", xray.New(err)
	}
	exe.Path = install_dir
	return install_dir, nil
}

// dirNonEmpty reports whether path is a directory with at least one
// entry. Bundle lookup uses this as the "installed" sentinel since
// there is no single binary whose existence we can probe.
func dirNonEmpty(path string) bool {
	entries, err := os.ReadDir(path)
	return err == nil && len(entries) > 0
}

// archiveType maps a download URL suffix to the format identifier
// ExtractArchive expects. Falls back to "zip" for unknown suffixes
// since every supported bundle today is a zip; bump the table when
// adding a tarball.
func archiveType(url string) string {
	switch {
	case strings.HasSuffix(url, ".tar.gz"):
		return "tar.gz"
	case strings.HasSuffix(url, ".tar.xz"):
		return "tar.xz"
	case strings.HasSuffix(url, ".tar.bz2"):
		return "tar.bz2"
	default:
		return "zip"
	}
}

// bundleArchiveSuffix derives a stable filename component for the
// staged .download. Keeps the URL's basename (less archive
// extension) so different bundle versions don't share a download
// slot. Falls back to "archive" when the URL parses oddly.
func bundleArchiveSuffix(url string) string {
	base := url
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	for _, ext := range []string{".tar.gz", ".tar.xz", ".tar.bz2", ".zip"} {
		if strings.HasSuffix(base, ext) {
			base = strings.TrimSuffix(base, ext)
			break
		}
	}
	if base == "" {
		return "archive"
	}
	return base
}

// downloadResumable copies url to dest with Range: resume support and
// a progress bar. Extracted from the single-file branch above so the
// bundle path can share the same retry semantics. Stays as a free
// function (not a method) so callers needing a one-shot download
// without all the install_dir / variables / sidecar bookkeeping can
// reach for it directly.
func downloadResumable(url, dest, displayName, displayVersion string) error {
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer out.Close()
	stat, err := out.Stat()
	if err != nil {
		return err
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	if stat.Size() > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", stat.Size()))
	}
	req.Header.Set("User-Agent", "graphics.gd/cmd/gd")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case 200:
	case 206:
		if _, err := out.Seek(stat.Size(), io.SeekStart); err != nil {
			return err
		}
	case 416:
		contentRange := resp.Header.Get("Content-Range")
		if contentRange != fmt.Sprintf("bytes */%d", stat.Size()) {
			return fmt.Errorf("unable to resume download of %s from %s (delete %s and retry)", displayName, url, dest)
		}
	default:
		return fmt.Errorf("GET %s returned HTTP %d (downloading %s)", url, resp.StatusCode, displayName)
	}
	if resp.StatusCode != 416 {
		bar := progressbar.DefaultBytes(
			resp.ContentLength,
			fmt.Sprintf("gd: downloading %s v%s", displayName, displayVersion),
		)
		if _, err := io.Copy(io.MultiWriter(out, bar), resp.Body); err != nil {
			_ = bar.Close()
			return err
		}
		_ = bar.Finish()
		_ = bar.Close()
		fmt.Fprintln(os.Stderr)
	}
	return nil
}

// sha256File streams the file at path through sha256 and returns the
// byte count and hex digest. Returns an error when path is a directory;
// KnownChecksums targets the downloaded archive, which is always a
// single file.
func sha256File(path string) (int64, string, error) {
	st, err := os.Stat(path)
	if err != nil {
		return 0, "", err
	}
	if st.IsDir() {
		return 0, "", fmt.Errorf("sha256File: %s is a directory", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return 0, "", err
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

// verifyChecksum accepts a download when one of the following holds:
//
//   - GDNEXT_SKIP_CHECKSUM=1 (or `--skip-checksum`) is set. The
//     sidecar is still written on success, pinning the freshly-seen
//     hash so future installs verify against it without the escape
//     hatch. This is the bootstrap path for a brand-new tool whose
//     hash hasn't been recorded yet.
//   - got matches any entry in catalogKnown (the pinned set in
//     product.Toolchain.KnownChecksums).
//   - got matches the value in the existing sidecar at sidecarPath.
//
// Otherwise the install is refused, including the "no catalog entries
// and no sidecar" case. Implicit trust on a fresh tool is a feature,
// not a default — the user has to opt in once via --skip-checksum.
func verifyChecksum(got string, catalogKnown []string, sidecarPath string) error {
	if os.Getenv(product.EnvSkipChecksum) != "" {
		return nil
	}
	for _, want := range catalogKnown {
		if want == got {
			return nil
		}
	}
	sidecarSum, _, sidecarErr := readSidecar(sidecarPath)
	if sidecarErr == nil && sidecarSum == got {
		return nil
	}
	if len(catalogKnown) == 0 && os.IsNotExist(sidecarErr) {
		return fmt.Errorf("no catalog KnownChecksums entries and no sidecar at %s; got %s. Pass --skip-checksum (or set GDNEXT_SKIP_CHECKSUM=1) once to accept this download — the sidecar will be written and pin the hash for future installs", sidecarPath, got)
	}
	return fmt.Errorf("got %s, no match in %d catalog entry/entries or sidecar %s (override with GDNEXT_SKIP_CHECKSUM=1 or --skip-checksum)", got, len(catalogKnown), sidecarPath)
}

// sidecarPathFor returns <Host.GDChecksumsPath>/<slug>-<goos>-<goarch>[-<libc>].sha256.
// libc segment is omitted when empty so existing (non-libc-fanned) sidecars stay valid.
func sidecarPathFor(host product.BuildHost, slug, goos, goarch, libc string) string {
	dir := host.GDChecksumsPath()
	if dir == "" {
		return ""
	}
	name := fmt.Sprintf("%s-%s-%s", slug, goos, goarch)
	if libc != "" {
		name += "-" + libc
	}
	return filepath.Join(dir, name+".sha256")
}

// writeSidecar persists the (sha256, size) for a freshly downloaded
// artefact under GDChecksumsPath. Same line format as the legacy
// per-binary sidecar so the read path is shared.
func writeSidecar(path, sum string, size int64) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	body := fmt.Sprintf("%s\nsize:%d\n", sum, size)
	return os.WriteFile(path, []byte(body), 0644)
}

// readSidecar parses a sidecar file. Format: "sha256:<hex>\nsize:<bytes>\n".
// Returns os.ErrNotExist when the file is absent so callers can
// distinguish "no sidecar" from "sidecar present but unreadable".
func readSidecar(path string) (sum string, size int64, err error) {
	if path == "" {
		return "", 0, os.ErrNotExist
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", 0, err
	}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		switch {
		case strings.HasPrefix(line, "sha256:"):
			sum = line
		case strings.HasPrefix(line, "size:"):
			fmt.Sscanf(strings.TrimPrefix(line, "size:"), "%d", &size)
		}
	}
	if sum == "" {
		return "", 0, fmt.Errorf("sidecar %s missing sha256 line", path)
	}
	return sum, size, nil
}

// SidecarPath is the package-public wrapper around sidecarPathFor for
// callers (cli audit, ci summary) that need to read the sidecar back.
func SidecarPath(host product.BuildHost, slug, goos, goarch, libc string) string {
	return sidecarPathFor(host, slug, goos, goarch, libc)
}

// ReadSidecar is the package-public read helper for the audit /
// summary code paths.
func ReadSidecar(path string) (sum string, size int64, err error) {
	return readSidecar(path)
}

// InstallLibraryFromFile copies srcPath into <host.GDLibPath>/destName and writes the matching sidecar
// under host.GDChecksumsPath keyed on (slug, goos, goarch, libc). Returns the installed absolute path
// plus the computed sha256. libc is only used for linux libgodot-style artefacts; pass "" otherwise.
func InstallLibraryFromFile(host product.BuildHost, slug, goos, goarch, libc, srcPath, destName string) (installedPath, sum string, err error) {
	if host.GDLibPath == "" {
		return "", "", fmt.Errorf("tooling.InstallLibraryFromFile: host has no GDLibPath (BuildEnv not resolved)")
	}
	if err := os.MkdirAll(host.GDLibPath, 0755); err != nil {
		return "", "", xray.New(err)
	}
	dest := filepath.Join(host.GDLibPath, destName)
	if err := copyFile(srcPath, dest); err != nil {
		return "", "", xray.New(err)
	}
	size, hash, err := sha256File(dest)
	if err != nil {
		return "", "", xray.New(err)
	}
	sum = "sha256:" + hash
	sidecarPath := sidecarPathFor(host, slug, goos, goarch, libc)
	if err := writeSidecar(sidecarPath, sum, size); err != nil {
		return "", "", xray.New(err)
	}
	return dest, sum, nil
}

// copyFile copies src to dst with 0644 perms, creating dst if missing
// and overwriting when present. Used by InstallLibraryFromFile so we
// don't rely on os.Rename (which fails across filesystems, common in
// CI when the scratch dir is a tmpfs and GDLibPath is on the runner's
// data volume).
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}
