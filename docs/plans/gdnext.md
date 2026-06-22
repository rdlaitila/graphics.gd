# gdnext

A `urfave/cli/v3`-based successor to `cmd/gd`, together with the
platform matrix, CI driver, canary example, and agent / contributor
docs that grow up around it.

## Why

graphics.gd is maturing. The original `gd` command proved the model —
a drop-in, Go-friendly UX around a Godot project, cross-compile
everywhere, hide the toolchain mess — and it carried the library
through years of platform additions, build refactors, and contributor
onboarding. That run earned a stable user base and a clear picture of
what the project actually needs as it scales further.

`gdnext` is the next chapter, built on what came before:

- **More platforms, more toolchains, more contributors.** Each new
  target (musl, metaquest, web, ARM hosts) added another row to the
  dispatch surface and another env-var convention. A first-class verb
  tree backed by a central platform matrix gives the project room to
  keep adding rows without the surface fraying.
- **Self-documenting beats tribal knowledge.** Subsystems with
  standalone value (toolchain management, debug keystore, APK
  sign/verify, web serve, refactor engine) become discoverable verbs
  with their own `--help`. New contributors and AI agents alike can
  learn the tool from the binary itself instead of from a maintainer.
- **Data, not strings.** Platform aliases, toolchain requirements,
  host-vs-target capability — metadata that used to be inline lifts
  into typed packages other layers (CLI, CI, docs, downstream tools)
  can import without reimplementing the rules.
- **Load-bearing CI.** The supported-target matrix is large enough
  that a per-platform regression is easy to miss without a canary
  exercising every subsystem on every runner.
- **One durable structure to clip onto.** `gdnext` pulls the platform
  matrix, toolchain catalog, CI driver, example projects, style guide,
  and agent instructions into a shape that future expansion can extend
  without rearchitecting around each new feature.

`gd` keeps working throughout. Promoting `gdnext` to the default is a
separate decision for after parity is proven in the wild.

## Pieces

- **New CLI — `cmd/gdnext/`.** Self-documenting subcommand tree.
  Every operation that used to be reachable only through a `gd build`
  side effect (toolchain, keystore, apk, lipo, web serve, refactor)
  becomes a verb with its own `--help`. Forwards unknown verbs to `go`
  so `gdnext` is a drop-in for `gd` and `go` alike inside a
  graphics.gd project.
- **Platform matrix — `product/`.** Pure data, no `graphics.gd`
  imports. Single source of truth for every `goos/goarch` row with
  role (host / target), support status, aliases, and renderers.
  `gdnext platforms` is the introspection surface; consumed by the
  CLI, CI, and any future docs generator.
- **Toolchain catalog — `cmd/gdnext/internal/tooling/`.** Each entry
  declares what it's required for (target side) and where it can run
  (host side). `gdnext toolchain doctor` classifies every entry per
  (host, target) so a missing prerequisite surfaces with a clear
  message instead of a deeper builder failure.
- **CI driver — `cmd/gdnext/internal/ci/`.** A `gdnext ci` subcommand
  tree, one verb per workflow phase. The workflow YAML at
  `.github/workflows/gdnext.yml` is a thin wrapper; the help text on
  each verb is the canonical documentation for what it asserts.
- **Canary example — `examples/canarybird/`.** Smallest project that
  touches every subsystem (UI, 2D, 3D, audio, input, Go ↔ Godot
  bindings), so a per-platform regression surfaces as a failed example
  build. Adding canaries is additive: any `examples/<name>/` with a
  `go.mod` requiring `graphics.gd` and a `graphics/project.godot`
  joins the matrix.
- **Agent + contributor docs.** `AGENTS.md` (symlinked to
  `docs/agents/instructions.md`) is the canonical entry point for
  every coding agent and human reading this repo, pointing at
  `docs/style.md` (Go conventions) and `docs/structure.md` (top-level
  layout). Other agent entry points (`CLAUDE.md`,
  `.github/copilot-instructions.md`, …) symlink to the same file so
  there is exactly one source of truth.
- **LinkMode — third platform axis.** Every target row carries a
  `LinkMode` bitfield (`GDExtension | LibGodot`) declaring which
  linking recipes the platform supports. Replaces the legacy
  `musl/*` rows: linux/* gains `GDExtension | LibGodot`; the user
  picks via a new `--link` flag. Unblocks per-target library
  artefacts (`libgodot.linux.amd64.a`, future libgodot variants)
  resolving correctly in `toolchain install` / `doctor` without
  needing to know about musl specifically. See *LinkMode* below.

## Out of scope

- **No edits to `cmd/gd`.** The legacy binary stays intact during the
  POC.
- **No new external dependencies** beyond `urfave/cli/v3` and
  `yaml.v3` (for `gdnext platforms --format yaml`).
- **No iOS / signed-macOS end-to-end CI.** Both need real Apple certs
  that can't live in a public repo.
- **No `gdnext run` in CI.** Needs a windowed GPU session;
  `gdnext test` exercises the same setup pipeline headlessly.

## LinkMode

Today the build pipeline supports two fundamentally different ways of
producing a Godot-driven binary, and the platform matrix doesn't
distinguish them. That mismatch is why `toolchain install` asks for
`libgodot.linux.amd64.a` (404) instead of `libgodot.musl.amd64.a` (the
artefact that actually exists) when libgodot is in scope. Adding
`LinkMode` as a first-class platform axis fixes the modelling.

### The two linking models

**GDExtension — the legacy path.** Build output is a shared library
(`.so` / `.dll` / `.dylib`, or `.a` on iOS) loaded by Godot at runtime
via `library.gdextension`. Godot is a separate process installed on
the user's machine; the build needs no `libgodot.*` artefact at all.
Covers linux, windows, darwin, android, ios, web, metaquest as they
ship today.

**LibGodot — the single-static-binary path.** Build output is one
statically-linked executable that embeds Godot itself. Compiles the
Go code with `-buildmode=c-archive` then links it against a per-target
`libgodot.$(GOOS).$(GOARCH).$(EXT)` archive. Used today only by the
musl-based static-linux flow, but the recipe is general — any (GOOS,
GOARCH) for which the project publishes a `libgodot.*` artefact can
support it.

### Schema changes

- `product.LinkMode` bitfield with `GDExtension` and `LibGodot`
  values; `String`, `MarshalText`, `Has` parallel to `Kind`/`Status`.
- `Platform.LinkModes LinkMode` declares which modes each target
  supports (bit-OR of one or both).
- `GOOSLinkModeDefaults map[string]LinkMode` picks the default mode
  per GOOS when the user doesn't pass `--link`. Today every entry is
  `GDExtension`; the table is the single change point when the
  default ever flips.
- `BuildEnv.Target.LinkMode` carries the resolved value through every
  downstream consumer; `BuildEnv.Validate` checks it.

### Matrix consequences

- `PlatformLinuxMuslAmd64` / `PlatformLinuxMuslArm64` collapse into
  `PlatformLinuxAmd64` / `PlatformLinuxArm64` with `LinkModes =
  GDExtension | LibGodot`. The `musl` GOOS becomes an alias that
  resolves to `linux` + `LinkMode=LibGodot` so existing `GOOS=musl`
  invocations keep working.
- `MuslToolchains` (currently `LLVM, libgodot, libgodot-editor, ldd`)
  becomes `LibGodotToolchains`. Only platforms whose `LinkModes`
  includes `LibGodot` declare it in `BuildTools`.
- The CI matrix tuple-keying becomes `(target.GOOS, target.GOARCH,
  LinkMode)` to keep `linux/amd64+GDExtension` and
  `linux/amd64+LibGodot` distinct cells.

### CLI surface

- New global `--linkmode`, bridged via
  `GOLINK`. Values `gdextension` / `libgodot` populated from the
  catalog so the help text stays in sync with `product.LinkMode`.
- `gdnext platform` adds a `LINK` column showing each row's supported
  modes.
- `gdnext toolchain doctor` adds a `LINK` column and walks per-
  `(target, tool)` pair. Tools whose `IsLibrary` is true resolve via
  `LookupPlatform(target.GOOS, target.GOARCH, ...)` so per-target
  archives (`libgodot`, `libgodot-editor`, `android.jar`) fetch the
  right artefact instead of defaulting to the host tuple.

### Builder dispatch

- `platform.For(env)` routes on `(env.Target.GOOS, env.Target.LinkMode)`.
  `(*, GDExtension)` reaches the existing per-OS builders;
  `(*, LibGodot)` reaches a single `libgodot` builder (current
  `musl.go` re-mounted under the new dispatch key). Future per-host
  libgodot recipes (glibc-static linux, static-windows, …) slot in
  beside it without touching the GDExtension path.
- Until additional libgodot recipes exist, the libgodot builder
  emits the same `libgodot.musl.$(GOARCH).a` request the catalog
  publishes today. A `DownloadOS` override on `ToolchainLibGodot`
  keeps the URL substitution producing `libgodot.musl.<arch>.a` when
  target GOOS is `linux`. The override drops the day a per-libc
  variant is published.

### Backwards compatibility

`GOOS=musl gdnext build` keeps working: `FindBuildEnv("musl", "")`
remaps to `linux` + `LinkMode=LibGodot` and the spawned `go build`
sees `GOOS=linux` (which Go understands; it never understood `musl`).
Scripts that pass `--goos musl` continue to dispatch to the libgodot
builder. The CI matrix gains a `link` axis; rows that previously
emitted `target=musl/amd64` now emit `target=linux/amd64
link=libgodot`.

### Phased rollout

1. **Schema + resolver** — add `LinkMode`, `Platform.LinkModes`,
   `GOOSLinkModeDefaults`, `BuildEnv.Target.LinkMode`,
   `FindBuildEnv` extension. No consumer changes yet.
2. **CLI + matrix collapse** — `--link` flag + bridge, drop the
   musl-specific Platform rows, add the alias remap, surface the new
   column in `platform` and `doctor`.
3. **Install / doctor loop rewrite** — walk per-`(target, tool)` pair,
   honour `IsLibrary` for per-target downloads.
4. **Builder dispatch + catalog tidy-up** — re-mount musl.go under
   `(*, LibGodot)`, drop libgodot from `MuslToolchains` (now gone),
   add the `DownloadOS` override.
5. **Verify** — `gdnext toolchain install` resolves
   `libgodot.musl.amd64.a` end-to-end; `gdnext build --goos=musl` and
   `gdnext build --goos=linux --link=libgodot` both still produce a
   working musl binary.
