# CLI

graphics.gd ships two side-by-side Go CLIs that both stand in for
`go` when working on a Godot-based project:

- **`gd`** ([cmd/gd/](../cmd/gd/)) — the original, in-production CLI.
  Auto-installs the supported Godot version, lays down a `graphics/`
  Godot project, and forwards every unrecognised verb to `go`.
- **`gdnext`** ([cmd/gdnext/](../cmd/gdnext/)) — the next-generation
  rebuild of the same idea. Code was copied out of `cmd/gd/` as a
  starting point and refactored on `urfave/cli/v3` + samber/do
  dependency injection, with the toolchain catalog lifted into the
  standalone `product/` package. First-class subcommand trees for
  the subsystems `gd` keeps internal (toolchain management, project
  scaffolding, per-platform helpers, CI verbs).

The two binaries deliberately share no code. `gdnext`'s builders,
tooling, and project setup live entirely under
[cmd/gdnext/internal/](../cmd/gdnext/internal/); `gd`'s equivalents
stay under [cmd/gd/internal/](../cmd/gd/internal/). The split is to
prevent cross-pollination — a change in `gdnext` cannot accidentally
break `gd` and vice versa. They reach the same on-disk locations
(`~/gd` for the toolchain cache, `releases/<goos>/<goarch>/` for build
output) because both follow the published `$GDPATH` convention, not
because they call the same code.

`gdnext` will run alongside `gd` until it has proven out across every
supported target the workflow exercises; then `gd` retires. Until
then, day-to-day work can use either.

`gdnext` sits on top of three other catalogued layers:

- The catalog (`product/`) — what targets / hosts / link modes exist.
  See [catalog.md](catalog.md).
- The toolchain manager (`gdnext toolchain ...`) — the external
  programs gdnext drives. See [toolchains.md](toolchains.md).
- The CI driver (`gdnext ci ...`) — the verbs the GitHub Actions
  workflow calls. See [workflow.md](workflow.md).

- [CLI](#cli)
  - [Why two CLIs](#why-two-clis)
  - [`gd` — the existing CLI](#gd--the-existing-cli)
    - [Verbs](#verbs)
    - [Anything else](#anything-else)
  - [`gdnext` — the next-generation CLI](#gdnext--the-next-generation-cli)
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
    - [Pick a CLI for the task](#pick-a-cli-for-the-task)
    - [Build for the current host](#build-for-the-current-host)
    - [Cross-build for another target](#cross-build-for-another-target)
    - [Run the project under Godot](#run-the-project-under-godot)
    - [Run headless tests](#run-headless-tests)
    - [Deploy to a connected Android device](#deploy-to-a-connected-android-device)
    - [Serve a WebAssembly build locally](#serve-a-webassembly-build-locally)
    - [Initialise a fresh project](#initialise-a-fresh-project)
  - [Development](#development)
    - [Where each CLI lives](#where-each-cli-lives)
    - [Adding a verb to `gdnext`](#adding-a-verb-to-gdnext)
    - [Verb conventions](#verb-conventions)

## Why two CLIs

`gd` works. It has shipped countless builds and is the version every
existing graphics.gd project depends on. Replacing it in-place would
mean accepting an unbounded blast radius on any regression.

`gdnext` started as a copy of `gd`'s tree and was refactored under
three constraints:

1. **Catalogued state.** Targets, link modes, toolchains, and their
   pinned versions live in a standalone `product/` package as plain
   data. The CLI looks them up rather than hard-coding
   `switch goos`. See [catalog.md](catalog.md).
2. **First-class subsystems.** Things `gd` keeps internal —
   toolchain management, project scaffolding, per-platform helpers,
   CI orchestration — get their own subcommand trees so they're
   discoverable via `--help` instead of source-diving.
3. **Verifiable CI.** Every CI step is a `gdnext ci <verb>` call,
   not inline shell. Locally and in CI run the same Go code. See
   [workflow.md](workflow.md).

The two trees stay independent on purpose: `gdnext` evolves without
risking `gd`'s production behaviour, and `gd` stays maintainable
without having to keep up with `gdnext`'s refactors. The side-by-side
stance lasts until `gdnext` has proven parity across every supported
`(host, target, link-mode)` cell. The workflow at
[.github/workflows/gdnext.yml](../.github/workflows/gdnext.yml)
exists to do exactly that.

## `gd` — the existing CLI

[cmd/gd/main.go](../cmd/gd/main.go). One binary, hand-rolled argv
dispatch, drop-in for `go` on graphics.gd projects.

```
gd              # auto-install Godot, build the lib, launch the editor
gd build        # cross-compile for $GOOS/$GOARCH, place under releases/
gd run          # build the shared lib and launch via Godot
gd test         # cross-compile + run tests inside the Godot runtime
gd doc          # `go doc` with //gd: tag lookup against classdb
gd version      # gd's own version + the go version
```

### Verbs

| Verb              | Behaviour                                                                  |
| ----------------- | -------------------------------------------------------------------------- |
| (none)            | Set up the project (download Godot if missing, write `graphics/`, build the shared lib) and launch the Godot editor. Flags are forwarded to `godot -e`. |
| `build`           | Cross-compile to `releases/<goos>/<goarch>/`. Asserts export templates are installed for the current Godot version first. |
| `run`             | Build the project as a shared lib and launch it via the platform's `Run` driver (godot on desktop, adb on android, http server on web). |
| `test`            | Cross-compile and run `go test`s inside the Godot runtime. Rewrites known `-flag` arguments to `-test.flag` form. |
| `doc`             | `go doc` with extra resolution for `//gd:`-tagged symbols.                 |
| `version`         | Prints `gd version <vcs>` plus delegates to `go version`.                  |

Target selection is `$GOOS` / `$GOARCH` (or the host defaults). The
`builderFor(goos)` switch in [cmd/gd/main.go](../cmd/gd/main.go)
maps GOOS aliases (`ubuntu`, `quest`, `win`, `wasm`, `musl`, ...) to
the right `builder.X` struct in [cmd/gd/internal/builder](../cmd/gd/internal/builder/).

### Anything else

Anything not on the verb list is forwarded straight to the `go`
toolchain:

```
gd mod tidy        # → go mod tidy
gd get example.com # → go get example.com
gd vet ./...       # → go vet ./...
```

That's what makes `gd` a drop-in `go` replacement. `gdnext` keeps
the same shape — see [Go passthrough](#go-passthrough).

## `gdnext` — the next-generation CLI

[cmd/gdnext/main.go](../cmd/gdnext/main.go) plus
[cmd/gdnext/internal/cli/](../cmd/gdnext/internal/cli/). Started as a
copy of `gd`'s tree, rebuilt as a urfave/cli/v3 subcommand tree with
DI-resolved state and catalog-driven target validation. The internal
packages (`builder/`, `tooling/`, `project/`, `setup/`, ...) under
[cmd/gdnext/internal/](../cmd/gdnext/internal/) are independent of
`gd`'s equivalents — changes to one do not touch the other.

```
$ gdnext --help
COMMANDS:
   build      cross-compile and produce a distributable binary (Godot --export-release)
   run        build the project as a shared library and launch it via Godot (or adb / web server)
   test       cross-compile and run go tests inside the Godot runtime
   export     alias for 'build' — produce a distributable binary via Godot export
   doc        go doc with //gd: tag lookup against classdb
   fix        rewrite code to migrate from deprecated graphics.gd APIs
   version    print gdnext, go, and godot versions
   project    initialise and inspect a graphics.gd project
   toolchain  manage the external programs gdnext drives
   platform   show the graphics.gd platform / host / target matrix
   android    Android device and APK helpers
   ios        iOS-specific helpers
   macos      macOS-specific helpers (lipo, codesign)
   web        WebAssembly serving and template helpers
   musl       static-musl Linux build helpers
   ci         CI helpers for the gdnext GitHub Actions workflow
```

### Concepts

#### Drop-in `go` replacement

Same contract as `gd`: graphics.gd-specific verbs are handled
in-process; everything else is forwarded to the underlying `go`
toolchain. Plus a short-flag rewrite shim so `gd`-shaped argv
(`-goos linux`) keeps working under urfave's `--goos linux`
convention.

#### Target selection

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

#### Link mode

Every catalog row supports one or more link modes:

- `gdextension` — produces a shared library Godot loads at runtime
  via `library.gdextension`. The default for every target today.
- `libgodot` — produces a static executable that embeds the engine
  by linking against `libgodot.<goos>.<goarch>.<ext>`. Currently
  only `linux/amd64` ships a published artefact.

See [catalog.md → Link modes](catalog.md#link-modes) for the
catalog side.

#### `$GDPATH`

`$GDPATH` (default `~/gd`) is where gdnext caches downloaded
toolchains and library artefacts. Override via `--gdpath` or the env
var. Layout:

```
$GDPATH/
  bin/        gd-managed executables (godot, zig, llvm, adb, ...)
  lib/        gd-managed libraries (libgodot.*.a, android.jar)
  checksums/  per-tool sha256 sidecars
```

See [toolchains.md → GDPath layout](toolchains.md#gdpath-layout).

#### Short-flag rewrite

The `go` toolchain accepts single-dash multi-char flags
(`-goos linux`); urfave only recognises `--goos linux`. gdnext
rewrites the former into the latter before urfave sees the argv,
so muscle memory from `gd`/`go` keeps working:

```
gdnext build -goos linux -goarch arm64       # ok
gdnext build --goos linux --goarch arm64     # ok
GOOS=linux gdnext build                      # ok
```

The rewrite covers every global flag gdnext registers. The CI verb
`gdnext ci check-short-flag-rewrite` pins the behaviour.

#### Go passthrough

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

### Top-level verbs

#### `build` / `export`

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

Output lands under `releases/<goos>/<goarch>/`, matching `gd build`'s
layout. The `gdnext ci build-target` verb asserts artefacts at
exactly this location.

#### `run`

Build the project as a shared library and launch it. On desktop
that's `godot -e` against the staged `graphics/` directory; on
android it's `adb install` + launch; on web it's a local HTTP
server with COEP/COOP. Mirrors `gd run`.

```
gdnext run
gdnext run --goos android   # builds + adb installs + launches
gdnext run --goos js        # builds + serves on localhost
```

#### `test`

Cross-compile and run go tests inside the Godot runtime. Same
behaviour as `gd test`: known `-flag` arguments get rewritten to
`-test.flag` form, and tests get the full engine API.

```
gdnext test
gdnext test --goos linux --link libgodot
gdnext test -- -run TestSpecific -v
```

#### `doc`

`go doc` with extra resolution for graphics.gd's `//gd:`-tagged
symbols. Mirrors `gd doc`.

```
gdnext doc Node
gdnext doc graphics.gd/Animation Player
```

#### `fix`

Rewrites code to migrate from deprecated graphics.gd APIs. Wraps
the rewrite rules under `cmd/gdnext/internal/refactor`. Not a
mirror of any `gd` verb — `gdnext`'s addition.

```
gdnext fix
```

#### `version`

Prints gdnext, go, and godot versions. Wider than `gd version`,
which only covers itself + `go`.

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

#### `project`

Initialise and inspect a graphics.gd project. The first verb in
`gdnext` that doesn't have a `gd` counterpart — `gd` does the
init implicitly when run with no args, `gdnext` exposes it
explicitly.

```
gdnext project init       # create graphics/, project.godot, presets
gdnext project info       # print resolved metadata
gdnext project version    # print config/version from project.godot
```

#### `platform`

Show the graphics.gd platform / host / target matrix. The full
verb tree (filters, single-row detail, structured output) lives in
[catalog.md → Verbs](catalog.md#verbs):

```
gdnext platform                       # full matrix
gdnext platform linux                 # single-row detail
gdnext platform --hosts                # only hosts
gdnext platform --targets              # only targets
gdnext platform -f markdown            # paste-ready table
```

#### `toolchain`

Manage the external programs gdnext drives. Full reference in
[toolchains.md](toolchains.md):

```
gdnext toolchain list             # catalog view
gdnext toolchain doctor           # host probe
gdnext toolchain install          # fetch missing into $GDPATH
gdnext toolchain uninstall --all  # drop gd-managed installs
gdnext toolchain path <slug>      # resolve a binary
```

`gd` does toolchain management implicitly (downloads Godot + zig +
... as needed). `gdnext` exposes the same machinery as a first-class
verb with verification and audit support.

#### `android`

Android device + APK helpers. Mostly thin wrappers around the
bundled `adb` from the toolchain catalog.

```
gdnext android adb devices
gdnext android install app.apk    # adb install
gdnext android logcat             # stream logcat
gdnext android apk inspect app.apk
gdnext android keystore           # TODO: manage the debug keystore
```

If a system `adb` is on `$PATH`, gdnext defers to it.

#### `ios`

iOS-specific helpers. Requires a macOS host for codesign + lipo.

```
gdnext ios xcode-gen   # generate releases/ios/<arch>/<project>.xcodeproj
```

`xcode-gen` runs `gdnext build --goos ios` under the hood and then
materialises an Xcode project around the produced artefact.

#### `macos`

macOS export helpers (lipo, codesign).

```
gdnext macos lipo       # merge per-arch dylibs into a universal one
gdnext macos codesign   # TODO: extract codesign --deep from builder.MacOS
```

#### `web`

WebAssembly serving and template helpers.

```
gdnext web serve   # serve releases/js/wasm/ with COEP/COOP headers
```

The COEP/COOP headers are what `SharedArrayBuffer` requires in the
browser; serving any other way will silently break threads.

#### `ci`

CI helpers for the GitHub Actions workflow. Full reference in
[workflow.md → CI verbs](workflow.md#ci-verbs).

```
gdnext ci build-matrix            # emit the build matrix as JSON
gdnext ci build-target            # build one (goos, goarch) cell
gdnext ci play-cell               # drive a built example via the play-bot
gdnext ci workflow-summary        # render the rolling step summary
gdnext ci check-toolchain-checksums  # harvest SHAs for KnownChecksums
...
```

Not meant for day-to-day use — these verbs read `$GITHUB_*` env
vars, expect a CI scratch dir, and emit JSON for `fromJSON()`.
Reproducing a failing CI cell locally is fine via the runbook in
[workflow.md → Reproduce a failing matrix cell locally](workflow.md#reproduce-a-failing-matrix-cell-locally).

## Runbook

### Pick a CLI for the task

- **Shipping a project today** — use `gd`. It's the in-production
  CLI; every published example and downstream project is built on it.
- **Reaching subsystems `gd` doesn't expose** (toolchain audit,
  catalog query, CI orchestration, project scaffolding) — use
  `gdnext`. They aren't in `gd`.
- **Working on the workflow / docs / catalog / `gdnext` itself** —
  use `gdnext`. That's where the work lands.

Don't mix the two inside a single command pipeline. They install to
the same `~/gd` cache directory and target the same
`releases/<goos>/<goarch>/` output paths, but each owns its own
resolution logic; alternating between them within a single workflow
is a way to hit subtle inconsistencies. Pick one per session.

### Build for the current host

```
gd build                # or
gdnext build
```

Output: `releases/<goos>/<goarch>/<project>` — the path is keyed on
the target tuple. When no flags are passed, the target defaults to
the current host.

### Cross-build for another target

```
gd build -goos windows -goarch amd64           # or
gdnext build --goos windows --goarch amd64

gdnext build --goos android                    # arm64 default
gdnext build --goos js                         # wasm default
gdnext build --goos linux --link libgodot      # static, embedded engine
```

Targets the host can build are listed in
[catalog.md → Hosts vs targets](catalog.md#hosts-vs-targets);
`gdnext platform --targets` shows the live set. If the target needs
a toolchain you don't have yet, `gdnext toolchain install <slug>`
or the bulk `gdnext toolchain install` fills the gap.

### Run the project under Godot

```
gd run                         # or
gdnext run

gdnext run --goos android      # adb install + launch
gdnext run --goos js           # local HTTP server with COEP/COOP
```

### Run headless tests

```
gd test                        # or
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

`gd` performs the same setup implicitly when invoked with no args —
`gdnext project init` is the explicit form.

## Development

### Where each CLI lives

- [cmd/gd/main.go](../cmd/gd/main.go) — `gd`'s entire dispatch loop:
  argv parsing, `builderFor(goos)` switch, go passthrough, project
  setup.
- [cmd/gd/internal/builder/](../cmd/gd/internal/builder/) —
  `gd`'s per-platform builders (`Linux`, `Windows`, `MacOS`, `IOS`,
  `Android`, `MetaQuest`, `Musl`, `Browser`).
- [cmd/gd/internal/tooling/](../cmd/gd/internal/tooling/) —
  `gd`'s in-CLI toolchain catalog and downloader.
- [cmd/gdnext/main.go](../cmd/gdnext/main.go) — root command
  assembly, DI registration, short-flag rewrite, go passthrough.
- [cmd/gdnext/internal/cli/](../cmd/gdnext/internal/cli/) — one Go
  file per top-level verb (`build.go`, `run.go`, `test.go`,
  `project.go`, `toolchain.go`, ...). Each file owns its DI structs,
  command wiring, and action functions.
- [cmd/gdnext/internal/builder/](../cmd/gdnext/internal/builder/) —
  `gdnext`'s per-platform builders. Independent copy of `gd`'s set,
  refactored to read from the standalone `product/` catalog rather
  than hard-coded constants.
- [cmd/gdnext/internal/tooling/](../cmd/gdnext/internal/tooling/) —
  runtime side of `gdnext`'s toolchain catalog (download, verify,
  exec). Catalog data lives in [product/](../product/). See
  [toolchains.md](toolchains.md).
- [cmd/gdnext/internal/ci/](../cmd/gdnext/internal/ci/) — CI verbs.
  See [workflow.md](workflow.md).

Neither `cmd/gd/internal/` nor `cmd/gdnext/internal/` imports the
other. The split is intentional — see
[Why two CLIs](#why-two-clis).

### Adding a verb to `gdnext`

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
  `gd`'s `builderFor(goos)` switch is what `gdnext` exists to replace
  — every fact about a target should round-trip through the catalog.
- **Banner output matches CI.** `gdnext ci <verb>` prints
  `==> name args` banners; user-facing verbs follow the same shape so
  logs read consistently whether run locally or in CI.
- **Wrap errors with `runtime.link/api/xray`.** Every build-pipeline
  error path uses `xray.New(err)` so failures carry context rather
  than a single line.
- **No interactive prompts.** Both CLIs must run unattended in CI.
  Anything that needs a value comes through a flag or env var.
