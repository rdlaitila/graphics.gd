package builder

import (
	"fmt"
	"os"
	"path/filepath"

	"graphics.gd/cmd/gdnext/internal/project"
	"graphics.gd/cmd/gdnext/internal/tooling"
	"graphics.gd/product"

	"runtime.link/api/xray"
)

type Linux struct{}

func (Linux) Build(env product.BuildEnv, args ...string) error {
	if !project.IncludesGo {
		return nil
	}
	if env.HostGOOS != "linux" || env.HostGOARCH != env.TargetGOARCH {
		zig, err := tooling.Zig.Lookup()
		if err != nil {
			return xray.New(err)
		}
		switch env.TargetGOARCH {
		case "amd64":
			if err := os.Setenv("CC", zig+" cc -target x86_64-linux-gnu"); err != nil {
				return xray.New(err)
			}
		case "arm64":
			if err := os.Setenv("CC", zig+" cc -target aarch64-linux-gnu"); err != nil {
				return xray.New(err)
			}
		default:
			return fmt.Errorf("gd build: cannot cross-compile linux %v on %v", env.TargetGOARCH, env.HostGOOS)
		}
	}
	return tooling.Go.Action("build", args, "-buildmode=c-shared", "-o", filepath.Join(project.GraphicsDirectory, fmt.Sprintf("linux_%v.so", env.TargetGOARCH)))
}

func (linux Linux) BuildMain(env product.BuildEnv, args ...string) error {
	if err := linux.Build(env, args...); err != nil {
		return xray.New(err)
	}
	var export []string
	switch env.TargetGOARCH {
	case "amd64":
		export = []string{"--headless", "--export-release", "Linux x86_64"}
	case "arm64":
		export = []string{"--headless", "--export-release", "Linux arm64"}
	default:
		return fmt.Errorf("gd export: cannot export linux %v", env.TargetGOARCH)
	}
	if err := os.Chdir(project.GraphicsDirectory); err != nil {
		return xray.New(err)
	}
	if err := tooling.Godot.Exec(export...); err != nil {
		return xray.New(err)
	}
	return nil
}

func (linux Linux) Run(env product.BuildEnv, args ...string) error {
	if env.HostGOOS != "linux" || env.HostGOARCH != env.TargetGOARCH {
		return fmt.Errorf("gd run: cannot run linux/%v executable on %s", env.TargetGOARCH, env.HostTuple())
	}
	if err := linux.Build(env, args...); err != nil {
		return xray.New(err)
	}
	if err := os.Chdir(project.GraphicsDirectory); err != nil {
		return xray.New(err)
	}
	return tooling.Godot.Exec(args...)
}

func (Linux) Test(env product.BuildEnv, args ...string) error {
	if env.HostGOOS != "linux" || env.HostGOARCH != env.TargetGOARCH {
		return fmt.Errorf("gd test: cannot run linux/%v tests on %s", env.TargetGOARCH, env.HostTuple())
	}
	if err := tooling.Go.Action("test", args, "-c", "-buildmode=c-shared", "-o", filepath.Join(project.GraphicsDirectory, fmt.Sprintf("linux_%v.so", env.TargetGOARCH))); err != nil {
		return xray.New(err)
	}
	if err := os.Chdir(project.GraphicsDirectory); err != nil {
		return xray.New(err)
	}
	args = append(args, "--headless")
	return tooling.Godot.Exec(args...)
}
