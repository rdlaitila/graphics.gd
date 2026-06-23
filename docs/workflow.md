# Workflow

The graphics.gd CI lives in [.github/workflows/gdnext.yml](../.github/workflows/gdnext.yml).
Almost every step is a thin wrapper around a `gdnext ci <verb>` call;
the actual work — building, testing, driving the engine, rendering
summaries — is plain Go code under
[cmd/gdnext/internal/ci/](../cmd/gdnext/internal/ci/). This doc covers
how the pieces fit together, what each verb does, and the policies the
matrix is enforcing.

- [Workflow](#workflow)
  - [Why this exists](#why-this-exists)
    - [CI steps are Go code](#ci-steps-are-go-code)
    - [Full matrix coverage](#full-matrix-coverage)
  - [Topology](#topology)
    - [Jobs and their dependencies](#jobs-and-their-dependencies)
    - [Trigger and concurrency](#trigger-and-concurrency)
    - [Caches and artefacts](#caches-and-artefacts)
  - [CI verbs](#ci-verbs)
    - [`build-vet-test`](#build-vet-test)
    - [`help-text`](#help-text)
    - [`diagnostic-verbs`](#diagnostic-verbs)
    - [`go-passthrough`](#go-passthrough)
    - [`short-flag-rewrite`](#short-flag-rewrite)
    - [`toolchain-install`](#toolchain-install)
    - [`toolchain-checksums`](#toolchain-checksums)
    - [`matrix`](#matrix)
    - [`stage-example`](#stage-example)
    - [`build-target`](#build-target)
    - [`test-headless`](#test-headless)
    - [`play-matrix`](#play-matrix)
    - [`play-cell`](#play-cell)
    - [`workflow-summary`](#workflow-summary)
  - [Runbook](#runbook)
    - [Reproduce a failing matrix cell locally](#reproduce-a-failing-matrix-cell-locally)
    - [Add a new build target](#add-a-new-build-target)
    - [Add a new playable target](#add-a-new-playable-target)
    - [Mark a target experimental](#mark-a-target-experimental)
    - [Pin a new toolchain hash after a green run](#pin-a-new-toolchain-hash-after-a-green-run)
    - [A summary step shows a regression](#a-summary-step-shows-a-regression)
  - [Development](#development)
    - [Where things live](#where-things-live)
    - [Adding a CI verb](#adding-a-ci-verb)
    - [Editing the workflow YAML](#editing-the-workflow-yaml)

## Why this exists

### CI steps are Go code

The predecessor workflow ([go.yml](../.github/workflows/go.yml))
built `gd` once per runner and ran `gd test -v` — a single cell per
host. `gdnext.yml` keeps that 'CI is Go code, not shell' stance and
extends it to every step of the build pipeline: every `run:` line is
one of pure shell plumbing (`cd`, `mkdir`, `tee`), a vendored action
(`actions/checkout`, `actions/setup-go`, ...), or a single
`gdnext ci <verb>` invocation.

The verbs are typed Go programs that:

- Share the same DI container, catalog (`product.*`), and tooling
  layer (`cmd/gdnext/internal/tooling`) as the user-facing CLI.
- Get unit-tested alongside the rest of `cmd/gdnext`.
- Produce identical output locally and in CI — `go install ./cmd/gdnext
  && gdnext ci <verb>` reproduces a failing job step exactly.
- Fail fast with typed errors instead of shell exit codes.

There are no `if: runner.os == 'Windows'` branches re-encoding
host-specific logic three ways. There are no `grep -E` regexes
drifting from the producing tool's output format. If a verb changes
behaviour, the workflow change is one line; if a verb gains a flag,
only the YAML calling it needs to know.

Concretely, the shell in `gdnext.yml` is:

- `set -euo pipefail` boilerplate.
- `go install ./cmd/gdnext` + a `PATH` export so `gdnext` is callable.
- `actions/cache` / `actions/upload-artifact` / `actions/download-artifact`
  glue with explicit paths.
- One `gdnext ci <verb>` line per step.

That's the entire surface. Everything else lives in Go.

### Full matrix coverage

graphics.gd ships across a wide platform matrix and the only way to
catch regressions is to actually exercise every cell of it. The
workflow follows a few rules to keep that real, not aspirational:

- **Every supported `(host, target, link-mode)` triple has a CI
  cell.** `gdnext ci matrix` walks `product.PlatformMatrix` and
  emits one row per Cartesian product of CI runners (linux, windows,
  darwin) × supported targets × supported link modes.
- **No host-locked workflow edits.** Adding a row to
  `product.PlatformMatrix` (or flipping its `BuildHosts`) lands in
  the build matrix on the next push — no YAML changes, no manual
  list to keep in sync.
- **Experimental rows ride along.** Targets marked
  `Status: Supported | Experimental` (or just `Experimental`) are
  emitted with `continue-on-error: true` so they're exercised every
  run; failures surface as yellow cells in the matrix UI without
  blocking the green workflow status.
- **Built artefacts get played.** Cells whose target has a non-empty
  `PlayHosts` upload their staged build; the `gdnext-play` job
  downloads it and drives the produced binary headlessly via the
  play-bot. A target isn't "supported" until it has actually run.
- **Toolchain installs are checksum-verified per host.** Each host
  uploads a `toolchain-audit-<runner>` JSON artefact; the workflow
  summary diffs hashes across runs to flag any drift.
- **The summary aggregates the last 14 runs of the branch.** Single
  flakes are visible against the recent history, not buried.

When a target is left out of CI, it's only a matter of time before
it stops working. The verb-driven design is what makes "exercise
every supported cell, every push" affordable to maintain.

## Topology

### Jobs and their dependencies

```
discover-targets ──┐
                   ▼
              gdnext-checks ──► (toolchain cache, audit artefacts)
                   │
                   ▼
              gdnext-build ──► (per-cell build, play-artefact upload)
                   │
                   ▼
              discover-plays
                   │
                   ▼
                gdnext-play ──► (per-cell headless drive)
                   │
                   ▼
                 summary  (always(), aggregates everything)
```

- **`discover-targets`** runs `gdnext ci matrix --summary` and stashes
  the JSON as a step output. The build matrix consumes it via
  `fromJSON(needs.discover-targets.outputs.matrix)`. The matrix is
  a fully-expanded `include:` list — each cell is `(os, example,
  target, link, experimental, playable, artifact)`.
- **`gdnext-checks`** runs once per host OS (ubuntu, windows, macos)
  and exercises everything that doesn't depend on a specific build
  cell: build/vet/test, CLI help surface, diagnostic verbs, the
  short-flag rewrite shim, the go passthrough, and the full
  `toolchain install` walk. The freshly installed `~/gd` is cached
  under `${{ runner.os }}-gdnext-${{ github.sha }}` for the build
  job to restore.
- **`gdnext-build`** is the per-cell build matrix. Each cell
  restores the toolchain cache, runs `gdnext toolchain doctor --fix`
  to self-heal if anything was missed, stages the example into a
  scratch dir, builds the target, dumps the file tree, and (when
  `playable`) uploads the scratch dir as a play artefact.
- **`discover-plays`** runs `gdnext ci play-matrix --summary` against
  the same catalog and emits a `(play-host, build-host, target, link)`
  quad list. Skips the play job when no rows exist (e.g. every play
  target was excluded).
- **`gdnext-play`** downloads each cell's artefact, installs `xvfb`
  + (on linux + windows-target) `wine`, runs `gdnext ci play-cell`
  headlessly, and shows the `play-report.json`.
- **`summary`** runs `gdnext ci workflow-summary` with
  `always()` so it renders even when prior jobs fail. The output is
  piped into `$GITHUB_STEP_SUMMARY`.

### Trigger and concurrency

```yaml
on:
  push:
    branches: ["release", "staging", "gdnext", "gdnext-cli"]
    paths:
      - "cmd/gdnext/**"
      - "examples/**"
      - "product/**"
      - ".github/workflows/gdnext.yml"
  pull_request:
    branches: ["release"]
    paths: [...same...]
  workflow_dispatch:

concurrency:
  group: gdnext-${{ github.ref }}
  cancel-in-progress: true
```

Push-on-branches + PRs against `release` + manual dispatch. The
`paths:` filter keeps the workflow off pushes that don't touch any
of the catalog, the CLI, the examples, or the workflow itself.
`cancel-in-progress: true` kills any in-flight run for the same ref
when a new commit lands so runner minutes aren't wasted on stale work.

### Caches and artefacts

| Name                                | Producer        | Consumer        | Lifetime              |
| ----------------------------------- | --------------- | --------------- | --------------------- |
| `${{ runner.os }}-gdnext-${{ sha }}`| `gdnext-checks` | `gdnext-build`  | Per-SHA cache         |
| `toolchain-audit-<runner.os>`       | `gdnext-checks` | `summary`       | 30-day artefact       |
| `<matrix.artifact>` (play scratch)  | `gdnext-build`  | `gdnext-play`   | 1-day artefact        |

The per-SHA cache key means any source change forces a fresh
`toolchain install` on the next push — no chance of a stale
toolchain leaking across runs. Play artefacts are 1-day because
they're a few hundred MB per cell and only the very next job needs
them.

## CI verbs

Every verb is invoked as `gdnext ci <name>`. The list below mirrors
`gdnext ci --help` in source order.

### `build-vet-test`

Compiles every package in the `cmd/gdnext` tree, runs `go vet ./...`,
and runs `go test ./...`. The one-stop "is the codebase healthy"
check. Run once per host OS in `gdnext-checks`.

### `help-text`

Walks every registered gdnext verb and asserts `--help` resolves
without error. Catches subcommands whose flags drift out of sync
with their `Usage` strings, or commands that fail at help-render
time because of broken DI wiring.

### `diagnostic-verbs`

Runs the diagnostic verbs (`gdnext platform`, `gdnext toolchain
list`, `gdnext version`, ...) before any toolchain is installed.
These have to work on a bare runner — they're what a user with a
broken install reaches for to figure out what's wrong.

### `go-passthrough`

Confirms gdnext forwards unrecognised verbs to the underlying `go`
toolchain. `gdnext mod tidy`, `gdnext build`, `gdnext vet ./...` all
need to delegate correctly. Regressions here break user muscle memory.

### `short-flag-rewrite`

Verifies the single-dash-multi-char shim: `-goos linux` becomes
`--goos linux`. Several Go conventions and gdnext conventions
collide here; the test pins the behaviour so neither side wins
silently.

### `toolchain-install`

The smoke-test variant of `gdnext toolchain install` plus a
`gdnext toolchain path <slug>` round-trip for each installed
binary. Skips `go` (gdnext is already running on it), `ldd` (system
tool), library entries (no executable), and optional flaky tools
(`upx`, `vpk`) — the optional ones still run but their failure is
non-fatal. This is the step that populates `~/gd` for the build
matrix.

### `toolchain-checksums`

Maintainer-side harvester. Walks the most recent successful
workflow run, downloads every `toolchain-audit-<runner>` artefact,
aggregates the `(slug, goos, goarch) → sha256` view, and renders it
in `table | json | go` format ready to paste into
`product.Toolchain.KnownChecksums`. See
[toolchains.md → Updating known checksums](toolchains.md#updating-known-checksums).

### `matrix`

Emits the build matrix as a JSON `include:` list keyed by
`(os, target, example, link, experimental, playable, artifact)`.
Walks `product.PlatformMatrix` × the configured runner pool (linux,
windows, darwin arm64) × the requested examples. `--summary` also
prints a human-readable table to stderr. Consumed by the
`discover-targets` pre-job.

```
$ gdnext ci matrix --summary
{"include":[{"os":"ubuntu-latest","example":"canarybird","target":"linux/amd64",...},...]}
Build matrix (38 cells):
  ubuntu-latest  × canarybird × linux/amd64    × gdextension
  ubuntu-latest  × canarybird × linux/amd64    × libgodot
  ubuntu-latest  × canarybird × linux/arm64    × gdextension
  ...
```

### `stage-example`

Copies `examples/<name>/` into an empty scratch directory and rewrites
the example's `replace graphics.gd => ...` to point at the local
checkout, so the build picks up in-tree changes instead of the
remote module. The build matrix uses `$RUNNER_TEMP/staged` for the
scratch dir so node-based actions (upload-artifact) can see the path.

### `build-target`

The per-cell build driver. Takes `--goos`, `--goarch`, `--link`,
`--scratch <dir>`. Validates the target against `PlatformMatrix`
(typo'd tuple → clear "unknown platform" error, not a cryptic
deeper failure), then runs the underlying `gdnext build` against
the staged example.

### `test-headless`

Runs the staged example's headless test pipeline. The point is to
catch regressions in the runtime side of graphics.gd, not just the
build side.

### `play-matrix`

Same shape as `matrix`, but for the play job: emits one row per
`(play-host, build-host, target, link)` quad where the target's
`PlayHosts` is non-empty. Empty matrix → the `gdnext-play` job
short-circuits via its `if:` condition (GHA rejects empty matrices).

### `play-cell`

Downloads the play artefact, locates the produced binary under
`scratch/releases/<goos>/<goarch>/`, and drives it via the
play-bot (canarybird). The bot connects, sends inputs, asserts
expected scene state, and writes `play-report.json`. The workflow's
"Show play report" step dumps that file inside a `::group::` block
in the run log.

### `workflow-summary`

Aggregates the current run + the last N runs of the branch into a
markdown block piped to `$GITHUB_STEP_SUMMARY`. Sections:

- Checks status across the last N runs (pass/fail icons per cell).
- Build matrix status (same shape).
- Play matrix status.
- Toolchain audit table with sha256 + size + source URL + a
  `Changed` flag set when this run's hash differs from the prior run.
- Latest-run failure list with tailed log excerpts.

`--repo`, `--workflow`, `--branch`, `--runs`, `--log-tail` flags
keep it useful from any branch.

## Runbook

### Reproduce a failing matrix cell locally

```
go install ./cmd/gdnext
gdnext ci matrix --summary | head -30          # find the cell

mkdir -p /tmp/gdnext-repro
gdnext ci stage-example --example canarybird --scratch /tmp/gdnext-repro
gdnext ci build-target --goos <goos> --goarch <goarch> \
  --link <link> --scratch /tmp/gdnext-repro
gdnext ci test-headless --scratch /tmp/gdnext-repro
```

For a play failure:

```
gdnext ci play-cell --scratch /tmp/gdnext-repro \
  --example canarybird --target <goos>/<goarch> --link <link>
cat /tmp/gdnext-repro/play-report.json
```

Each verb prints an `==> <name args>` banner matching the CI log, so
the local output reads the same as the runner.

### Add a new build target

You don't edit `gdnext.yml`. Add the row to
`product/matrix.go` (see [catalog.md → Adding a new platform](catalog.md#adding-a-new-platform)).
The next push triggers `discover-targets`, which picks the row up and
emits cells for every CI host. If the target is also `playable`
(non-empty `PlayHosts`), `discover-plays` picks it up too.

### Add a new playable target

Populate `PlayHosts` on the target's `Platform`. The build matrix
will start uploading a play artefact for it, and `discover-plays`
will emit play cells. If the play-bot needs new runtime support
(an emulator, a virtual display), wire it into the
`Install xvfb + wine` step in `gdnext-play`.

### Mark a target experimental

Set `Status: Supported | Experimental` (or just `Experimental`) on
the catalog row. The matrix verb tags the row with
`experimental: true`; the workflow's `continue-on-error: ${{
matrix.experimental }}` keeps the job green when it fails. The
failure still appears as a yellow cell in the matrix UI and shows
up in the workflow summary.

### Pin a new toolchain hash after a green run

Bump the version in `product/matrix.go`, push, wait for a green run,
then:

```
gdnext ci toolchain-checksums --repo <owner>/<repo> --format=go
```

Paste the emitted `Toolchain<Name>.KnownChecksums = []string{...}`
into `product/matrix.go`. See
[toolchains.md → Updating known checksums](toolchains.md#updating-known-checksums)
for the full flow.

### A summary step shows a regression

The summary block at the top of the run lists, per section:

- **Checks**: a row per (host × check verb), one column per run.
  Red column = that whole verb broke; red cell in only one column =
  a regression introduced in that commit.
- **Builds / Plays**: one row per `(host, target, link)`, same
  shape. The latest column shows the cell with a link to the job.
- **Toolchains**: the `Changed` ⚠ flag marks any sha256 that
  differs from the prior run. Investigate before pinning.
- **Latest failures**: tailed log excerpts for every failed job in
  the latest run.

For each failure, jump to the corresponding `gdnext ci <verb>` and
reproduce locally via the runbook above.

## Development

### Where things live

- [.github/workflows/gdnext.yml](../.github/workflows/gdnext.yml) —
  the workflow itself. Jobs, matrix expansion, cache + artefact glue,
  step wiring.
- [cmd/gdnext/internal/ci/](../cmd/gdnext/internal/ci/) — one Go file
  per verb (`build_target.go`, `play_cell.go`, `matrix.go`, ...) plus
  shared helpers (`helpers.go`, `ci.go`).
- [cmd/gdnext/internal/ci/workflow_summary.go](../cmd/gdnext/internal/ci/workflow_summary.go)
  + `workflow_sum_*.go` — the rolling summary's domain model and
  per-section renderers.
- `cmd/gdnext/internal/ci/ci.go` — DI registration. Every new verb
  gets added to `Provides` and to `NewCICommand`'s subcommand list.

### Adding a CI verb

1. Create `cmd/gdnext/internal/ci/<verb_name>.go` with the standard
   shape (see any existing verb): `XCommand` + `XActions` DI structs,
   `NewXCommand` + `NewXActions` constructors, `(*XActions).action`
   method bound via `shared.BindAction`.
2. Register both in `ci.go`'s `Provides` set:

   ```go
   do.Lazy(NewXCommand),
   do.Lazy(NewXActions),
   ```

3. Add it to `NewCICommand`'s `Commands:` list.
4. Call the verb from the relevant `gdnext.yml` step. Keep the YAML
   trivial — the verb owns the logic.

### Editing the workflow YAML

Two rules:

- **Don't reproduce verb logic in shell.** If you find yourself
  writing more than a handful of shell lines per step, lift the
  logic into a `gdnext ci <verb>` and call that.
- **Pin every third-party action by SHA.** Existing pins look like
  `actions/checkout@9c091bb21b... # v7.0.0`. The comment is the
  human-readable version; the SHA is the trust anchor. Dependabot
  bumps both atomically.
