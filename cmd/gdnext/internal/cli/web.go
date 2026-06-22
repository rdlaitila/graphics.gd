package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"graphics.gd/cmd/gdnext/internal/project"
	"graphics.gd/cmd/gdnext/internal/tooling"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
	"runtime.link/api/xray"
)

// WebCommand exposes `gdnext web`: WebAssembly serving and template
// helpers.
type WebCommand struct {
	*cli.Command
	ToolCatalog tooling.Catalog `do:""`
}

// NewWebCommand constructs the `gdnext web` subcommand
func NewWebCommand(di do.Injector) (*WebCommand, error) {
	t := do.MustInvokeStruct[*WebCommand](di)
	t.Command = &cli.Command{
		Name:  "web",
		Usage: "WebAssembly serving and template helpers",
		Commands: []*cli.Command{
			{
				Name:  "serve",
				Usage: "serve releases/js/wasm/ over HTTP with COEP/COOP headers",
				Flags: []cli.Flag{
					&cli.IntFlag{
						Name:    "port",
						Aliases: []string{"p"},
						Value:   8080,
						Usage:   "TCP port to listen on",
						Sources: cli.EnvVars("PORT"),
					},
				},
				Action: t.serve,
			},
		},
	}
	return t, nil
}

func (t *WebCommand) serve(_ context.Context, cmd *cli.Command) error {
	if err := project.Setup(t.ToolCatalog, func() error { return nil }); err != nil {
		return err
	}
	port := fmt.Sprint(cmd.Int("port"))
	root := filepath.Join(project.ReleasesDirectory, "js", "wasm")
	if _, err := os.Stat(root); err != nil {
		return fmt.Errorf("nothing to serve at %s — run `gdnext build --goos web` first: %w", root, err)
	}
	fs := http.FileServer(http.Dir(root))
	srv := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		fs.ServeHTTP(w, r)
	})
	fmt.Println("gdnext: serving wasm/js on http://localhost:" + port)
	return xray.New(http.ListenAndServe(":"+port, srv))
}
