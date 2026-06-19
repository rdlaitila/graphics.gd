// canarybird is a graphics.gd CI canary: a flappy-bird-style game where a
// canary flaps through a sky of clouds. Touches every subsystem we care
// about (UI, 2D, 3D, audio, input, animation, persistence, Go-defined
// classdb types, signals) without any committed binary assets.
//
// If any subsystem regresses on any platform, the canary fails to build
// or fails to start — the maintainer evacuates the PR.
//
// See examples/canarybird/Readme.md for the full design rationale.
package main

import (
	"graphics.gd/classdb"
	"graphics.gd/classdb/SceneTree"
	"graphics.gd/startup"
)

func main() {
	classdb.Register[CanaryBird]()
	startup.LoadingScene()
	game := new(CanaryBird)
	SceneTree.Add(game.AsNode())
	startup.Scene()
}
