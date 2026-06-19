# canarybird

CI canary for graphics.gd. Tiny flappy-bird clone exercising 3D, UI,
audio, input, signals, and `ConfigFile` persistence in one binary so
every PR proves the build still works end-to-end on every supported
target. No binary assets; all meshes are Godot primitives, all audio is
synthesised at runtime.

```sh
cd examples/canarybird
gdnext run     # play
gdnext build   # cross-compile + export
gdnext test    # headless test pipeline
```
