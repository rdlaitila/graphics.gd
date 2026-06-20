package setup

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/schollz/progressbar/v3"
	"graphics.gd/cmd/gdnext/internal/tooling"
	"graphics.gd/product"
	"runtime.link/api/xray"
)

// AssertTemplate ensures the Godot export templates for the supplied version are
// present at the platform-specific install location, downloading them from
// GitHub releases if not. Returns nil immediately on platforms without a
// known install location (so e.g. android hosts skip the check).
func AssertTemplate(env product.BuildEnv, version string) error {
	var godot string
	switch env.Host.GOOS {
	case product.GOOSLinux:
		godot = "godot"
	case product.GOOSWindows, product.GOOSDarwin:
		godot = "Godot"
	default:
		return nil
	}
	location := filepath.Join(env.Host.UserAppdataRoot, godot, "export_templates", version+".stable")
	url := "https://github.com/godotengine/godot/releases/download/" + version + "-stable/Godot_v" + version + "-stable_export_templates.tpz"
	if _, err := os.Stat(location); err == nil {
		return nil
	}
	dest := filepath.Join(filepath.Dir(location), version+".stable.download")
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
		req.Header.Set("User-Agent", "graphics.gd/cmd/gdnext")
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
				return fmt.Errorf("unable to resume download of 'export templates' (required for 'gdnext build'), please delete %v and try again\nGET %s HTTP status: %v", dest, url, resp.StatusCode)
			}
		default:
			return fmt.Errorf(
				"unable to download 'export templates' (required for 'gdnext build') and not found in %s, please install them, ie. %v\nGET %s HTTP status: %v",
				filepath.Dir(location), "https://godotengine.org/download/linux/", url, resp.StatusCode,
			)
		}
		if resp.StatusCode != 416 {
			bar := progressbar.DefaultBytes(
				resp.ContentLength,
				"gdnext: downloading export templates",
			)
			if _, err := io.Copy(io.MultiWriter(out, bar), resp.Body); err != nil {
				return xray.New(err)
			}
		}
		if err := tooling.ExtractArchive(dest, location, "zip", "", true); err != nil {
			return xray.New(err)
		}
		return nil
	}(); err != nil {
		return xray.New(err)
	}
	if err := os.Remove(dest); err != nil {
		return xray.New(err)
	}
	return nil
}
