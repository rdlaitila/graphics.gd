package builder

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"graphics.gd/cmd/gdnext/internal/project"
	"graphics.gd/cmd/gdnext/internal/tooling"
	"graphics.gd/product"

	"github.com/samber/do/v2"
	"runtime.link/api/xray"
)

// Browser drives wasm/js builds + a local dev server.
type Browser struct {
	BuildEnv    product.BuildEnv `do:""`
	ToolCatalog tooling.Catalog  `do:""`
	testing     bool
	handler     http.Handler
}

func NewBrowser(di do.Injector) (*Browser, error) {
	return do.InvokeStruct[*Browser](di)
}

func (t *Browser) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
	w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
	t.handler.ServeHTTP(w, r)
}

func (t *Browser) Build(args ...string) error {
	os.Remove(filepath.Join(project.GraphicsDirectory, "library.gdextension"))
	if err := os.MkdirAll(filepath.Join(project.ReleasesDirectory, "js", "wasm"), 0755); err != nil {
		return xray.New(err)
	}
	if !project.IncludesGo {
		return nil
	}
	if err := t.AssertExportTemplate(); err != nil {
		return xray.New(err)
	}
	if err := os.Setenv(product.EnvGOARCH, product.GOARCHWasm); err != nil {
		return xray.New(err)
	}
	if t.testing {
		return t.ToolCatalog.Go.Action("test", args, "-c", "-o", filepath.Join(project.ReleasesDirectory, "js", "wasm", "library.wasm"))
	}
	return t.ToolCatalog.Go.Action("build", args, "-o", filepath.Join(project.ReleasesDirectory, "js", "wasm", "library.wasm"))
}

func (t *Browser) Run(args ...string) error {
	os.Remove(filepath.Join(project.GraphicsDirectory, "library.gdextension"))
	if err := t.Build(args...); err != nil {
		return xray.New(err)
	}
	if err := os.Chdir(project.GraphicsDirectory); err != nil {
		return xray.New(err)
	}
	if err := t.ToolCatalog.Godot.Exec("--headless", "--export-release", "Web"); err != nil {
		return xray.New(err)
	}
	PORT := os.Getenv(product.EnvPort)
	if PORT == "" {
		PORT = "8080"
	}
	if project.IncludesGo {
		wasm_exec_path := filepath.Join(project.ReleasesDirectory, "js", "wasm", "wasm_exec.js")
		wasm_exec, err := os.ReadFile(wasm_exec_path)
		if err != nil {
			return xray.New(err)
		}
		var argsQuoted = []string{`"js"`}
		for _, arg := range args {
			if arg == "-x" {
				continue
			}
			argsQuoted = append(argsQuoted, strconv.Quote(arg))
		}
		wasm_exec = bytes.ReplaceAll(wasm_exec, []byte("this.argv = [\"js\"]"), []byte("this.argv = ["+strings.Join(argsQuoted, ", ")+"]"))
		if err := os.WriteFile(wasm_exec_path, wasm_exec, 0644); err != nil {
			return xray.New(err)
		}
	}
	fmt.Println("gd: serving wasm/js on http://localhost:" + PORT)
	t.handler = http.FileServer(http.Dir(filepath.Join(project.ReleasesDirectory, "js", "wasm")))
	http.Handle("/", t)
	return xray.New(http.ListenAndServe(":"+PORT, nil))
}

func (t *Browser) BuildMain(args ...string) error {
	if err := t.Build(args...); err != nil {
		return xray.New(err)
	}
	if err := os.Chdir(project.GraphicsDirectory); err != nil {
		return xray.New(err)
	}
	return t.ToolCatalog.Godot.Exec("--headless", "--export-release", "Web")
}

func (t *Browser) Test(args ...string) error {
	os.Remove(filepath.Join(project.GraphicsDirectory, "library.gdextension"))
	t.testing = true
	converted := []string{}
	for _, arg := range os.Args[2:] {
		switch arg {
		case "-bench", "-benchmem", "-benchtime", "blockprofile",
			"-blockprofilerate", "-count", "-coverprofile", "-cpu",
			"-cpuprofile", "-failfast", "-fullpath", "-fuzz", "-fuzzcachedir",
			"-fuzzminimizetime", "-fuzztime", "-fuzzworker", "-gocoverdir",
			"-list", "-memprofile", "-memprofilerate", "-mutexprofile",
			"-mutexprofilefraction", "-outputdir", "-paniconexit0",
			"-parallel", "-run", "-short", "-shuffle", "-skip", "-testlogfile",
			"-timeout", "-trace", "-v":
			converted = append(converted, "-test."+strings.TrimPrefix(arg, "-"))
		default:
			converted = append(converted, arg)
		}
	}
	return t.Run(converted...)
}

func (t *Browser) AssertExportTemplate() error {
	if err := os.MkdirAll(filepath.Join(project.GraphicsDirectory, ".godot", "public"), 0o755); err != nil {
		return xray.New(err)
	}
	path := filepath.Join(project.GraphicsDirectory, "..", "releases", "js", "wasm", "wasm_exec.js")
	GOROOT, err := t.ToolCatalog.Go.Output("env", "GOROOT")
	if err != nil {
		return xray.New(err)
	}
	wasm_exec, err := os.Open(filepath.Join(GOROOT, "lib", "wasm", "wasm_exec.js"))
	if err != nil && !os.IsNotExist(err) {
		return xray.New(err)
	}
	err1 := err
	if err != nil {
		wasm_exec, err = os.Open(filepath.Join(GOROOT, "misc", "wasm", "wasm_exec.js"))
		if err != nil {
			return xray.New(errors.Join(err1, err))
		}
	}
	defer wasm_exec.Close()
	out, err := os.Create(path)
	if err != nil {
		return xray.New(err)
	}
	defer out.Close()
	if _, err := io.Copy(out, wasm_exec); err != nil {
		return xray.New(err)
	}
	template_path := filepath.Join(project.GraphicsDirectory, ".godot", "godot.web.template_release.wasm32.zip")
	stat, statErr := os.Stat(template_path)
	resp, err := http.Get("https://release.graphics.gd/godot.web.template_release.wasm32.zip")
	if err != nil && !os.IsNotExist(err) {
		return xray.New(err)
	} else {
		defer resp.Body.Close()
	}
	last_modified, err := time.Parse(http.TimeFormat, resp.Header.Get("Last-Modified"))
	if err != nil {
		return xray.New(err)
	}
	if (os.IsNotExist(statErr) || (statErr == nil && last_modified.After(stat.ModTime()))) && resp.Body != nil {
		fmt.Println("gd: downloading latest https://release.graphics.gd/godot.web.template_release.wasm32.zip")
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return xray.New(err)
		}
		if err := os.WriteFile(template_path, data, 0o644); err != nil {
			return xray.New(err)
		}
	} else if resp.Body == nil || resp.StatusCode != 200 {
		return fmt.Errorf("gd: failed to download godot.web.template_release.wasm32.zip: %v (is your gd command out of date?)", resp.Status)
	}
	return nil
}
