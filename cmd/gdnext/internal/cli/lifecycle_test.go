package cli

import (
	"context"
	"os"
	"testing"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"

	"graphics.gd/cmd/gdnext/internal/builder"
	"graphics.gd/cmd/gdnext/internal/ci"
	"graphics.gd/cmd/gdnext/internal/setup"
	"graphics.gd/cmd/gdnext/internal/tooling"
	"graphics.gd/product"
)

// TestBuildEnvSeesPromotedFlag verifies the Construction/Runtime split:
// `--goos linux --goarch arm64` set BEFORE the verb must be visible on
// the *BuildActions.BuildEnv resolved INSIDE the Action handler. Pre-fix,
// *BuildCommand cached BuildEnv at startup before promoteFlagsToEnv ran,
// so the Target.GOOS/GOARCH always reflected the host. Post-fix, Before
// promotes flags to env AND clears the BuildEnv cache, so the Actions
// struct gets a fresh BuildEnv per invocation.
func TestBuildEnvSeesPromotedFlag(t *testing.T) {
	t.Setenv("GOOS", "")
	t.Setenv("GOARCH", "")
	t.Setenv("GD_NO_DOWNLOAD", "1")

	di := do.New()
	do.ProvideValue(di, di)
	Provides(di)
	setup.Provides(di)
	tooling.Provides(di)
	builder.Provides(di)
	ci.Provides(di)

	root, err := do.Invoke[*RootCommand](di)
	if err != nil {
		t.Fatalf("invoke root: %v", err)
	}

	var got product.BuildEnv
	root.Command.Commands = append(root.Command.Commands, &cli.Command{
		Name: "envprobe",
		Action: bindActionEnvProbe(di, &got),
	})

	saved := os.Args
	defer func() { os.Args = saved }()
	if err := root.Run(context.Background(), []string{"gdnext", "--goos", "linux", "--goarch", "arm64", "envprobe"}); err != nil {
		t.Fatalf("run: %v", err)
	}

	if got.Target.GOOS != product.GOOSLinux {
		t.Errorf("Target.GOOS = %q, want %q", got.Target.GOOS, product.GOOSLinux)
	}
	if got.Target.GOARCH != product.GOARCHArm64 {
		t.Errorf("Target.GOARCH = %q, want %q", got.Target.GOARCH, product.GOARCHArm64)
	}
}

func bindActionEnvProbe(di do.Injector, out *product.BuildEnv) cli.ActionFunc {
	return func(_ context.Context, _ *cli.Command) error {
		env, err := do.Invoke[product.BuildEnv](di)
		if err != nil {
			return err
		}
		*out = env
		return nil
	}
}
