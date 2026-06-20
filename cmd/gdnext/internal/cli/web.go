package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"graphics.gd/cmd/gdnext/internal/project"

	"github.com/urfave/cli/v3"
	"runtime.link/api/xray"
)

func webCmd() *cli.Command {
	return &cli.Command{
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
				Action: webServe,
			},
		},
	}
}

func webServe(_ context.Context, cmd *cli.Command) error {
	if err := project.Setup(func() error { return nil }); err != nil {
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
