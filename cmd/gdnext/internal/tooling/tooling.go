package tooling

import "github.com/samber/do/v2"

var Provides = do.Package(
	do.Lazy(NewCatalog),
)
