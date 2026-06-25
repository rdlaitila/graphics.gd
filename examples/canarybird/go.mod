module example.com/canarybird

go 1.26.1

require graphics.gd v0.0.0-00010101000000-000000000000

require (
	github.com/samber/lo v1.53.0 // indirect
	github.com/tetratelabs/wazero v1.8.2 // indirect
	golang.org/x/mod v0.30.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/text v0.32.0 // indirect
	golang.org/x/tools v0.39.0 // indirect
	runtime.link v0.0.0-20250814043127-466c6970c4a5 // indirect
)

replace graphics.gd => ../..
