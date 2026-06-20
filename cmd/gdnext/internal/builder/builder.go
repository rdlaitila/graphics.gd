package builder

import (
	"fmt"

	"graphics.gd/product"
	"runtime.link/api/xray"
)

// Builder is the union of every method any of the per-platform Builder types
// expose to gdnext. Each method takes a product.BuildEnv up front so the
// (host, target) pair is explicit at the API boundary — the methods do not
// re-read $GOOS / $GOARCH for their own decisions.
type Builder interface {
	Run(env product.BuildEnv, args ...string) error
	Build(env product.BuildEnv, args ...string) error
	BuildMain(env product.BuildEnv, args ...string) error
	Test(env product.BuildEnv, args ...string) error
}

// Compile-time assertions
var (
	_ Builder = Linux{}
	_ Builder = Windows{}
	_ Builder = MacOS{}
	_ Builder = IOS{}
	_ Builder = Android{}
	_ Builder = MetaQuest{}
	_ Builder = Browser{}
	_ Builder = (*Musl)(nil)
)

// For returns the Builder responsible for the supplied target. The
// LinkMode axis takes precedence: every (*, LibGodot) target routes to
// the libgodot (current musl) builder regardless of GOOS, since the
// recipe (link Go's c-archive against a per-target libgodot.*.a) is the
// same. (*, GDExtension) dispatches per GOOS to the existing builders.
// Returns an error when nothing in the matrix matches; the caller
// surfaces it to the user.
func For(env product.BuildEnv) (Builder, error) {
	if env.Target.LinkMode.Has(product.LibGodot) {
		return &Musl{}, nil
	}
	switch env.Target.GOOS {
	case product.GOOSLinux:
		return Linux{}, nil
	case product.GOOSWindows:
		return Windows{}, nil
	case product.GOOSDarwin:
		return MacOS{}, nil
	case product.GOOSIOS:
		return IOS{}, nil
	case product.GOOSAndroid:
		return Android{}, nil
	case product.GOOSMetaQuest:
		return MetaQuest{}, nil
	case product.GOOSJS:
		return Browser{}, nil
	default:
		return nil, xray.New(fmt.Errorf("gdnext: unsupported (%s, %s)", env.Target.GOOS, env.Target.LinkMode))
	}
}
