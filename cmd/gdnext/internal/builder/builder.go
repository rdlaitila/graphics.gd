package builder

import (
	"fmt"

	"graphics.gd/product"
	"runtime.link/api/xray"
)

// Builder is the per-platform build interface. Methods take a
// product.BuildEnv so the (host, target) pair is explicit and not
// re-read from $GOOS / $GOARCH.
type Builder interface {
	Run(env product.BuildEnv, args ...string) error
	Build(env product.BuildEnv, args ...string) error
	BuildMain(env product.BuildEnv, args ...string) error
	Test(env product.BuildEnv, args ...string) error
}

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

// For returns the Builder responsible for env. LinkMode wins: any
// (*, LibGodot) target routes to Musl{} (same recipe regardless of
// GOOS); (*, GDExtension) dispatches per GOOS.
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
