# Plan: gdnext — urfave/cli/v3 POC refactor of cmd/gd

Build `cmd/gdnext/` as a parallel, fully-featured CLI on `urfave/cli/v3` that
preserves every existing `gd` usage path, fixes the messy ad-hoc dispatch in
`cmd/gd/main.go`, and exposes the now-buried subsystems (toolchain download,
keystore, APK packaging, SDK staging, Web serve, refactor) as first-class
subcommands. Keep the existing `cmd/gd` binary intact during the POC so we
can iterate side by side and only flip the default once `gdnext` reaches
parity.

---

## Audit findings (why this needs a rewrite)

### `cmd/gd` top-level surface
- **All dispatch is ad-hoc string matching** on `os.Args`
  ([main.go:139](../../cmd/gd/main.go#L139), [main.go:271](../../cmd/gd/main.go#L271)) — two separate
  dispatch sites, no flag parser, no help text.
- **Environment is the only "configuration channel"**: `GOOS`, `GOARCH`, `CC`,
  `CGO_ENABLED`, `RUNNING_INSIDE_GODOT`, `HOME`, `APPDATA`. Many are
  `os.Setenv`'d mid-flow ([main.go:73–101](../../cmd/gd/main.go#L73), [main.go:217–236](../../cmd/gd/main.go#L217))
  with no centralized contract.
- **`builderFor(goos)`** ([main.go:65–105](../../cmd/gd/main.go#L65)) accepts ~15 alias strings
  (`linux/ubuntu/arch/debian/nix/musl/windows/win/darwin/macos/ios/iphone/
  android/metaquest/quest/meta/browser/js/web/wasm`) but there's no help, no
  list, no validation surface.
- **Test flag translation** is a one-off table ([main.go:112–128](../../cmd/gd/main.go#L112)) that
  rewrites Go-style `-bench`, `-run`, etc. into `-test.*` form.
- **`gd fix` exists in `deprecated.go` but is never wired** ([deprecated.go:46](../../cmd/gd/deprecated.go#L46))
  — the help text *advertises* it but typing `gd fix` falls through to
  `go fix`.
- **No `--help`, no version flag, no shell completion**.

### Buried functional areas (good candidates for new subcommands)
From `internal/`:
- **`tooling/`**: 15 external tools (Godot, Zig, Go, LLVM, apksigner, adb,
  upx, apktool, aapt2, bundletool, vpk, android.jar, LibGodot,
  LibGodotEditor, ldd) with auto-download/cache/version logic. Currently only
  triggered transitively on `Lookup()`.
- **`cryptic/`**: deterministic RSA-4096 keystore generation, JAR/APK v1
  signing, PKCS#7 envelope, X.509 utilities, custom zipslicer for APK
  in-place modification. Triggered only as a side effect of `gd build` on
  Android.
- **`builder/metaquest.go`**: APK injection (OpenXR loader, GodotVR vendor,
  classes2.dex), AndroidManifest.xml patching, zipalign. Buried inside a
  single Run/Build flow.
- **`builder/browser.go`**: HTTP server with COEP/COOP headers, wasm_exec.js
  injection. No way to invoke independently.
- **`refactor/eg/`**: golang.org/x/tools example-based refactor engine,
  embedded `deprecated.txt` rules. `gd fix` not exposed.
- **`gdpaths/`**: `$GDPATH/{bin,lib}` discovery — no `gd config` UI.

---

## Porting requirement

**Every line of the legacy `cmd/gd/*.go` source must be carried forward into
`cmd/gdnext`** — none of the existing functionality is dropped during this
refactor. Specifically:

- `cmd/gd/main.go` — `gd()` body, `builderFor()`, `testArgs()`, the
  no-args editor launch, the musl auto-detect closures, and the commented-
  out tool-autoinstall helpers (`findProjectGoMod`, `readGoModGraphicsVersion`,
  `ensureGoToolGd`) all carry over. The autoinstall helpers stay as
  unexported helpers ready to be re-enabled.
- `cmd/gd/docs.go` — `doc`, `findGdDocMatches`, `snakeToPascal` ⇒ `doc/`.
- `cmd/gd/export_templates.go` — `AssertExportTemplates` ⇒ `internal/templates/`.
- `cmd/gd/deprecated.go` — `fix`, `pkgsImporter`, *and* `checkForFixes` (the
  hint shown when compile errors look like deprecated-API misuses) ⇒ `fix/`.
  `checkForFixes` is currently dead code inside `cmd/gd` but was clearly
  intended for the auto-tool-installer; it ports as an exported helper so a
  future build-failure hook can call it.
- `cmd/gd/deprecated.txt` ⇒ embedded inside `fix/`.

The refactor optimises for **legibility and ease of extension**:
- One concern per file. No 300-line `main.go`.
- Subcommand-scoped logic lives in a folder named after the verb
  (`gdnext build` → `cmd/gdnext/build/`). Each folder is a package that
  exports a single `Command() *cli.Command` constructor. `main.go` does
  nothing but assemble `Command()` calls into a tree.
- Genuinely shared helpers live under `cmd/gdnext/internal/` so they can be
  imported by every subcommand package without polluting the main package.
- The legacy `internal/builder`, `internal/tooling`, `internal/cryptic`,
  `internal/project`, `internal/refactor`, `internal/gdpaths` packages
  under `cmd/gd/` stay untouched — `gdnext` just gives them a clean CLI face.

## Target architecture

```
cmd/
├── gd/                              # untouched legacy CLI
└── gdnext/
    ├── main.go                      # package main, root cli.Command tree only
    ├── internal/                    # shared helpers (importable by every verb)
    │   ├── cliflags/cliflags.go     # global --goos/--goarch/--cc/... flags
    │   ├── platform/platform.go     # GOOS alias → builder.Builder + Builder iface
    │   ├── setup/setup.go           # SetupForBuild pipeline (env, musl detect, project.Setup, docgen)
    │   ├── passthrough/passthrough.go # ToGo(): CommandNotFound → tooling.Go.Exec
    │   ├── shortflags/shortflags.go # `-foo` → `--foo` Go-style rewriter (+ test)
    │   ├── templates/templates.go   # AssertExportTemplates
    │   ├── testargs/testargs.go     # translate Go-style test flags to -test.* form
    │   └── projmod/projmod.go       # findProjectGoMod, readGoModGraphicsVersion, ensureGoToolGd
    ├── editor/editor.go             # root Action — no-args = launch Godot
    ├── build/build.go               # gdnext build, gdnext export
    ├── run/run.go                   # gdnext run
    ├── test/test.go                 # gdnext test
    ├── doc/doc.go                   # gdnext doc <symbol>
    ├── fix/fix.go + deprecated.txt  # gdnext fix + checkForFixes hint
    ├── version/version.go           # gdnext version
    ├── project/project.go           # gdnext project {init,info,presets,version}
    ├── toolchain/toolchain.go       # gdnext toolchain {list,install,path,doctor,exec}
    ├── android/android.go           # gdnext android {sdk-setup,keystore,apk,install,logcat}
    ├── ios/ios.go                   # gdnext ios {sdk-setup,xcode-gen}
    ├── macos/macos.go               # gdnext macos {sdk-setup,lipo,codesign}
    ├── web/web.go                   # gdnext web {serve,wasm-exec}
    └── musl/musl.go                 # gdnext musl {setup,patch-malloc}
```

All subcommand packages depend on `cmd/gdnext/internal/...` and on the
existing `cmd/gd/internal/...` packages. No edits to `cmd/gd/` files.

### Implementation notes — divergence from initial layout

The plan above describes the *intended* one-folder-per-verb structure.
The actual implementation collapsed several files and packages; the CI
plan, file references, and verification steps below all match the
**as-built** layout, which is:

```
cmd/gdnext/
├── main.go                          # root cli.Command + goPassthrough sniff
└── internal/
    ├── cli/                         # ONE package, one file per verb
    │   ├── commands.go              # Commands(), Version(), helpRequested(), LaunchEditor()
    │   ├── flags.go + flags_test.go # Global(), PromoteFlagsToEnv, CollectFlagNames, RewriteShortFlags
    │   ├── build.go                 # buildCmd() + exportCmd()
    │   ├── run.go, test.go, doc.go, fix.go, version.go
    │   ├── project.go, toolchain.go
    │   └── android.go, ios.go, macos.go, web.go, musl.go
    ├── builder/   ⎤
    ├── cryptic/   ⎥  Mirrored copies of cmd/gd/internal/* — the legacy
    ├── gdpaths/   ⎥  packages were duplicated rather than imported in
    ├── project/   ⎥  place. Both trees compile against the same source
    ├── refactor/  ⎥  shape for now; a future cleanup can collapse them
    ├── tooling/   ⎦  into a shared cmd/internal/ once gd is retired.
    ├── platform/                    # GOOS alias → builder.Builder
    ├── projmod/                     # findProjectGoMod helpers (parked)
    ├── setup/                       # ForBuild() pipeline
    └── templates/                   # Assert() for export templates
```

Practical consequences for the rest of this document:

- The "shortflags preprocessor" and "global flags" live together in
  [cmd/gdnext/internal/cli/flags.go](../../cmd/gdnext/internal/cli/flags.go);
  the unit tests in [flags_test.go](../../cmd/gdnext/internal/cli/flags_test.go).
- Passthrough to `go` is split: a `goPassthrough` sniff in
  [cmd/gdnext/main.go](../../cmd/gdnext/main.go) handles the common case
  (so the editor `Action` doesn't swallow unknown verbs), and a
  `CommandNotFound` hook is the fallback.
- The mirrored `cmd/gdnext/internal/{builder,cryptic,gdpaths,project,refactor,tooling}`
  packages compile and run independently of the originals under
  `cmd/gd/internal/`. The CI workflow path-filter watches `cmd/gdnext/**`
  alone — changes to `cmd/gd/internal/*` don't trigger `cli-gdnext`.

---

## Subcommand tree (self-documenting via `--help`)

```
gdnext                                  (no args → launch Godot editor)
gdnext build           [GOOS/GOARCH]    cross-compile shared lib + assert templates
gdnext run             [GOOS/GOARCH]    build + run (or serve, or adb install)
gdnext test            [GOOS/GOARCH]    cross-compile + run tests
gdnext export          [GOOS/GOARCH]    full distributable (BuildMain)
gdnext doc <symbol>                     enhanced go doc + //gd: lookup
gdnext fix                              apply deprecated.txt eg refactors
gdnext version                          gdnext + go + godot + GDExtension versions
gdnext help [command]                   built-in

gdnext project init                     create graphics/, project.godot, presets
gdnext project info                     name, dirs, version, IncludesGo
gdnext project presets                  rewrite export_presets.cfg from embed
gdnext project version [<v>]            read/set config/version in project.godot

gdnext toolchain list                   names, versions, install state
gdnext toolchain install [<tool>]       force download (all or one)
gdnext toolchain path <tool>            absolute path resolution
gdnext toolchain doctor                 verify every tool is callable
gdnext toolchain exec <tool> -- ...     raw passthrough with arg conversion

gdnext android sdk-setup                stage adb/apksigner/aapt2 stubs
gdnext android keystore create          generate ~/.godot/keystores/debug.keystore
gdnext android keystore show            print cert/alias/expiry
gdnext android apk sign <apk>           apksigner v1
gdnext android apk verify <apk>         verify signature
gdnext android apk packagename <apk>    aapt2 dump packagename
gdnext android install <apk>            adb install + launch
gdnext android logcat                   adb logcat filtered to project pid

gdnext ios sdk-setup                    unpack bundled iOS SDK
gdnext ios xcode-gen                    generate .xcodeproj from go.xcframework

gdnext macos sdk-setup                  unpack bundled macOS SDK
gdnext macos lipo <out> <ins…>          merge arch dylibs (konoui/lipo)
gdnext macos codesign <bundle>          codesign --deep

gdnext web serve [--port 8080]          HTTP server with COEP/COOP
gdnext web wasm-exec                    copy wasm_exec.js from GOROOT

gdnext musl setup                       stage musl libc + Zig
gdnext musl patch-malloc                deterministic malloc.c patch
```

Categories (urfave `Category:` field) group these in `--help`:
**core** (build/run/test/export/doc/fix/version), **project**, **toolchain**,
**android**, **apple** (ios/macos), **web**, **linux** (musl).

---

## Solving the `-foo` Go-style flag constraint

urfave/cli/v3's `UseShortOptionHandling: true` allows combining short flags
(`-som`) but **explicitly breaks multi-letter single-dash flags** like
`-foo` — and Go's stdlib `flag` package uses `-foo` for everything. The user
requires Go-style.

**Solution: `shortflags.go` preprocessor**
- Before calling `cmd.Run(ctx, args)`, walk the registered command tree and
  collect every known long-flag name into a set.
- Transform `os.Args`: for any token matching `^-([A-Za-z][A-Za-z0-9_-]+)$`
  whose name is in the known set (after stripping `=value` suffix), rewrite
  the leading `-` to `--`.
- Leave `-h`, `-v` (single-letter), `--foo` (already double), and
  positional/passthrough args alone.
- After a `--` literal, stop rewriting (standard argv convention).
- For the **passthrough path** (`go mod tidy`, etc.), the preprocessor
  must NOT rewrite — handled by `SkipFlagParsing: true` on the catch-all
  command, or by detecting "first non-flag arg is not a known verb" and
  bypassing rewrite entirely.

Tradeoff: the preprocessor is ~40 lines, no external dependency, fully
inspectable. Alternative (forking urfave's parser) is much more code and
locks us to a fork.

---

## Preserving the passthrough contract

`gd` today silently forwards unknown verbs to `go` (`gd mod tidy`,
`gd get foo`, `gd vet ./...`, `gd env`, etc.) — this is documented behaviour
and people rely on it.

**Solution**: set `cli.Command.CommandNotFound = passthroughToGo` on the
root command. urfave calls this when the first positional doesn't match a
registered command. Inside, do `tooling.Go.Exec(args...)`. Because
`SkipFlagParsing: false` on root still allows our top-level flags
(`--goos`, etc) before the unknown verb, this composes cleanly with the
existing pattern.

For the no-args case (launch Godot editor unless `RUNNING_INSIDE_GODOT` is
set), put the logic in the root `Action`.

---

## Global flags (every env var becomes a flag)

| Flag | Env var(s) | Type | Default | Subcommands |
|------|------------|------|---------|-------------|
| `--goos` | `GOOS` | string | runtime.GOOS | build, run, test, export |
| `--goarch` | `GOARCH` | string | runtime.GOARCH | build, run, test, export |
| `--cc` | `CC` | string | auto (zig/clang) | build, run, test, export |
| `--cgo` | `CGO_ENABLED` | bool | `1` | build, run, test, export |
| `--gdpath` | `GDPATH` | string | `~/gd` | toolchain.*, all build verbs |
| `--godot` | `GODOT` | string | auto | all build verbs |
| `--inside-godot` | `RUNNING_INSIDE_GODOT` | bool | false | editor default |
| `--verbose, -v` | `GD_VERBOSE` | bool | false | all |

Per-command flags include the test-flag translation table (`--bench`,
`--run`, `--count`, `--cover`, etc.) declared on the `test` subcommand;
this kills the ad-hoc `testArgs()` rewriter and makes `gdnext test --help`
self-documenting.

---

## Steps

### Phase 1 — Skeleton (parallelizable: 1a/1b/1c)
1a. Create `cmd/gdnext/` with `main.go` that constructs a `cli.Command{Name:
    "gdnext", UseShortOptionHandling: true}` and runs help.
1b. Add `go get github.com/urfave/cli/v3` to `go.mod`.
1c. Write `shortflags.go` preprocessor and unit tests covering: `-goos
    linux` → `--goos linux`, `-goos=linux` → `--goos=linux`, `-h`
    unchanged, `--` boundary, unknown `-zzz` left alone.

### Phase 2 — Core verbs (parity with current `gd`)
*depends on Phase 1*
2a. `cmd/editor.go` — root Action that launches Godot editor (port logic
    from [main.go:254–269](../../cmd/gd/main.go#L254)).
2b. `cmd/build.go` — `build`, `run`, `test`, `export` subcommands, each
    routing through `platform.go`'s `builderFor()` (extracted unchanged
    from [main.go:65–105](../../cmd/gd/main.go#L65)). Wire the Setup pipeline
    ([main.go:155–248](../../cmd/gd/main.go#L155)) into a shared `Before` hook.
2c. `cmd/doc.go` — port `gd doc` from [main.go:148](../../cmd/gd/main.go#L148).
2d. `cmd/fix.go` — wire `gd fix` to the existing `fix()` func in
    [cmd/gd/deprecated.go:46](../../cmd/gd/deprecated.go#L46) (currently dead
    code).
2e. `passthrough.go` — `CommandNotFound` → `tooling.Go.Exec`.
2f. `cmd/version.go` — print gdnext, go, godot, GDExtension versions.

### Phase 3 — New first-class subcommands (parallelizable, one file each)
*depends on Phase 2 (only because they share Before/global flags)*
3a. `cmd/toolchain.go` — `list/install/path/doctor/exec` over the 15
    `tooling.*` toolchain vars. Reflect over the package's exported vars
    to drive `list`.
3b. `cmd/project.go` — `init/info/presets/version`.
3c. `cmd/android.go` — `sdk-setup/keystore/apk/install/logcat` by
    refactoring the relevant helpers in
    [internal/builder/android.go](../../cmd/gd/internal/builder/android.go) into
    exported funcs (or calling them in place from gdnext).
3d. `cmd/ios.go` — `sdk-setup/xcode-gen`.
3e. `cmd/macos.go` — `sdk-setup/lipo/codesign`.
3f. `cmd/web.go` — `serve/wasm-exec` extracted from
    [internal/builder/browser.go](../../cmd/gd/internal/builder/browser.go).
3g. `cmd/musl.go` — `setup/patch-malloc`.

### Phase 4 — Polish
4a. Shell completion via urfave's `EnableShellCompletion: true` +
    document install in Readme.
4b. Suggestions: enable `Suggest: true` so `gdnext buidl` → "did you mean
    build?".
4c. Update `cmd/gd/main.go` to print a one-line deprecation hint pointing
    at `gdnext` (optional; do *not* delete `gd`).

---

## Relevant files

### Existing (read / extract, do not modify in this POC)
- [cmd/gd/main.go](../../cmd/gd/main.go) — `gd()` (line 65), `builderFor()` (line 65),
  `testArgs()` (line 112), dispatch (line 139/271), env setup (155–248).
- [cmd/gd/deprecated.go](../../cmd/gd/deprecated.go) — embedded `deprecated.txt` and
  unused `fix()` to be wired up.
- [cmd/gd/docs.go](../../cmd/gd/docs.go) — `gd doc` lookup logic.
- [cmd/gd/export_templates.go](../../cmd/gd/export_templates.go) — template path
  resolution per OS.
- [cmd/gd/internal/builder/](../../cmd/gd/internal/builder) — 8 Builder
  implementations, each currently with Build/BuildMain/Run/Test.
- [cmd/gd/internal/tooling/](../../cmd/gd/internal/tooling) — 15 toolchains with
  Lookup/LookupPlatform/Exec/Action/Output/CombinedOutput.
- [cmd/gd/internal/cryptic/](../../cmd/gd/internal/cryptic) — keystore, signjar,
  zipslicer, x509tools.
- [cmd/gd/internal/project/project.go](../../cmd/gd/internal/project/project.go) —
  Setup, SetupFile, SetupFiles, SetupVersion, CopyDir, CopyFile.
- [cmd/gd/internal/refactor/eg/](../../cmd/gd/internal/refactor/eg) — example-based
  refactor engine.
- [cmd/gd/internal/gdpaths/gdpaths.go](../../cmd/gd/internal/gdpaths/gdpaths.go) —
  `$GDPATH/{bin,lib}`.

### New (as-built)
- [cmd/gdnext/main.go](../../cmd/gdnext/main.go) — root command + `goPassthrough` sniff.
- [cmd/gdnext/internal/cli/](../../cmd/gdnext/internal/cli/) — one file per
  verb (`build.go`, `run.go`, `test.go`, `export` aliased in `build.go`,
  `doc.go`, `fix.go`, `version.go`, `project.go`, `toolchain.go`,
  `android.go`, `ios.go`, `macos.go`, `web.go`, `musl.go`), plus the
  shared `commands.go` and `flags.go` (+ `flags_test.go`).
- [cmd/gdnext/internal/setup/](../../cmd/gdnext/internal/setup/),
  [platform/](../../cmd/gdnext/internal/platform/),
  [templates/](../../cmd/gdnext/internal/templates/),
  [projmod/](../../cmd/gdnext/internal/projmod/) — top-level helper
  packages imported by the cli verbs.
- `.github/workflows/cli-gdnext.yml` — see [Continuous integration](#continuous-integration).

---

## Verification

1. **Side-by-side parity matrix** — for every documented `gd` usage, run
   the equivalent `gdnext` and diff behaviour:
   - `gd` (editor)
   - `gd build`, `gd run`, `gd test`, `gd doc fmt.Println`
   - `gd mod tidy`, `gd get foo` (passthrough)
   - `GOOS=android gd run`, `GOOS=android GOARCH=amd64 gd run`,
     `GOOS=ios gd run`, `GOOS=web gd run`, `GOOS=metaquest gd run`,
     `GOOS=musl gd build`
   - `gd test -run TestX -count 1 -bench .`
2. **Short-flag preprocessor unit tests** — table-driven in
   `shortflags_test.go` (10+ cases including edge cases: `-goos=linux`,
   `-- -goos linux`, `-v`, `-h`, `-unknown`).
3. **`gdnext --help` output review** — confirm every subcommand and every
   global flag has Usage text and env var listed.
4. **`gdnext toolchain doctor`** — actually invoke each registered tool's
   version flag and confirm install/version detection works.
5. **`go vet ./cmd/gdnext/...`** and `go build ./cmd/gdnext` clean.
6. **Manual smoke**: run `gdnext` (editor), `gdnext build`, `gdnext run`
   in an existing sample project from the samples branch.

---

## Example projects (CI canaries)

Introduce a new top-level `examples/` directory housing self-contained
graphics.gd projects that double as CI canaries. Every PR builds each
example on every target the runners can reach; if a cross-platform
invariant breaks, an example fails to build and the canary "stops
singing".

The naming is not arbitrary. From the late 1800s into the 1980s,
British coal mines literally carried caged canaries underground because
the birds dropped from carbon monoxide and methane long before levels
became fatal to the miners — when the canary went quiet, the shift
evacuated. Our CI plays exactly that role for the codebase: the canary
example is the smallest project that touches every subsystem (UI, 2D,
3D, audio, input, animation, Go ↔ Godot bindings), and if any one of
those subsystems regresses on any platform, the canary stops tweeting
and the maintainer evacuates the PR.

### `examples/canary/` — what it demonstrates

A complete graphics.gd vertical slice, kept small enough to build on
every target in the matrix. Set in a stylised coal mine: a wire-frame
cage hangs from a chain, a sphere "canary" perches inside, an
`OmniLight3D` lantern bathes the scene in warm light. Tap "Listen" and
the canary sings a synthesised chirp; let too long pass without
listening and the CO meter creeps up until an alarm sounds. Pick a song
from the OptionButton to change the chirp pitch, slap "Reset" before
the canary goes silent.

| Subsystem | What canary uses |
|-----------|------------------|
| **3D** | mine walls (`BoxMesh`), cage bars (`CylinderMesh`), canary body (`SphereMesh`), `DirectionalLight3D` (overhead) + `OmniLight3D` (lantern). All primitives from Godot's procedural meshes — no `.glb` files. |
| **2D / UI** | `CanvasLayer` HUD: `Button` ("Listen", "Reset"), `OptionButton` (song picker), `ProgressBar` (CO meter), `Label` readouts. Default theme. |
| **Audio** | positional canary chirps via `AudioStreamPlayer3D` + `AudioStreamGenerator` (procedural sine-wave bursts); ambient mine drone via `AudioStreamPlayer` + same generator at a lower fundamental. No `.ogg` files. |
| **Input** | `InputMap` actions (`listen`, `reset`, `cycle_song`) declared in `project.godot`, bound to keyboard, touch, and gamepad. |
| **Animation** | cage rotation tweened in the `Canary` node's `Process(delta)` callback — no `AnimationPlayer`, the Go callback IS the animation. `CPUParticles3D` for drifting dust (CPU-only so Compatibility renderer + web both work). |
| **graphics.gd Go bindings** | one Go-defined node, `Canary`, with one signal `Tweeted Signal.Solo[string]` and one method `Tweet(song string)`, exercising registration + signal emission + method dispatch round-trip. |
| **Cross-platform persistence** | a single `ConfigFile` write to `user://settings.cfg` storing the last-picked song — proves write access works on every platform's sandbox. |

### Cross-platform constraints

To stay green on every runner-buildable target the canary follows a
strict diet:

- Renderer profile: **Compatibility** (works on web, quest, mobile,
  and desktop without per-target shader variants).
- No compute shaders, no `RenderingDevice`-only APIs, no GLES2-only
  paths.
- **No committed binary assets.** Meshes are Godot's built-in
  primitives; audio is synthesised at runtime by
  `AudioStreamGenerator`; the only committed `.tscn` is the empty
  root scene. This makes diffs reviewable and lets the canary be
  regenerated from text alone.
- No filesystem writes outside `user://`; no `OS.execute`; no
  networking.
- ≤ ~250 LOC of Go total. The canary is a CI fixture, not a tutorial.

### Layout

```
examples/
└── canary/
    ├── go.mod                   # module example.com/canary; require graphics.gd
    ├── main.go                  # classdb.Register[Canary] + startup.LoadingScene/Scene
    ├── canary.go                # type Canary + Tweet method + Tweeted signal + state
    ├── canary_test.go           # one smoke test so `gdnext test` has work to do
    ├── Readme.md
    └── graphics/
        ├── project.godot        # Compatibility renderer, GoMainLoop, input map
        ├── main.tscn            # empty Node3D root; the scene is built in Go
        ├── export_presets.cfg   # covers linux/windows/macos/android/web
        ├── library.gdextension  # points at the built shared libs
        └── .gitignore
```

`project.godot`, `export_presets.cfg`, `library.gdextension`,
`main.tscn`, and `.gitignore` are committed so the example builds
deterministically. `project.Setup` honours pre-existing files (it only
writes a template when the file is absent), so checking them in does
not conflict with `gdnext project init` in fresh projects.

A **`go.work` file at the repository root** lists the canary alongside
the `graphics.gd` module:

```
go 1.26.1
use .
use ./examples/canary
```

This means `go build ./...` from the repo root resolves
`require graphics.gd` in the canary to the local working tree
automatically — useful for local development and for `gopls` so the
editor finds the in-tree library. **CI does not rely on this**: the
workflow runs `go mod edit -replace=graphics.gd=$GITHUB_WORKSPACE`
inside the staged tempdir, which works the same way regardless of
whether `go.work` is present.

### Generalising — adding more examples

`examples/<name>/` is the contract. Any directory that

1. has a `go.mod` depending on `graphics.gd`,
2. has a `project.godot` at its root, and
3. builds cleanly with `gdnext build` when invoked from inside it,

is a valid CI canary. Adding a new one is two changes:

1. drop the project under `examples/<name>/` honouring the constraints
   above,
2. append the directory name to the workflow's `matrix.example` axis.

CI inspects only the build product on disk (the shared library and the
`releases/<goos>/<goarch>/` output of `gdnext build`), not project
contents, so different examples can demonstrate wildly different things
as long as the constraints hold. `fail-fast: false` ensures one
example's regression on one platform never masks another's.

### How CI consumes the examples

The CI section below relies on two pieces tied to this layout:

- A `ci/cli-gdnext/stage-example.sh` helper that copies
  `examples/<name>/` into a tempdir, runs `go mod edit
  -replace=graphics.gd=$GITHUB_WORKSPACE` so the build pins to the PR's
  library code, then `go mod tidy`.
- A workflow matrix axis `example: [canary]` so each
  `(os, example)` pair runs independently. New examples extend the
  axis; nothing else in the workflow changes.

---

## Continuous integration

A dedicated **`.github/workflows/cli-gdnext.yml`** drives every PR that
touches the CLI. The job is named **`cli-gdnext`** today — verbose but
temporary — so the day gdnext replaces the legacy `gd` binary, the
rename is a single search-and-replace (`cli-gdnext` → `cli`,
`cmd/gdnext` → `cmd/cli`). The existing `.github/workflows/go.yml` is
left untouched; gdnext stands on its own.

The workflow's `on:` clause is **path-filtered** to fire only when the
CLI source under `cmd/gdnext/**`, the workflow file itself, or the
module manifests (`go.mod` / `go.sum`) actually change. Unrelated PRs
skip it entirely.

### Scope — verify every invariant the runner can reach

The principle is: **if a runner can produce the artefact, CI produces
it**. The only things excluded are targets that fundamentally need
unavailable infrastructure (Apple signing certs, real Android/iOS
hardware, GPU sessions). Anything that boils down to "cross-compile
with zig + run `godot --headless --export-release`" is in scope, because
templates download once and cache weekly under the same `actions/cache`
pattern as `~/gd`.

Three toolchains have to be present before the build matrix runs;
`gdnext toolchain install` fetches them and the cache amortises the
~535 MB across the week:

- **`zig`** (~50 MB) — `CC` for every cross-compile in `builder/*`.
- **`godot`** (~85 MB) — needed by `project.Setup` (it stamps
  `gdextension_version` from `godot --version`) and by every `BuildMain`
  (it shells `godot --headless --export-release`).
- **Godot export templates** (~400 MB once per Godot version) — pulled
  by `templates.Assert` on first `gdnext build`; cached under the OS-
  specific install dir (`~/.local/share/godot/export_templates/...` on
  Linux, `~/Library/Application Support/Godot/export_templates/...` on
  macOS, `%APPDATA%/Godot/export_templates/...` on Windows).

#### What every runner runs (unconditional)

| Class | Examples | How |
|-------|----------|-----|
| **Compile** | `go build ./cmd/gdnext/...`, `go vet ./cmd/gdnext/...` | one step |
| **Unit tests** | `internal/cli/flags_test.go` shortflag table + any future `cli/*_test.go` | `go test ./cmd/gdnext/...` |
| **Help text** | `gdnext --help`, `gdnext <verb> --help` exit 0 with non-empty stdout | shell loop |
| **Diagnostic verbs** | `gdnext version`, `gdnext toolchain list`, `gdnext toolchain doctor` (no --fix) | shell loop |
| **Go passthrough** | `gdnext mod help`, `gdnext env GOVERSION`, `gdnext list std` exit 0 | shell loop |
| **Short-flag rewrite** | `gdnext -goos linux build --help` shows build help, not a parser error | shell assertion |
| **Project bootstrap** | `gdnext project init` in a scratch dir produces `graphics/project.godot` + `export_presets.cfg` | tempdir fixture |
| **Toolchain doctor is diagnostic-only** | `gdnext toolchain doctor` finishes without downloading anything (size of `$GDPATH/bin` unchanged) | before/after `du -s` |
| **Toolchain install fetches** | `gdnext toolchain install zig`/`godot` populate `$GDPATH/bin/` and `--version` works | cached week-over-week |
| **Headless test pipeline** | `gdnext test` in the example project compiles a native test `.so`/`.dll`/`.dylib` and runs `godot --headless` | the canary has no Go tests, so the run is a no-op success; value is exercising the full setup-and-dispatch path of `test.go` |

#### Build matrix — every target the runner can actually export

Each row produces both the shared library (`graphics/<goos>_<goarch>.*`)
and the full distributable (`releases/<goos>/<goarch>/<example>.*`) via
`gdnext build`, which calls `templates.Assert` → `platform.BuildMain` →
`godot --headless --export-release`. Targets are gated with `if:
matrix.os == ...` so each runner only attempts what it can succeed at.
The `example` matrix axis (today `[canary]`, see
[Example projects](#example-projects-ci-canaries)) cross-products with
the OS axis so every example builds on every supported OS.

| Target | ubuntu-latest | windows-latest | macos-latest | Notes |
|--------|---------------|----------------|--------------|-------|
| **linux/amd64** | native | zig cross | zig cross | every runner builds it |
| **linux/arm64** | zig cross | — | — | one runner is enough |
| **windows/amd64** | zig cross | native | zig cross | every runner builds it |
| **darwin/arm64** | — | — | native (unsigned) | macOS-only, no codesign in CI |
| **js/wasm** | yes | yes | yes | Go wasm backend + Godot web template, host-agnostic |
| **android/arm64** | yes | — | — | ubuntu has JDK; uses our deterministic cryptic keystore + auto-downloaded apksigner/aapt2; ~300 MB SDK adds to weekly cache |
| **musl/amd64** | yes | — | — | zig+musl target, linux-only |

This matrix exercises **8 of the 9 builders** (`linux`, `windows`,
`macos`, `browser`, `android`, `musl`; plus implicit coverage of
`metaquest` since it's an Android variant). The 9th — `ios` — is the
only platform left out; see below.

### What's deliberately NOT in CI

- **`gdnext ios` end-to-end.** `xcode-gen` + `xcodebuild` works on
  macOS runners in principle, but a real `.ipa` needs an Apple
  developer cert + provisioning profile, neither of which can live in
  a public repo. CI runs `gdnext ios --help` and `gdnext ios xcode-gen
  --dry-run`; the rest is a local / release-time concern.
- **Signed macOS bundles.** `gdnext macos codesign` needs a real cert.
  The unsigned `darwin/arm64` `.app` produced above proves the export
  pipeline; signing is verified by the release pipeline outside CI.
- **`gdnext run` for any target.** Needs a windowed Godot session;
  even with `xvfb-run` the GPU/audio dependencies on the public runners
  are flaky. The `gdnext test` step exercises the same setup pipeline
  headlessly, which is the part that depends on our CLI code.
- **`gdnext android install` / `gdnext android logcat`.** Needs a real
  device or emulator. `apk sign` and `keystore create` *are* exercised
  transitively by the `android/arm64` build above.
- **`gdnext web serve`.** Just an HTTP server with COEP/COOP headers;
  unit-test target rather than CI-job target.

### Triggers + cost

Path-filtered `push` (release, staging) + `pull_request` (release).
Three runners × ~8 minutes per CI pass on a cache hit (Go cache
warm-up + per-platform exports), ~15 minutes on a cache-miss week
(zig + godot + templates + android SDK download). Within the GitHub
free-tier minutes for public repos. `fail-fast: false` so a single
OS-specific flake doesn't mask issues on the other two. Because the
workflow is path-filtered, PRs that don't touch `cmd/gdnext/**` skip
it entirely.

### Workflow shape

The workflow lives at
[.github/workflows/cli-gdnext.yml](../../.github/workflows/cli-gdnext.yml).
Every non-trivial step delegates to a shell script under
[ci/cli-gdnext/](../../ci/cli-gdnext) so each one can be run by hand on
real hardware for debugging — they take their inputs as args and read
`GITHUB_WORKSPACE` (or the `GRAPHICS_GD_ROOT` fallback) for the
graphics.gd checkout location.

| Workflow step | Script | What it asserts |
|---------------|--------|-----------------|
| Build + vet + unit tests | [build-vet-test.sh](../../ci/cli-gdnext/build-vet-test.sh) | `go build/vet/test ./cmd/gdnext/...` clean |
| Help text smoke | [help-text.sh](../../ci/cli-gdnext/help-text.sh) | `--help` exits 0 with the verb's name on every registered command |
| Diagnostic verbs (pre-install) | [diagnostic-verbs.sh](../../ci/cli-gdnext/diagnostic-verbs.sh) | `version` works; `toolchain list` lists all 15 catalog entries; `toolchain doctor` does not mutate `$GDPATH` |
| Go passthrough | [go-passthrough.sh](../../ci/cli-gdnext/go-passthrough.sh) | `gdnext env`/`list std`/`mod help` forward to `go` |
| Short-flag rewrite | [short-flag-rewrite.sh](../../ci/cli-gdnext/short-flag-rewrite.sh) | `-goos linux build --help` is rewritten to `--goos` and resolves to the build verb |
| Toolchain install (zig + godot) | [toolchain-install.sh](../../ci/cli-gdnext/toolchain-install.sh) | both tools install, `toolchain path` resolves them, post-install `doctor` reports both `OK` |
| Scratch / staged example | [stage-example.sh](../../ci/cli-gdnext/stage-example.sh) | copies `examples/<name>/` to a tempdir, overwrites the committed `replace graphics.gd => ../..` with `replace=graphics.gd=$GITHUB_WORKSPACE`, runs `go mod tidy`. Asserts the example shipped a `graphics/project.godot` and a `graphics/main.tscn` |
| Build `<goos>/<goarch>` (matrix) | [build-target.sh](../../ci/cli-gdnext/build-target.sh) | `gdnext build` produces `graphics/<goos>_<goarch>.{so,dll,dylib,wasm}` and (where applicable) `releases/<goos>/<goarch>/`. Android branch additionally runs `apksigner verify --print-certs` against the signed apk to exercise the cryptic keystore + apksigner + aapt2 + apktool + bundletool + android.jar pipeline. Example-agnostic — takes a project dir and operates on it |
| Headless test pipeline | [test-headless.sh](../../ci/cli-gdnext/test-headless.sh) | `gdnext test` exits 0 against the example (no-op success when the example has no Go tests — value is in exercising the test verb's setup pipeline) |

Steps that remain inline in the workflow are pure GitHub Actions wiring
that wouldn't be useful as standalone scripts: `actions/checkout`,
`actions/setup-go`, the week key for cache busting, the per-OS export-
templates dir resolution (it writes to `$GITHUB_OUTPUT`), `actions/cache`
itself, `Install gdnext into PATH` (it writes to `$GITHUB_PATH`), and
the `mktemp -d` + `$GITHUB_OUTPUT` plumbing around the staged-example
step.

To run the same checks locally before pushing (substitute `canary` with
any example under `examples/`):

```bash
go install ./cmd/gdnext
export PATH="$(go env GOPATH)/bin:$PATH"
export GRAPHICS_GD_ROOT="$(pwd)"

ci/cli-gdnext/build-vet-test.sh
ci/cli-gdnext/help-text.sh
ci/cli-gdnext/diagnostic-verbs.sh
ci/cli-gdnext/go-passthrough.sh
ci/cli-gdnext/short-flag-rewrite.sh
ci/cli-gdnext/toolchain-install.sh

tmp=$(mktemp -d)
ci/cli-gdnext/stage-example.sh canary "$tmp"
ci/cli-gdnext/build-target.sh linux amd64 "$tmp"
# … and so on per platform you want to verify
ci/cli-gdnext/test-headless.sh "$tmp"
```

---

## Decisions

- **`gdnext` lives alongside `gd`** — POC, no breaking change. Both
  binaries compile from the same module; promotion is a future decision.
- **No `internal/` refactor in this POC** — all existing packages used as-is.
  If Android helpers need exporting for `gd android apk sign`, that's a
  *minimal* addition (move funcs from package-private to package-exported
  identifiers) deferred until phase 3c blocks on it.
- **`-foo` Go-style flags supported via preprocessor**, not via a urfave
  fork. Single-letter `-h`, `-v` still work normally.
- **Passthrough to `go` preserved** via `CommandNotFound`. This means any
  unknown subcommand is forwarded; help shows our registered verbs but
  doesn't lie about the others.
- **No new external dependencies** other than `github.com/urfave/cli/v3`.
- **Subcommand surface is intentionally not exhaustive** — every helper
  inside the builders is not a subcommand. We expose the ones with clear
  standalone value (toolchain mgmt, keystore, APK ops, web serve, refactor).

---

## Further considerations

1. **Should `gdnext build` produce the *executable* (current BuildMain) or
   the *shared library* (current Build)?** The current `gd build` does
   BuildMain, but the name "build" for a Go dev evokes `go build` which
   produces a binary. **Recommendation**: keep `gdnext build` = current
   `gd build` behaviour (BuildMain — produces the distributable), and add
   `gdnext compile` as an alias / lower-level verb for just the shared
   library step. Option A: identical to today (build=BuildMain only).
   Option B: split build (lib) / export (full bundle). Option C: both
   verbs, with build doing the full thing for compatibility.
2. **Verb naming for GOOS-aware commands**: today you say
   `GOOS=android gd run`. With subcommands we *could* support
   `gdnext android run` as sugar. **Recommendation**: yes, but as
   aliases — the canonical form stays `gdnext run --goos android`, with
   `gdnext android run` being a thin wrapper that pre-sets `--goos
   android`. This keeps a single code path.
3. **Embed `urfave/cli/v3` shell completions in the install flow?**
   `gd` today has none. **Recommendation**: ship the completion scripts as
   embedded resources and add `gdnext completion {bash,zsh,fish}` to print
   them — opt-in install, no system mutation by default.
