// Package builder hosts the per-platform Builder implementations
// (Linux, Windows, MacOS, IOS, Android, MetaQuest, Browser, LibGodot).
// Each implementation is a DI struct resolved via samber/do; the
// `NewX` constructors are pure wiring registered in Provides and
// consumed by For, which dispatches to the right builder for a
// given BuildEnv.
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
	do.Lazy(NewLibGodot),
)

// For returns the Builder responsible for env. Dispatch is per-GOOS;
// the selected builder is responsible for branching on LinkMode
// internally (linux, e.g., folds libgodot single-file builds into
// its own methods by branching on LinkMode + GD_LIBGODOT_LIBC).
func For(di do.Injector, env product.BuildEnv) (Builder, error) {
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
