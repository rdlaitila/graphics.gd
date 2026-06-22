// Package shared collects small helpers
package shared

import (
	"context"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
	"runtime.link/api/xray"
)

// BindAction returns a cli.ActionFunc that lazily resolves a runtime
// actions struct of type T from di and dispatches to one of its
// methods (expressed via a method expression).
//
// Usage:
//
//	Action: shared.BindAction(t.Injector, (*BuildActions).build),
//
// The method expression converts `func (a *BuildActions) build(ctx,
// cmd) error` into `func(*BuildActions, ctx, cmd) error`, which
// BindAction adapts to cli.ActionFunc.
func BindAction[T any](di do.Injector, fn func(T, context.Context, *cli.Command) error) cli.ActionFunc {
	return func(ctx context.Context, cmd *cli.Command) error {
		t, err := do.Invoke[T](di)
		if err != nil {
			return xray.New(err)
		}
		return fn(t, ctx, cmd)
	}
}
