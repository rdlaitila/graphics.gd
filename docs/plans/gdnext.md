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
- **CI driver — `cmd/gdnext-ci/`.** Go binary, one verb per workflow
  phase. The workflow YAML at `.github/workflows/gdnext-ci.yml` is a
  thin wrapper; the help text on each verb is the canonical
  documentation for what it asserts.
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

## Out of scope

- **No edits to `cmd/gd`.** The legacy binary stays intact during the
  POC.
- **No new external dependencies** beyond `urfave/cli/v3` and
  `yaml.v3` (for `gdnext platforms --format yaml`).
- **No iOS / signed-macOS end-to-end CI.** Both need real Apple certs
  that can't live in a public repo.
- **No `gdnext run` in CI.** Needs a windowed GPU session;
  `gdnext test` exercises the same setup pipeline headlessly.
