package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"graphics.gd/cmd/gdnext/internal/project"
	"graphics.gd/cmd/gdnext/internal/tooling"
	"graphics.gd/product"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
	"graphics.gd/cmd/gdnext/internal/shared"
	"runtime.link/api/xray"
)

// WebCommand wires `gdnext web`. Runtime state lives on *WebActions.
type WebCommand struct {
	*cli.Command
	Injector do.Injector `do:""`
}

// WebActions carries the runtime state for the wasm dev server.
type WebActions struct {
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
						Sources: cli.EnvVars(product.EnvPort),
					},
				},
				Action: shared.BindAction(t.Injector, (*WebActions).serve),
			},
		},
	}
	return t, nil
}

// NewWebActions resolves the runtime state for web.
func NewWebActions(di do.Injector) (*WebActions, error) {
	return do.InvokeStruct[*WebActions](di)
}

func (t *WebActions) serve(_ context.Context, cmd *cli.Command) error {
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
