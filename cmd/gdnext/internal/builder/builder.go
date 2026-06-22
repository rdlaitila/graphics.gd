package builder

import (
	"fmt"

	"graphics.gd/product"

	"github.com/samber/do/v2"
	"runtime.link/api/xray"
)

// Builder is the per-platform build interface. Each implementation is
// a DI struct that resolves its BuildEnv + ToolCatalog via samber/do,
// so methods stay free of context-shaped arguments.
type Builder interface {
	Run(args ...string) error
	Build(args ...string) error
	BuildMain(args ...string) error
	Test(args ...string) error
}

var (
	_ Builder = (*Linux)(nil)
	_ Builder = (*Windows)(nil)
	_ Builder = (*MacOS)(nil)
	_ Builder = (*IOS)(nil)
	_ Builder = (*Android)(nil)
	_ Builder = (*MetaQuest)(nil)
	_ Builder = (*Browser)(nil)
	_ Builder = (*Musl)(nil)
)

// Provides is the package-level provider set for every builder.
// Register on the root injector before resolving a Builder via For.
var Provides = do.Package(
	do.Lazy(NewLinux),
	do.Lazy(NewWindows),
	do.Lazy(NewMacOS),
	do.Lazy(NewIOS),
	do.Lazy(NewAndroid),
	do.Lazy(NewMetaQuest),
	do.Lazy(NewBrowser),
	do.Lazy(NewMusl),
)

// For returns the Builder responsible for env. LinkMode wins: any
// (*, LibGodot) target routes to *Musl (same recipe regardless of
// GOOS); (*, GDExtension) dispatches per GOOS.
//
// env is passed explicitly so setup.ForBuild can dispatch on a
// musl-mutated copy without re-registering the injector's BuildEnv.
// The selected builder still resolves its OWN env via DI, which is
// fine: the only field musl-detection mutates is Target.LinkMode,
// and that's only read here for dispatch.
func For(di do.Injector, env product.BuildEnv) (Builder, error) {
	if env.Target.LinkMode.Has(product.LibGodot) {
		return do.Invoke[*Musl](di)
	}
	switch env.Target.GOOS {
	case product.GOOSLinux:
		return do.Invoke[*Linux](di)
	case product.GOOSWindows:
		return do.Invoke[*Windows](di)
	case product.GOOSDarwin:
		return do.Invoke[*MacOS](di)
	case product.GOOSIOS:
		return do.Invoke[*IOS](di)
	case product.GOOSAndroid:
		return do.Invoke[*Android](di)
	case product.GOOSMetaQuest:
		return do.Invoke[*MetaQuest](di)
	case product.GOOSJS:
		return do.Invoke[*Browser](di)
	default:
		return nil, xray.New(fmt.Errorf("gdnext: unsupported (%s, %s)", env.Target.GOOS, env.Target.LinkMode))
	}
}
