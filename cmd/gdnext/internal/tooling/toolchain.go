package tooling

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/user"
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

// toolchain wraps a product.Toolchain record with the mutable runtime
// state gdnext needs to drive it: the cached install Path. The embedded
// product.Toolchain provides every declarative field
// (Slug/Name/Version/Download*/Required/Available/...) via promotion;
// methods on *Tool read those fields and manage Path locally.
type Tool struct {
	product.Toolchain
	Path string // cached by [toolchain.Lookup]
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
	my, err := user.Current()
	if err != nil {
		return "", xray.New(err)
	}
	HOME := my.HomeDir
	GDPATH := os.Getenv("GDPATH")
	if GDPATH == "" && HOME != "" {
		GDPATH = filepath.Join(HOME, "gd")
	}
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
	var install_dir = filepath.Join(GDPATH, "bin")
	if exe.IsLibrary {
		install_dir = filepath.Join(GDPATH, "lib")
	}
	if dir, ok := exe.Installations[GOOS]; ok {
		install_dir = variables.Replace(dir)
	}
	var name = variables.Replace(exe.Name)
	var install_path = filepath.Join(install_dir, name)
	if runtime.GOOS == "windows" {
		install_path += ".exe"
	}
	if exe.IsApp && runtime.GOOS == "darwin" {
		install_path += ".app"
	}
	// always prefer the GDPATH-installed version if it matches the expected version.
	if _, err := os.Stat(install_path); err == nil {
		if exe.IsLibrary {
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
	if m == ModeFind || os.Getenv("GDTOOLCHAIN") == "local" || GDPATH == "" {
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
	if !exe.IsLibrary {
		// if the expected version of the tool is already installed in $PATH, then we can
		// just use it.
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
