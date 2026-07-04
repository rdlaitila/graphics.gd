# Catalog

> Command invocations in this doc (`gdnext platform ...`, `gdnext toolchain ...`)
> refer to the next-generation CLI at [`cmd/gdnext/`](../cmd/gdnext/), which is
> being rebuilt alongside the original `gd` CLI. See [cli.md](cli.md) for
> the CLI overview.

The `product/` package is graphics.gd's catalog: the declarative
source of truth for the platforms, hosts, link modes, and toolchains
the project supports. Every other layer — the CLI, the CI driver,
builders, downstream tooling, this repo's docs — reads from
`product.PlatformMatrix`, `HostMatrix`, `ToolchainMatrix`, etc.
rather than hard-coding its own list.

`gdnext platform` is the catalog's CLI surface: list, filter, render,
and inspect rows in `table | json | yaml | xml | markdown`. The
toolchain side of the catalog has its own verb tree
(`gdnext toolchain ...`); see [toolchains.md](toolchains.md) for that.

- [Catalog](#catalog)
  - [Why this exists](#why-this-exists)
    - [Hygiene](#hygiene)
  - [Concepts](#concepts)
    - [Platforms](#platforms)
    - [Hosts vs targets](#hosts-vs-targets)
    - [Link modes](#link-modes)
    - [Status bitmask](#status-bitmask)
    - [Aliases](#aliases)
  - [Verbs](#verbs)
    - [`platform` — list / filter / inspect](#platform--list--filter--inspect)
    - [`platform <name>` — single-row detail](#platform-name--single-row-detail)
    - [`platform --hosts` / `--targets`](#platform---hosts----targets)
    - [`--format=...` — structured output](#--format--structured-output)
  - [Runbook](#runbook)
    - [Which platforms can I build for from this machine?](#which-platforms-can-i-build-for-from-this-machine)
    - [Is `<platform>` supported?](#is-platform-supported)
    - [Generate a markdown support matrix for a README](#generate-a-markdown-support-matrix-for-a-readme)
    - [Wire the catalog into a downstream tool](#wire-the-catalog-into-a-downstream-tool)
  - [Development](#development)
    - [Where the catalog lives](#where-the-catalog-lives)
    - [Adding a new platform](#adding-a-new-platform)
    - [Adding a new alias](#adding-a-new-alias)
    - [Adding a new link mode](#adding-a-new-link-mode)
    - [Changing status](#changing-status)
    - [Adding a new GOOS or GOARCH](#adding-a-new-goos-or-goarch)

## Why this exists

graphics.gd has to coordinate a lot of moving parts across a lot of
target platforms — every build target, host, link mode, and external
tool. Without a single declarative catalog, that information ends up
duplicated across the CLI, the builders, CI, the docs, and downstream
projects, and the copies drift. The catalog is the one place that
information is encoded, and everything else reads from it:

- **One source of truth.** `gdnext platform`, `gdnext toolchain`,
  the build matrix in CI, the auto-generated docs, and the runtime
  link-mode picker all consult `product.*Matrix`. Change a row once,
  every consumer picks it up.
- **No import cycles.** `product` imports nothing else in the repo. Every higher layer can depend on
  it without circular-import risk.
- **Pure data.** Rows are plain struct literals with no init-time
  logic. Tests assert invariants (alias uniqueness, host coverage,
  etc.) but they don't compute the matrix.

### Hygiene

A few rules keep the catalog honest:

- **Aliases are unique across the matrix.** `TestAliasesUnique`
  fails if two platforms claim the same alias.
- **Every Status mode that isn't `Broken` ships with CI coverage.**
  Marking a row `Supported` is a commitment.
- **`Notes` is for caveats the cell can't express on its own** —
  upstream bugs, renderer fallbacks, host-only constraints. Don't
  use it for free-form prose.
- **No version pinning here.** That belongs in
  `product.Toolchain.KnownChecksums` / `Toolchain.Version`, not in
  `Platform`. The platform row says *what* is supported, the
  toolchain row says *with what version*.

## Concepts

### Platforms

A `Platform` is one row in the support matrix: a canonical
`(GOOS, GOARCH)` pair plus the metadata graphics.gd ships for it
(title, aliases, status, link modes, available build hosts, renderers,
notes). `PlatformMatrix` is the ordered list of every row.

```go
type Platform struct {
    Title      string
    GOOS       string
    GOARCH     string
    Aliases    []string
    Kind       Kind        // Host | Target bitmask
    Status     Status      // Supported | Stable | Quirky | ...
    LinkModes  LinkMode    // GDExtension | LibGodot bitmask
    BuildHosts []BuildHost
    PlayHosts  []BuildHost
    BuildTools []Toolchain
    Renderers  []string
    Notes      string
}
```

`Tuple()` returns the canonical `"goos/goarch"` identifier used
across CLIs and APIs (matching `go env`, Docker, buildx).

### Hosts vs targets

The `Kind` bitmask classifies what graphics.gd does *to* a platform:

- `Host` — the CLI itself runs on this platform. Reflected in
  `HostMatrix` (the list of platforms with a CI runner).
- `Target` — graphics.gd can build for this platform.

Most rows are `Host | Target` (linux/amd64, darwin/arm64, ...).
Pure-target rows include iOS, Android, Meta Quest, and Web.
`gdnext platform --hosts` and `--targets` filter on this bitmask.

### Link modes

`LinkMode` is a bitmask of linking recipes:

- `GDExtension` — produces a shared library Godot loads via
  `library.gdextension` at runtime. The default.
- `LibGodot` — produces a static executable that embeds the engine
  by linking against `libgodot.<goos>.<goarch>.<ext>`.

A platform may support both (`LinkModes: GDExtension | LibGodot`);
the user picks via the `--link` CLI flag (`gdextension` is the
default per `GOOSLinkModeDefaults`).

### Status bitmask

`Status` is a bitmask describing *how* support is claimed:

| Bit            | Meaning                                                                |
| -------------- | ---------------------------------------------------------------------- |
| `Supported`    | Headline bit. CI covers this row; regressions are bugs.                |
| `Stable`       | Rock-solid, exercised by every release.                                |
| `Quirky`       | Builds and runs but has documented caveats — see `Notes`.              |
| `Experimental` | Builds, lacks CI / has known gaps. Usable, no stability promise.       |
| `Deprecated`   | Still maintained while phasing out.                                    |
| `Broken`       | Known not to build or run. Usually set alone, without `Supported`.     |

Rendered as `+`-joined lowercase tokens: `supported+stable`,
`supported+quirky`, `experimental`, etc. The zero value renders as
`?` — a fill-me-in signal.

Bitmask membership tested with `s.Has(Supported)` etc.

### Aliases

Every Platform carries a list of accepted aliases — alternate names
the CLI / downstream tools should resolve to the canonical row.
Examples:

- `linux/amd64` → `ubuntu`, `arch`, `debian`, `nix`, `ublue/bazzite`
- `darwin/amd64` → `macos`
- `js/wasm` → `web`, `browser`, `wasm`
- `metaquest/arm64` → `quest`, `meta`

`product.FindPlatformByName(alias)` does case-insensitive resolution
against the full alias space. Aliases are required to be unique
across the matrix (enforced by `TestAliasesUnique`).

## Verbs

### `platform` — list / filter / inspect

`gdnext platform` is the single CLI surface for the catalog. With no
args it dumps every row; with a name it shows a single-row detail
view; with `--hosts` or `--targets` it filters; with `--format` it
renders structured output.

```
gdnext platform                  # full matrix, table
gdnext platform <name>           # single row detail
gdnext platform --hosts          # only Host rows
gdnext platform --targets        # only Target rows
gdnext platform -f markdown      # GFM table (also: json, yaml, xml)
gdnext platform -l               # --vertical, one field per line
```

Example:

```
graphics.gd all platforms (host: linux/amd64)

PLATFORM         KIND         STATUS            LINK                  ALIASES                                   NOTES
linux/amd64      host+target  supported+stable  gdextension+libgodot  ubuntu, arch, debian, nix, ublue/bazzite  libgodot mode (--link=libgodot or GOOS=musl alias) currently fetches the .musl. artefact
linux/arm64      target       supported         gdextension+libgodot  -                                         cross-compiled from any host via zig
windows/amd64    host+target  supported+stable  gdextension           win
windows/arm64    target       supported         gdextension           -
darwin/amd64     host+target  supported+stable  gdextension           macos                                     exports as a universal .app alongside arm64
darwin/arm64     host+target  supported+stable  gdextension           -                                         produces a universal .app; lipo + codesign need a darwin host
ios/arm64        target       supported         gdextension           iphone                                    requires llvm; signing needs a macOS host + Apple cert
android/arm64    target       supported+stable  gdextension           -
android/amd64    target       supported+quirky  gdextension           -                                         primarily emulator or desktop android targeted (ex: waydroid)
metaquest/arm64  target       supported         gdextension           quest, meta                               Android profile with GodotVR + OpenXR injected into the apk
js/wasm          target       supported         gdextension           web, browser, wasm                        COEP/COOP headers required when serving
```

### `platform <name>` — single-row detail

The argument is an alias (case-insensitive). Resolves via
`product.FindPlatformByName` against the union of every row's name
list (canonical tuple, title, GOOS, GOARCH, aliases).

```
gdnext platform linux
gdnext platform ubuntu          # resolves to linux/amd64
gdnext platform quest           # resolves to metaquest/arm64
```

Example:

```
$ gdnext platform linux
title:      Linux x86_64
platform:   linux/amd64
aliases:    ubuntu, arch, debian, nix, ublue/bazzite
kind:       host+target
status:     supported+stable
renderers:  vulkan, opengl3, gl_compatibility
notes:      libgodot mode (--link=libgodot or GOOS=musl alias) currently fetches the .musl. artefact
```

The detail view is verbose by design — it's what to paste into a bug
report or PR description when discussing a specific row.

### `platform --hosts` / `--targets`

Filters by `Kind`. Mutually exclusive.

- `--hosts` — rows that can run the CLI (i.e. rows in `HostMatrix`).
- `--targets` — rows graphics.gd can build for.

Example:

```
$ gdnext platform --hosts
graphics.gd hosts (host: linux/amd64)

PLATFORM       KIND         STATUS            LINK                  ALIASES                                   NOTES
linux/amd64    host+target  supported+stable  gdextension+libgodot  ubuntu, arch, debian, nix, ublue/bazzite  libgodot mode (--link=libgodot or GOOS=musl alias) currently fetches the .musl. artefact
windows/amd64  host+target  supported+stable  gdextension           win
darwin/amd64   host+target  supported+stable  gdextension           macos                                     exports as a universal .app alongside arm64
darwin/arm64   host+target  supported+stable  gdextension           -                                         produces a universal .app; lipo + codesign need a darwin host
```

### `--format=...` — structured output

`-f json | yaml | xml | markdown` renders the same rows in machine
or doc-friendly formats. `markdown` produces a GitHub-flavoured
table suitable for pasting into a README; `--vertical` (`-l`) flips
markdown to one-field-per-line, which is what to use for a long
detail view.

```
gdnext platform -f markdown                 # full matrix as a GFM table
gdnext platform -f json                     # one big JSON array
gdnext platform <name> -f yaml              # single-row YAML
gdnext platform --hosts -f markdown -l      # vertical markdown for hosts
```

Example (markdown):

```
# graphics.gd platform linux/amd64

| Platform | Kind | Status | Aliases | Renderers | Notes |
|----------|------|--------|---------|-----------|-------|
| linux/amd64 | host+target | supported+stable | ubuntu, arch, debian, nix, ublue/bazzite | vulkan, opengl3, gl_compatibility | libgodot mode (--link=libgodot or GOOS=musl alias) currently fetches the .musl. artefact |
```

## Runbook

### Which platforms can I build for from this machine?

```
gdnext platform --targets
```

Rows whose `BuildHosts` includes the current host can be built from
here. Cross-compilation goes through `zig` for the C/C++ side —
see [toolchains.md](toolchains.md). iOS additionally requires a
macOS host for the final lipo + codesign step.

### Is `<platform>` supported?

```
gdnext platform <name>
```

Read the `status:` line:

- `supported+stable` — green-lit.
- `supported+quirky` — usable, check `notes:`.
- `supported` — covered, no specific stability promise.
- `experimental` (no `supported`) — usable, no CI, no guarantee.
- `broken` (no `supported`) — known broken, builds may fail.

Unknown alias? `gdnext platform` for the full set, or grep
`product/matrix.go` directly.

### Generate a markdown support matrix for a README

```
gdnext platform -f markdown > support.md
```

Single row:

```
gdnext platform <name> -f markdown
```

The markdown form is GitHub-flavoured — pipes inside cells are
escaped. Drop the output directly into a `.md` file or a PR
description.

### Wire the catalog into a downstream tool

Import `graphics.gd/product`. `product` has no internal repo deps, so it's safe to consume from any tool:

```go
import "graphics.gd/product"

for _, p := range product.PlatformMatrix {
    if p.Status.Has(product.Supported) && p.Kind.Has(product.Target) {
        // …
    }
}
```

Key entry points: `PlatformMatrix`, `Hosts()`, `Targets()`,
`FindPlatformByName(alias)`, `Tuple(goos, goarch)`,
`ToolchainMatrix`, `FindToolchainBySlug(slug)`.

## Development

For maintainers updating the catalog itself.

### Where the catalog lives

- `product/matrix.go` — every `*Matrix` var plus the per-row literals
  (`PlatformLinuxAmd64`, `ToolchainGodot`, `HostLinuxAmd64`, ...).
  This is the file you edit when adding or changing rows.
- `product/platform.go` — `Platform`, `BuildHost`, `TargetHost`,
  `Kind`, lookup helpers (`FindPlatformByName`, `Hosts`, `Targets`,
  `Tuple`).
- `product/linkmode.go` — `LinkMode` bitmask + the `LinkModeMatrix`
  string list + per-GOOS defaults.
- `product/product.go` — `Status` bitmask and shared formatting.
- `product/toolchain.go` — the `Toolchain` struct; see
  [toolchains.md](toolchains.md) for the catalog/runtime split.
- `product/product_test.go`, `product/toolchain_test.go` — invariants
  (alias uniqueness, host coverage, link-mode defaults, ...).

### Adding a new platform

1. Declare the platform literal in `product/matrix.go` next to the
   sibling rows (Linux block, Windows block, ...). Required fields:
   `Title`, `GOOS`, `GOARCH`, `Kind`, `Status`, `LinkModes`,
   `BuildHosts`. Optional: `Aliases`, `PlayHosts`, `BuildTools`,
   `Renderers`, `Notes`.
2. Append the literal to `PlatformMatrix` (catalog ordering matches
   the matrix grouping — Linux, Windows, macOS, iOS, Android,
   MetaQuest, Web).
3. If the GOOS isn't already in `GOOSMatrix` / `GOOSAliasLinkMode` /
   `GOOSLinkModeDefaults` / `GOOSArchDefaults`, add it there too.
4. If this introduces a new toolchain dependency, add the
   `Toolchain<Name>` entry per [toolchains.md → Adding a new tool](toolchains.md#adding-a-new-tool)
   and reference it in `BuildTools`.
5. Run `go -C cmd/gdnext test ./...` — the catalog invariants
   (`TestAliasesUnique`, host coverage, ...) gate the merge.

### Adding a new alias

Append it to the target platform's `Aliases` slice. `TestAliasesUnique`
will fail if the alias collides with another row's name space, so
you'll know immediately if it's taken. Case is normalised at lookup
time — store the canonical lowercase form.

If the alias is also a GOOS the user might pass to `gdnext --goos=...`
(e.g. `ubuntu`, `quest`), wire it into `GOOSRemaps` so the remap
happens at the GOOS layer too.

### Adding a new link mode

1. Declare the new bit in `product/linkmode.go` alongside
   `GDExtension` and `LibGodot`.
2. Add the lowercase name to `LinkModeMatrix` and the parser switch
   in `UnmarshalText`.
3. Add it to `linkModeOrder` so `String()` renders it in the
   canonical position.
4. For every platform that supports it, OR the bit into its
   `LinkModes` field in `product/matrix.go`.
5. If it should be the default for any GOOS, update
   `GOOSLinkModeDefaults`.
6. Run the test suite — the link-mode tests cover the bitmask round-trip.

### Changing status

Status is a commitment. The matrix-wide intent is:

- Promote from `Experimental` → `Supported` only after CI lands.
- Promote `Supported` → `Supported | Stable` only after a few
  releases without regressions.
- Demote to `Quirky` (with a `Notes` entry) rather than dropping
  `Supported` outright when caveats appear.
- Mark `Broken` (without `Supported`) when a target stops working
  and won't be fixed before the next release. CI is allowed to fail
  on `Broken` rows.

Edits are one-line: change the `Status: ...` field. The test suite
re-renders the matrix; the CI summary picks up the change for the
next run.

### Adding a new GOOS or GOARCH

Add the canonical constant in `product/matrix.go` (`GOOSWhatever`,
`GOARCHWhatever`), append it to `GOOSMatrix` / `GOARCHMatrix`, and
populate the relevant maps (`GOOSArchDefaults`, `GOOSRemaps`,
`GOOSLinkModeDefaults`, `GOOSAliasLinkMode`). Then add the
`Platform` row per [Adding a new platform](#adding-a-new-platform).

Remap aliases (`ubuntu`, `macos`, `quest`, ...) live in `GOOSRemaps`
and are applied before any matrix lookup, so a user passing
`--goos=ubuntu` sees the linux row without `FindPlatformByName`
needing to know about it.
