# CLI

> This doc describes the next-generation CLI shipped in
> [`cmd/gdnext/`](../cmd/gdnext/). The original `gd` CLI at
> [`cmd/gd/`](../cmd/gd/) still exists during the transition and
> continues to serve existing projects unchanged. `gdnext` is the
> next version of `gd`; the two coexist until parity is reached, then
> `gd` retires. Invocation examples below use the current binary name
> (`gdnext`); everything else is generic to "the CLI".

The CLI is a drop-in `go` replacement for graphics.gd projects: it
handles the Godot-specific verbs (build, run, test, ...) in-process
and forwards everything else to the underlying `go` toolchain. It
sits on top of three catalogued layers:

- The catalog (`product/`) — what targets / hosts / link modes exist.
  See [catalog.md](catalog.md).
- The toolchain manager (`gdnext toolchain ...`) — the external
  programs the CLI drives. See [toolchains.md](toolchains.md).
- The CI driver (`gdnext ci ...`) — the verbs the GitHub Actions
  workflow calls. See [workflow.md](workflow.md).

- [CLI](#cli)
  - [Concepts](#concepts)
    - [Drop-in `go` replacement](#drop-in-go-replacement)
    - [Target selection](#target-selection)
    - [Link mode](#link-mode)
    - [`$GDPATH`](#gdpath)
    - [Short-flag rewrite](#short-flag-rewrite)
    - [Go passthrough](#go-passthrough)
  - [Top-level verbs](#top-level-verbs)
    - [`build` / `export`](#build--export)
    - [`run`](#run)
    - [`test`](#test)
    - [`doc`](#doc)
    - [`fix`](#fix)
    - [`version`](#version)
    - [`project`](#project)
    - [`platform`](#platform)
    - [`toolchain`](#toolchain)
    - [`android`](#android)
    - [`ios`](#ios)
    - [`macos`](#macos)
    - [`web`](#web)
    - [`musl`](#musl)
    - [`ci`](#ci)
  - [Runbook](#runbook)
    - [Build for the current host](#build-for-the-current-host)
    - [Cross-build for another target](#cross-build-for-another-target)
    - [Run the project under Godot](#run-the-project-under-godot)
    - [Run headless tests](#run-headless-tests)
    - [Deploy to a connected Android device](#deploy-to-a-connected-android-device)
    - [Serve a WebAssembly build locally](#serve-a-webassembly-build-locally)
    - [Initialise a fresh project](#initialise-a-fresh-project)
  - [Development](#development)
    - [Where the CLI lives](#where-the-cli-lives)
    - [Adding a verb](#adding-a-verb)
    - [Verb conventions](#verb-conventions)

Anything unfamiliar is forwarded straight to `go`:

```
gdnext mod tidy        # → go mod tidy
gdnext get example.com # → go get example.com
gdnext vet ./...       # → go vet ./...
```

```
$ gdnext --help
COMMANDS:
   build      cross-compile and produce a distributable binary (Godot --export-release)
   run        build the project as a shared library and launch it via Godot (or adb / web server)
   test       cross-compile and run go tests inside the Godot runtime
   export     alias for 'build' — produce a distributable binary via Godot export
   doc        go doc with //gd: tag lookup against classdb
   fix        rewrite code to migrate from deprecated graphics.gd APIs
   version    print CLI, go, and godot versions
   project    initialise and inspect a graphics.gd project
   toolchain  manage the external programs the CLI drives
   platform   show the graphics.gd platform / host / target matrix
   android    Android device and APK helpers
   ios        iOS-specific helpers
   macos      macOS-specific helpers (lipo, codesign)
   web        WebAssembly serving and template helpers
   musl       static-musl Linux build helpers
   ci         CI helpers for the GitHub Actions workflow
```

## Concepts

### Drop-in `go` replacement

graphics.gd-specific verbs are handled in-process; everything else is
forwarded to the underlying `go` toolchain. A short-flag rewrite shim
lets `go`-shaped argv (`-goos linux`) keep working under urfave's
`--goos linux` convention.

### Target selection

A target is a `(GOOS, GOARCH, LinkMode)` triple. Resolved per
invocation from (in order of precedence):

1. Explicit flags: `--goos`, `--goarch`, `--link` (alias
   `--linkmode`).
2. Env vars: `GOOS`, `GOARCH`, `GOLINK` — the first two match what
   the `go` toolchain reads.
3. Catalog defaults from `product/`: `GOOSArchDefaults` picks an
   architecture for a GOOS that didn't specify one;
   `GOOSLinkModeDefaults` picks the link mode.
4. Fallback to the current host's `(GOOS, GOARCH)` and `GDExtension`.

Aliases work everywhere: `--goos=ubuntu` resolves to `linux`,
`--goos=quest` resolves to `metaquest`, etc. See
[catalog.md → Aliases](catalog.md#aliases).

### Link mode

Every catalog row supports one or more link modes:

- `gdextension` — produces a shared library Godot loads at runtime
  via `library.gdextension`. The default for every target today.
- `libgodot` — produces a static executable that embeds the engine
  by linking against `libgodot.<goos>.<goarch>.<ext>`. Currently
  only `linux/amd64` ships a published artefact.

See [catalog.md → Link modes](catalog.md#link-modes) for the
catalog side.

### `$GDPATH`

`$GDPATH` (default `~/gd`) is where the CLI caches downloaded
toolchains and library artefacts. Override via `--gdpath` or the env
var. Layout:

```
$GDPATH/
  bin/        gd-managed executables (godot, zig, llvm, adb, ...)
  lib/        gd-managed libraries (libgodot.*.a, android.jar)
  checksums/  per-tool sha256 sidecars
```

See [toolchains.md → GDPath layout](toolchains.md#gdpath-layout).

### Short-flag rewrite

The `go` toolchain accepts single-dash multi-char flags
(`-goos linux`); urfave only recognises `--goos linux`. The CLI
rewrites the former into the latter before urfave sees the argv,
so muscle memory from `go` keeps working:

```
gdnext build -goos linux -goarch arm64       # ok
gdnext build --goos linux --goarch arm64     # ok
GOOS=linux gdnext build                      # ok
```

The rewrite covers every global flag the CLI registers. The CI verb
`gdnext ci check-short-flag-rewrite` pins the behaviour.

### Go passthrough

If `gdnext <verb>` doesn't match a registered subcommand, the
remaining argv is forwarded to the underlying `go` toolchain:

```
gdnext mod tidy           # → go mod tidy
gdnext get example.com/m  # → go get example.com/m
gdnext vet ./...          # → go vet ./...
gdnext env GOOS           # → go env GOOS
```

The `gdnext ci check-go-passthrough` verb covers this in CI; see
[workflow.md → `go-passthrough`](workflow.md#go-passthrough).

## Top-level verbs

### `build` / `export`

Cross-compile the project and run a Godot release export. `export`
is a thin alias for `build` — same action, different name for
discoverability. Both verbs accept `-- go-build-flags...` after a
`--` separator so extra flags reach `go build` unchanged.

```
gdnext build
gdnext build --goos linux --goarch arm64
gdnext build --link libgodot
gdnext export --goos windows -- -trimpath
```

Output lands under `releases/<goos>/<goarch>/`. The
`gdnext ci build-target` verb asserts artefacts at exactly this
location.

### `run`

Build the project as a shared library and launch it. On desktop
that's `godot -e` against the staged `graphics/` directory; on
android it's `adb install` + launch; on web it's a local HTTP
server with COEP/COOP.

```
gdnext run
gdnext run --goos android   # builds + adb installs + launches
gdnext run --goos js        # builds + serves on localhost
```

### `test`

Cross-compile and run go tests inside the Godot runtime. Known
`-flag` arguments get rewritten to `-test.flag` form, and tests get
the full engine API.

```
gdnext test
gdnext test --goos linux --link libgodot
gdnext test -- -run TestSpecific -v
```

### `doc`

`go doc` with extra resolution for graphics.gd's `//gd:`-tagged
symbols.

```
gdnext doc Node
gdnext doc graphics.gd/Animation Player
```

### `fix`

Rewrites code to migrate from deprecated graphics.gd APIs. Wraps
the rewrite rules under `cmd/gdnext/internal/refactor`.

```
gdnext fix
```

### `version`

Prints the CLI, go, and godot versions.

```
$ gdnext version
gdnext version v0.0.0-20260623022512-cb749d1fcc03+dirty
go version go1.26.4 linux/amd64
godot expected version 4.6.2
godot installed 4.6.2.stable.official.71f334935 at /home/me/gd/bin/godot
```

`expected` is `product.ToolchainGodot.Version`; `installed` comes
from running `godot --version`. A mismatch signals a toolchain
catalog drift — re-run `gdnext toolchain install`.

### `project`

Initialise and inspect a graphics.gd project.

```
gdnext project init       # create graphics/, project.godot, presets
gdnext project info       # print resolved metadata
gdnext project version    # print config/version from project.godot
```

### `platform`

Show the graphics.gd platform / host / target matrix. The full
verb tree (filters, single-row detail, structured output) lives in
[catalog.md → Verbs](catalog.md#verbs):

```
gdnext platform                       # full matrix
gdnext platform linux                 # single-row detail
gdnext platform --hosts               # only hosts
gdnext platform --targets             # only targets
gdnext platform -f markdown           # paste-ready table
```

### `toolchain`

Manage the external programs the CLI drives. Full reference in
[toolchains.md](toolchains.md):

```
gdnext toolchain list             # catalog view
gdnext toolchain doctor           # host probe
gdnext toolchain install          # fetch missing into $GDPATH
gdnext toolchain uninstall --all  # drop gd-managed installs
gdnext toolchain path <slug>      # resolve a binary
```

### `android`

Android device + APK helpers. Mostly thin wrappers around the
bundled `adb` from the toolchain catalog.

```
gdnext android adb devices
gdnext android install app.apk    # adb install
gdnext android logcat             # stream logcat
gdnext android apk inspect app.apk
gdnext android keystore           # TODO: manage the debug keystore
```

If a system `adb` is on `$PATH`, the CLI defers to it.

### `ios`

iOS-specific helpers. Requires a macOS host for codesign + lipo.

```
gdnext ios xcode-gen   # generate releases/ios/<arch>/<project>.xcodeproj
```

`xcode-gen` runs `gdnext build --goos ios` under the hood and then
materialises an Xcode project around the produced artefact.

### `macos`

macOS export helpers (lipo, codesign).

```
gdnext macos lipo       # merge per-arch dylibs into a universal one
gdnext macos codesign   # TODO: extract codesign --deep from builder.MacOS
```

### `web`

WebAssembly serving and template helpers.

```
gdnext web serve   # serve releases/js/wasm/ with COEP/COOP headers
```

The COEP/COOP headers are what `SharedArrayBuffer` requires in the
browser; serving any other way will silently break threads.

### `ci`

CI helpers for the GitHub Actions workflow. Full reference in
[workflow.md → CI verbs](workflow.md#ci-verbs).

```
gdnext ci build-matrix               # emit the build matrix as JSON
gdnext ci build-target               # build one (goos, goarch) cell
gdnext ci play-cell                  # drive a built example via the play-bot
gdnext ci workflow-summary           # render the rolling step summary
gdnext ci check-toolchain-checksums  # harvest SHAs for KnownChecksums
...
```

Not meant for day-to-day use — these verbs read `$GITHUB_*` env
vars, expect a CI scratch dir, and emit JSON for `fromJSON()`.
Reproducing a failing CI cell locally is fine via the runbook in
[workflow.md → Reproduce a failing matrix cell locally](workflow.md#reproduce-a-failing-matrix-cell-locally).

## Runbook

### Build for the current host

```
gdnext build
```

Output: `releases/<goos>/<goarch>/<project>` — the path is keyed on
the target tuple. When no flags are passed, the target defaults to
the current host.

### Cross-build for another target

```
gdnext build --goos windows --goarch amd64
gdnext build --goos android                 # arm64 default
gdnext build --goos js                      # wasm default
gdnext build --goos linux --link libgodot   # static, embedded engine
```

Targets the host can build are listed in
[catalog.md → Hosts vs targets](catalog.md#hosts-vs-targets);
`gdnext platform --targets` shows the live set. If the target needs
a toolchain you don't have yet, `gdnext toolchain install <slug>`
or the bulk `gdnext toolchain install` fills the gap.

### Run the project under Godot

```
gdnext run
gdnext run --goos android      # adb install + launch
gdnext run --goos js           # local HTTP server with COEP/COOP
```

### Run headless tests

```
gdnext test
gdnext test -- -run TestSpecific -v
gdnext test --goos linux --link libgodot
```

Tests execute inside the Godot runtime, so they have full access to
the engine API.

### Deploy to a connected Android device

```
gdnext build --goos android
gdnext android install releases/android/arm64/<project>.apk
gdnext android logcat | grep <tag>
```

Or in one step:

```
gdnext run --goos android
```

### Serve a WebAssembly build locally

```
gdnext build --goos js
gdnext web serve
# → http://localhost:<port>/ with COEP/COOP headers set
```

### Initialise a fresh project

```
mkdir myproject && cd myproject
gdnext project init
gdnext run
```

## Development

### Where the CLI lives

- [cmd/gdnext/main.go](../cmd/gdnext/main.go) — root command
  assembly, DI registration, short-flag rewrite, go passthrough.
- [cmd/gdnext/internal/cli/](../cmd/gdnext/internal/cli/) — one Go
  file per top-level verb (`build.go`, `run.go`, `test.go`,
  `project.go`, `toolchain.go`, ...). Each file owns its DI structs,
  command wiring, and action functions.
- [cmd/gdnext/internal/builder/](../cmd/gdnext/internal/builder/) —
  per-platform builders, reading from the standalone `product/`
  catalog rather than hard-coded constants.
- [cmd/gdnext/internal/tooling/](../cmd/gdnext/internal/tooling/) —
  runtime side of the toolchain catalog (download, verify, exec).
  Catalog data lives in [product/](../product/). See
  [toolchains.md](toolchains.md).
- [cmd/gdnext/internal/ci/](../cmd/gdnext/internal/ci/) — CI verbs.
  See [workflow.md](workflow.md).

The legacy `gd` CLI at [cmd/gd/](../cmd/gd/) is kept independent —
neither tree imports the other. Changes to the next-gen CLI never
touch the legacy tree and vice versa, so a regression in one can't
break the other.

### Adding a verb

1. Create `cmd/gdnext/internal/cli/<name>.go` with the standard
   shape: `XCommand` + `XActions` DI structs, `NewXCommand` +
   `NewXActions` constructors, an `(*XActions).action` method bound
   via `shared.BindAction`. Look at any existing verb (e.g.
   [version.go](../cmd/gdnext/internal/cli/version.go)) for the
   template.
2. Register both in the CLI `Provides` set in
   [cli.go](../cmd/gdnext/internal/cli/cli.go):

   ```go
   do.Lazy(NewXCommand),
   do.Lazy(NewXActions),
   ```

3. Add the command to the root's `Commands:` list.
4. If the verb takes a target, declare it in the catalog
   (`product/`) rather than hard-coding tuples in the verb.

### Verb conventions

- **One verb per task.** If a boolean flag re-purposes the verb,
  split it into siblings instead.
- **Read state from `product/`, not env vars or hardcoded switches.**
  Every fact about a target should round-trip through the catalog.
- **Banner output matches CI.** `gdnext ci <verb>` prints
  `==> name args` banners; user-facing verbs follow the same shape so
  logs read consistently whether run locally or in CI.
- **Wrap errors with `runtime.link/api/xray`.** Every build-pipeline
  error path uses `xray.New(err)` so failures carry context rather
  than a single line.
- **No interactive prompts.** The CLI must run unattended in CI.
  Anything that needs a value comes through a flag or env var.
