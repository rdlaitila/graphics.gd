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

var debug = os.Getenv("DEBUG_CMD") != ""

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

// ManagedBy classifies who owns the resolved binary on disk.
// GDManaged when Path is under Host.GDRootPath (gdnext installed it,
// gdnext is responsible for upgrades + the checksum sidecar);
// UserManaged otherwise (came from $PATH or a pre-existing local
// install). Returns UserManaged for an unresolved tool to keep the
// "user owns it until proven otherwise" default.
func (exe Tool) ManagedBy() product.ManageType {
	if exe.Path == "" || exe.Host.GDRootPath == "" {
		return product.UserManaged
	}
	abs, err := filepath.Abs(exe.Path)
	if err != nil {
		abs = exe.Path
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
	if exe.IsApp && runtime.GOOS == "darwin" {
		return filepath.Join(exe.Path, "Contents", "MacOS", exe.Name)
	}
	return exe.Path
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
	cmd := exec.Command(path, args...)
	if debug {
		fmt.Println(path, strings.Join(args, " "))
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
	cmd := exec.Command(path, args...)
	if debug {
		fmt.Println(path, strings.Join(args, " "))
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
	if debug {
		fmt.Println(path, strings.Join(args, " "))
	}
	out, err := exec.Command(path, args...).Output()
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
	if debug {
		fmt.Println(path, strings.Join(args, " "))
	}
	out, err := exec.Command(path, args...).CombinedOutput()
	if debug {
		fmt.Println(string(out))
	}
	return strings.TrimSpace(string(out)), err
}

// Lookup resolves the toolchain to a runnable path. Pass ModeFind to skip
// the download step (diagnostic / dry-run); pass nothing or ModeInstall to
// auto-download when missing.
func (exe *Tool) Lookup(mode ...Mode) (string, error) {
	return exe.LookupPlatform(runtime.GOOS, runtime.GOARCH, mode...)
}

func (exe *Tool) LookupPlatform(GOOS, GOARCH string, mode ...Mode) (string, error) {
	m := ModeInstall
	if len(mode) > 0 {
		m = mode[0]
	}
	if exe.Path != "" {
		return exe.Path, nil
	}
	if exe.Host.GDRootPath == "" {
		return "", fmt.Errorf("tooling.Tool.LookupPlatform: %s has no Host populated (construct via tooling.NewCatalog from a resolved BuildEnv)", exe.Slug)
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
	if GOOS == "darwin" && exe.DarwinUniversal {
		MaybeUniversal = "universal"
	}
	var variables = strings.NewReplacer(
		"$(VERSION)", exe.Version, "$(ARCH)", ARCH, "$(OS)", OS, "$(GOARCH)", MaybeUniversal, "$(GOOS)", GOOS, "$(HOME)", HOME, "$(GDPATH)", GDPATH, "$(EXT)", EXT,
	)
	var install_dir = GDBin
	if exe.IsLibrary {
		install_dir = GDLib
	}
	if dir, ok := exe.Installations[GOOS]; ok {
		install_dir = variables.Replace(dir)
	}
	var name = variables.Replace(exe.Name)
	var install_path = filepath.Join(install_dir, name)
	// .exe is for executables we drop into GDBin on a Windows host;
	// libraries carry their own extension via $(EXT) (e.g. .a, .lib)
	// and must never get a host-driven suffix tacked on. Without this
	// gate libgodot.musl.amd64.a was being looked up as
	// libgodot.musl.amd64.a.exe on the Windows runner.
	if runtime.GOOS == "windows" && !exe.IsLibrary {
		install_path += ".exe"
	}
	if exe.IsApp && runtime.GOOS == "darwin" {
		install_path += ".app"
	}
	// always prefer the GDPATH-installed version if it matches the expected version.
	// ModeForceInstall skips this branch entirely so --force always
	// re-downloads.
	if _, err := os.Stat(install_path); err == nil && m != ModeForceInstall {
		if exe.IsLibrary {
			exe.Path = install_path
			return install_path, nil
		}
		var exe_path = install_path
		if exe.IsApp && runtime.GOOS == "darwin" {
			exe_path = filepath.Join(install_path, "Contents", "MacOS", name)
		}
		if exe.Name == "godot" && os.Getenv("RUNNING_INSIDE_GODOT") != "" {
			exe.Path = install_path
			return exe.PathToCommand(), nil
		}
		version, err := exec.Command(exe_path, exe.VersionFlags...).CombinedOutput()
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
	if m == ModeFind || os.Getenv("GDTOOLCHAIN") == "local" {
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
		// system copy.
		if path, err := exec.LookPath(name); err == nil {
			version, _ := exec.Command(path, exe.VersionFlags...).CombinedOutput()
			if (exe.Version != "" && string(version) == exe.Version) || (exe.VersionPrefix != "" && strings.HasPrefix(string(version), exe.VersionPrefix)) || (exe.Version == "" && exe.VersionPrefix == "") {
				exe.Path = path
				if exe.IsApp {
					exe.IsApp = false
				}
				return exe.PathToCommand(), nil
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
	if m == ModeForceInstall || os.Getenv("GDNEXT_SKIP_CHECKSUM") != "" {
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
				return xray.New(err)
			}
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
	sidecarPath := sidecarPathFor(exe.Host, exe.Slug, GOOS, GOARCH)
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
	if exe.IsApp && runtime.GOOS == "darwin" {
		unzip = ""
	}
	switch {
	case strings.HasSuffix(url, ".zip"):
		if err := ExtractArchive(dest, install_dir, "zip", unzip, runtime.GOOS != "darwin" || !exe.IsApp); err != nil {
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
	exe.Path = install_path
	return exe.PathToCommand(), nil
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
	if os.Getenv("GDNEXT_SKIP_CHECKSUM") != "" {
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

// sidecarPathFor returns <Host.GDChecksumsPath>/<slug>-<goos>-<goarch>.sha256.
// goos/goarch are the (target) tokens LookupPlatform was called with,
// so per-target IsLibrary downloads get one sidecar each and host-
// scoped tools get one sidecar keyed on host.GOOS/host.GOARCH.
func sidecarPathFor(host product.BuildHost, slug, goos, goarch string) string {
	dir := host.GDChecksumsPath()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, fmt.Sprintf("%s-%s-%s.sha256", slug, goos, goarch))
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
func SidecarPath(host product.BuildHost, slug, goos, goarch string) string {
	return sidecarPathFor(host, slug, goos, goarch)
}

// ReadSidecar is the package-public read helper for the audit /
// summary code paths.
func ReadSidecar(path string) (sum string, size int64, err error) {
	return readSidecar(path)
}
