# Toolchains

`gdnext toolchain ...` is gdnext's package manager for the external
programs it drives — godot, go, zig, llvm, the android tooling,
libgodot, etc. This doc covers when to reach for each verb, what
gdnext owns vs what your system owns, and how the supply-chain
verification works.

Specific tools and versions live in `product.ToolchainMatrix`. This
doc deliberately stays at the level of "what to do when," not "what's
in the catalog today."

- [Toolchains](#toolchains)
  - [Why this exists](#why-this-exists)
    - [Security](#security)
  - [Concepts](#concepts)
    - [Management types](#management-types)
    - [GDPath layout](#gdpath-layout)
    - [Checksums](#checksums)
  - [Verbs](#verbs)
    - [`list` — what gdnext could manage](#list--what-gdnext-could-manage)
    - [`doctor` — what gdnext sees on this host](#doctor--what-gdnext-sees-on-this-host)
    - [`install` — fetch into GDPath](#install--fetch-into-gdpath)
    - [`uninstall` — drop gd-managed installs](#uninstall--drop-gd-managed-installs)
    - [`path` — print the resolved binary](#path--print-the-resolved-binary)
  - [Runbook](#runbook)
    - [First-time setup on a fresh machine](#first-time-setup-on-a-fresh-machine)
    - [Upgrading a tool the catalog pinned to a new version](#upgrading-a-tool-the-catalog-pinned-to-a-new-version)
    - [I prefer my system's `<tool>`](#i-prefer-my-systems-tool)
    - [I want gdnext's version even though my system has one](#i-want-gdnexts-version-even-though-my-system-has-one)
    - [Removing everything gdnext installed](#removing-everything-gdnext-installed)
    - [A download failed checksum verification](#a-download-failed-checksum-verification)
    - [Seeding `KnownChecksums` from CI](#seeding-knownchecksums-from-ci)
  - [Development](#development)
    - [Where the catalog lives](#where-the-catalog-lives)
    - [Bumping a pinned version](#bumping-a-pinned-version)
    - [Updating known checksums](#updating-known-checksums)
    - [Adding a new tool](#adding-a-new-tool)

## Why this exists

A graphics.gd build pulls in a long list of third-party binaries
(godot, zig, llvm, the android SDK pieces, libgodot, ...). Without a
managed catalog, every contributor and CI runner ends up with their
own ad-hoc mix of versions installed from upstreams of varying
trustworthiness, and "works on my machine" becomes the norm. The
toolchain system exists to make every required binary explicit and
reproducible:

- **One pinned version per tool.** The catalog (`product.ToolchainMatrix`)
  is the single source of truth for what gdnext was tested against.
- **Coexist with system installs.** When you already have a tool
  installed via your package manager, gdnext uses it and stays out of
  its way. When you don't, gdnext installs into `$GDPATH` and owns
  its lifecycle. No silent overwrites.
- **Reproducible CI.** The same `gdnext toolchain install` walk runs
  locally and in CI, so a green build matrix matches what
  contributors have on disk.

### Security

Downloads cross the network from third-party hosts, so the catalog
treats every artefact as untrusted until verified. Two layers gate
each install:

1. **`KnownChecksums` in the catalog** — sha256 hashes that ship in
   `product/matrix.go`, reviewed in PRs, and are the canonical trust
   anchor. A hash bump is a code change that requires review.
2. **On-disk sidecar (`$GDPATH/checksums/<slug>-<host>.sha256`)** —
   a TOFU pin written on first successful install. Re-installs
   verify against it even when the catalog hasn't been updated yet.

A download passes when **either** source matches the observed hash.
With neither source available, the install **fails by default** —
contributors can't accidentally consume an unverified artefact.
`--skip-checksum` / `GDNEXT_SKIP_CHECKSUM=1` is the bootstrap escape
hatch and is opt-in per run; it never persists. A mismatch leaves
the failed download on disk for inspection (path + URL + hash in the
error) rather than retrying or falling back silently.

See [Checksums](#checksums) for mechanics and
[Updating known checksums](#updating-known-checksums) for the
maintainer workflow.

## Concepts

### Management types

Every resolved toolchain is one of two kinds:

- **gd-managed**: gdnext downloaded it and the binary lives under
  `$GDPATH` (default `~/gd`). gdnext owns its lifecycle — upgrades,
  reinstalls, and removals.
- **user-managed**: the binary lives somewhere else, usually because
  the user installed it via their system package manager (homebrew,
  apt, the Go installer, ...). gdnext finds it on `$PATH` and uses it
  but refuses to remove or overwrite it without an explicit opt-in.

Classification is runtime, not catalog: a tool is gd-managed iff its
resolved path is under `BuildHost.GDRootPath`. Every output surface
(`doctor`, `list`, the install/uninstall logs, the CI audit JSON)
shows this.

### GDPath layout

Everything gd-managed lives under `$GDPATH` (`~/gd` by default,
override with `--gdpath` or `GDPATH`):

```
~/gd/
  bin/        executables (godot, zig, llvm, adb, …)
  lib/        libraries (android.jar, libgodot.*.a, …)
  checksums/  <slug>-<goos>-<goarch>.sha256 sidecars
```

Sidecar filenames key on the (slug, target tuple) the download was
keyed on, so `IsLibrary` tools get one sidecar per target while
host-scoped tools get one sidecar keyed on the host tuple.

### Checksums

Every download is verified against a union of two sources:

1. **Catalog**: `product.Toolchain.KnownChecksums` — pinned at the
   catalog level, the canonical trust anchor.
2. **Sidecar**: the on-disk `<GDPath>/checksums/...sha256` written by
   the previous successful install. Acts as a TOFU pin so re-installs
   verify against the same artefact even when the catalog hasn't been
   updated yet.

A download passes when **either** source matches. When neither has a
hash to compare against (empty `KnownChecksums` AND no sidecar on
disk) the install fails by default. `--skip-checksum` (or
`GDNEXT_SKIP_CHECKSUM=1`) is the bootstrap escape hatch: it accepts
the download and writes the sidecar so future installs verify against
the freshly-seen hash.

A mismatch leaves the failed download on disk at `<install>.<version>.download`
for inspection; the error reports the path, source URL, and offending hash.

## Verbs

### `list` — what gdnext could manage

Pure catalog view. No host probing, no downloads. Use it to see every
tool the catalog declares plus its pinned version and which hosts it
can be installed on.

```
gdnext toolchain list
gdnext toolchain list --format=json    # or yaml | xml
```

Example (truncated):

```
NAME             VERSION                 PURPOSE                                              INSTALLABLE HOSTS
godot            4.6.2                   graphics                                             linux/amd64,windows/amd64,darwin/amd64,darwin/arm64
go               1.26.0                  compiling                                            linux/amd64,windows/amd64,darwin/amd64,darwin/arm64
zig              0.15.2                  cross-compiling                                      linux/amd64,windows/amd64,darwin/amd64,darwin/arm64
llvm             21.1.8                  linking iOS builds                                   linux/amd64,windows/amd64,darwin/amd64,darwin/arm64
adb              1.0.41                  launching the project on a connected android device  linux/amd64,windows/amd64,darwin/amd64,darwin/arm64
apksigner        0.9                     building the .apk                                    linux/amd64,windows/amd64,darwin/amd64,darwin/arm64
aapt2            2.19-android-13.0.0_r6  converting the exported .apk into an .aab            linux/amd64,windows/amd64,darwin/amd64,darwin/arm64
apktool          2.12.1                  converting the exported .apk into an .aab            linux/amd64,windows/amd64,darwin/amd64,darwin/arm64
bundletool       1.18.3                  converting the exported .apk into an .aab            linux/amd64,windows/amd64,darwin/amd64,darwin/arm64
android.jar      -                       converting the exported .apk into an .aab            android/amd64,android/arm64,metaquest/arm64
upx              5.0.2                   minifying builds                                     linux/amd64,windows/amd64
vpk              0.0.1298                self-updating-bundles                                linux/amd64,windows/amd64,darwin/amd64,darwin/arm64
libgodot         -                       libgodot static-link mode                            linux/amd64
libgodot-editor  -                       libgodot editor (musl host today)                    linux/amd64
ldd              -                       musl detection                                       linux/amd64
```

### `doctor` — what gdnext sees on this host

Per-tool status table for every job the current host needs. Reports
`gd` vs `user` management, status, the resolved path, and the short
SHA from the sidecar (when present). Read-only.

```
gdnext toolchain doctor
gdnext toolchain doctor --format=json   # for CI / scripting
gdnext toolchain doctor --fix           # also run `install` for missing tools
```

Example (truncated):

```
host:    linux/amd64
targets: linux/amd64,linux/arm64,windows/amd64,windows/arm64,darwin/amd64,darwin/arm64,ios/arm64,android/arm64,android/amd64,metaquest/arm64,js/wasm

NAME                           VERSION                 MANAGED  STATUS  SHA256        DETAIL
godot                          4.6.2                   gd       OK      30e6b6d141f0  /home/me/gd/bin/godot
go                             1.26.0                  user     OK      -             /home/linuxbrew/.linuxbrew/bin/go
zig                            0.15.2                  gd       OK      02aa270f183d  /home/me/gd/bin/zig
llvm                           21.1.8                  gd       OK      928da8c2a12f  /home/me/gd/bin/llvm
adb                            1.0.41                  user     OK      -             /home/linuxbrew/.linuxbrew/bin/adb
apksigner                      0.9                     gd       OK      8441bed7018d  /home/me/gd/bin/apksigner
aapt2                          2.19-android-13.0.0_r6  gd       OK      9dd86ae76ae1  /home/me/gd/bin/aapt2
apktool                        2.12.1                  gd       OK      cf6c59294179  /home/me/gd/bin/apktool
bundletool                     1.18.3                  gd       OK      649c11f74c05  /home/me/gd/bin/bundletool
android.jar (android/amd64)    -                       gd       OK      1ef3b7ae9e0d  /home/me/gd/lib/android.jar
libgodot (linux/amd64)         -                       gd       OK      3c85abc4b271  /home/me/gd/lib/libgodot.musl.amd64.a
libgodot-editor (linux/amd64)  -                       gd       OK      042c22cf9cb1  /home/me/gd/lib/libgodot.musl.editor.amd64.a
all toolchains present for every target buildable from linux/amd64
```

### `install` — fetch into GDPath

Argless walks every job, named installs a single slug. Default
behaviour:

- gd-managed and already installed → skip with hash report
- user-managed on `$PATH` → skip with a "user-managed at …" note
- not installed → download, verify, write sidecar

```
gdnext toolchain install                   # bulk
gdnext toolchain install <slug>            # single tool
gdnext toolchain install --force           # re-download all gd-managed
gdnext toolchain install <slug> --force    # adopt a user-managed slug
                                            #   into GDPath, or re-download
                                            #   a gd-managed one
gdnext toolchain install --skip-checksum   # bootstrap when no hashes pinned
```

Example (per-tool header, source URL, and full sha256 line per artefact):

```
toolchain install for linux/amd64

==> godot v4.6.2
    source: https://github.com/godotengine/godot/releases/download/4.6.2-stable/Godot_v4.6.2-stable_linux.x86_64.zip
    skip: already installed (gd-managed at /home/me/gd/bin/godot)
    sha256:30e6b6d141f0cd5bebd629ad1d0ef1324e60091bb20662d026b402ba58c59937

==> go v1.26.0
    skip: already installed (user-managed at /home/linuxbrew/.linuxbrew/bin/go)

==> zig v0.15.2
    source: https://ziglang.org/download/0.15.2/zig-x86_64-linux-0.15.2.tar.xz
    skip: already installed (gd-managed at /home/me/gd/bin/zig)
    sha256:02aa270f183da276e5b5920b1dac44a63f1a49e55050ebde3aecc9eb82f93239

==> llvm v21.1.8
    source: https://release.graphics.gd/llvm.linux.amd64
    skip: already installed (gd-managed at /home/me/gd/bin/llvm)
    sha256:928da8c2a12f1085052cd04f2877c2ed25a1f9b2492b0e654a65c39cffdd3167

==> adb v1.0.41
    source: https://release.graphics.gd/adb.linux.amd64
    skip: already installed (user-managed at /home/linuxbrew/.linuxbrew/bin/adb)

==> apksigner v0.9
    source: https://release.graphics.gd/apksigner.linux.amd64
    skip: already installed (gd-managed at /home/me/gd/bin/apksigner)
    sha256:8441bed7018d08af0d18653e9875290550ccc27f9f0f7768f12a59e28d60fbe0

==> aapt2 v2.19-android-13.0.0_r6
    source: https://release.graphics.gd/aapt2.linux.amd64
    skip: already installed (gd-managed at /home/me/gd/bin/aapt2)
    sha256:9dd86ae76ae12d263672c4c454f17b30e42bb9792b3e2c0ce9d68b33fd5a7d37

==> apktool v2.12.1
    source: https://release.graphics.gd/apktool.linux.amd64
    skip: already installed (gd-managed at /home/me/gd/bin/apktool)
    sha256:cf6c59294179c86d0778a15b0027197b5cbddfce2c95b7c8f5cb31b6d9705ebd

==> bundletool v1.18.3
    source: https://release.graphics.gd/bundletool.linux.amd64
    skip: already installed (gd-managed at /home/me/gd/bin/bundletool)
    sha256:649c11f74c05f76241362a496005ab81f887c48c4b9e6226260e7f0c68183ced

==> android.jar (android/amd64)
    source: https://release.graphics.gd/android.jar
    skip: already installed (gd-managed at /home/me/gd/lib/android.jar)
    sha256:1ef3b7ae9e0dd44d01958e798a75593e8ed1a948e309932b30a691312867249f

==> android.jar (android/arm64)
    source: https://release.graphics.gd/android.jar
    skip: shares artefact with a previous job (/home/me/gd/lib/android.jar)

==> android.jar (metaquest/arm64)
    source: https://release.graphics.gd/android.jar
    skip: shares artefact with a previous job (/home/me/gd/lib/android.jar)

==> libgodot (linux/amd64)
    source: https://release.graphics.gd/libgodot.musl.amd64.a
    skip: already installed (gd-managed at /home/me/gd/lib/libgodot.musl.amd64.a)
    sha256:3c85abc4b2711dd08a97cb1d58ea3d9833ea98709e62c9ab3264b6c535efbe4c

==> libgodot-editor (linux/amd64)
    source: https://release.graphics.gd/libgodot.musl.editor.amd64.a
    skip: already installed (gd-managed at /home/me/gd/lib/libgodot.musl.editor.amd64.a)
    sha256:042c22cf9cb1952be0ba83bdcc45154d9dadd44d0d7bee269da67cb06a66dcef

Summary: 0 installed, 10 already gd-managed, 2 already user-managed, 0 skipped (experimental), 0 failed.
```

`--force` never touches user-managed tools in **bulk** mode — that
would silently overwrite system installs. To install gdnext's pinned
copy of a system-managed tool, name the slug explicitly with `--force`.

### `uninstall` — drop gd-managed installs

Removes the binary and (by default) the sidecar. Refuses
user-managed tools — those are your package manager's job.

```
gdnext toolchain uninstall <slug>
gdnext toolchain uninstall <slug> --keep-checksum   # keep the sha256 pin
gdnext toolchain uninstall --all                    # all gd-managed
```

Example (`--all` walk, mixing removed, user-managed, and not-installed):

```
toolchain uninstall for linux/amd64

==> godot v4.6.2
    removed binary: /home/me/gd/bin/godot
    removed checksum: /home/me/gd/checksums/godot-linux-amd64.sha256

==> go v1.26.0
    found at /home/linuxbrew/.linuxbrew/bin/go
    skip: user-managed (gdnext didn't install this; leaving it alone)

==> zig v0.15.2
    removed binary: /home/me/gd/bin/zig
    removed checksum: /home/me/gd/checksums/zig-linux-amd64.sha256

==> llvm v21.1.8
    removed binary: /home/me/gd/bin/llvm
    removed checksum: /home/me/gd/checksums/llvm-linux-amd64.sha256

==> adb v1.0.41
    found at /home/linuxbrew/.linuxbrew/bin/adb
    skip: user-managed (gdnext didn't install this; leaving it alone)

==> apksigner v0.9
    removed binary: /home/me/gd/bin/apksigner
    removed checksum: /home/me/gd/checksums/apksigner-linux-amd64.sha256

==> aapt2 v2.19-android-13.0.0_r6
    removed binary: /home/me/gd/bin/aapt2
    removed checksum: /home/me/gd/checksums/aapt2-linux-amd64.sha256

==> apktool v2.12.1
    removed binary: /home/me/gd/bin/apktool
    removed checksum: /home/me/gd/checksums/apktool-linux-amd64.sha256

==> bundletool v1.18.3
    removed binary: /home/me/gd/bin/bundletool
    removed checksum: /home/me/gd/checksums/bundletool-linux-amd64.sha256

==> android.jar
    removed binary: /home/me/gd/lib/android.jar
    removed checksum: /home/me/gd/checksums/android.jar-android-amd64.sha256

==> upx v5.0.2
    skip: not installed

==> vpk v0.0.1298
    skip: not installed

==> libgodot
    removed binary: /home/me/gd/lib/libgodot.musl.amd64.a
    removed checksum: /home/me/gd/checksums/libgodot-linux-amd64.sha256

==> libgodot-editor
    removed binary: /home/me/gd/lib/libgodot.musl.editor.amd64.a
    removed checksum: /home/me/gd/checksums/libgodot-editor-linux-amd64.sha256

==> ldd
    found at /usr/bin/ldd
    skip: user-managed (gdnext didn't install this; leaving it alone)

Summary: 10 removed, 3 user-managed (skipped), 2 not installed, 0 failed.
```

`--keep-checksum` is the safety net for "I want to rebuild this tool
but lock the next install to the same hash."

### `path` — print the resolved binary

Lookup-only. Prints the absolute path the next exec would use, or
errors if the tool is missing.

```
gdnext toolchain path <slug>
```

Example:

```
$ gdnext toolchain path zig
/home/me/gd/bin/zig
```

## Runbook

### First-time setup on a fresh machine

```
gdnext toolchain install
```

If you've never seeded `KnownChecksums` in your fork of the catalog,
the first run will fail strict verification. Bootstrap once:

```
GDNEXT_SKIP_CHECKSUM=1 gdnext toolchain install
```

That writes sidecars for every artefact. Subsequent runs without the
env var verify against those sidecars.

### Upgrading a tool the catalog pinned to a new version

The catalog bump arrives via `git pull`. Then:

```
gdnext toolchain install <slug> --force
```

`--force` re-downloads even though the old binary is still on disk,
verifies against the catalog's new hash (or the sidecar — whichever
is set), and rewrites the sidecar.

### I prefer my system's `<tool>`

Just install it via your package manager. gdnext finds it on `$PATH`
and treats it as user-managed. `doctor` will show `MANAGED=user` and
no sha256. `install` will skip it.

### I want gdnext's version even though my system has one

```
gdnext toolchain install <slug> --force
```

This pulls the pinned copy into `$GDPATH/bin/<slug>`. Because gdnext
checks `$GDPATH` before `$PATH` during lookup, the new copy wins on
subsequent invocations. Your system copy is untouched.

### Removing everything gdnext installed

```
gdnext toolchain uninstall --all
```

Walks every gd-managed slug, refuses every user-managed one with a
reported skip line. Sidecars are removed by default — pass
`--keep-checksum` to retain them.

### A download failed checksum verification

The error tells you the source URL, the path the partial download was
kept at, and the observed hash. Decide:

- **It's a flake / mirror glitch** → delete the partial, retry:
  `rm <install>.<version>.download && gdnext toolchain install <slug> --force`.
- **You trust this artefact and want to lock it in** →
  `gdnext toolchain install <slug> --force --skip-checksum`. The
  sidecar will pin the hash for future installs.
- **The catalog's pinned hash is stale and needs updating** → harvest
  the new hash and update `product.Toolchain.KnownChecksums`. See
  [Seeding `KnownChecksums` from CI](#seeding-knownchecksums-from-ci)
  for the workflow-driven path.

### Seeding `KnownChecksums` from CI

Every CI run uploads a `toolchain-audit-<runner>` artefact containing
the full `doctor --format=json` output, with `sha256` and `source`
per job. To lock the catalog to a known-good run:

1. Pull the audit artefact from a green CI run on `main` (or a
   trusted branch).
2. For each `(slug, goos, goarch)` row, copy the `sha256` value into
   the matching `product.Toolchain.KnownChecksums` slice.
3. Drop the `GDNEXT_SKIP_CHECKSUM=1` env var from the workflow's
   install step. Subsequent runs will verify against the catalog.

After seeding, the workflow can run without the bootstrap escape
hatch; deviations show up as mismatched-hash failures in the install
step before the build matrix starts.

## Development

For maintainers updating the catalog itself, not just consuming it.

### Where the catalog lives

- `product/matrix.go` — `ToolchainMatrix` plus the per-tool
  `Toolchain<Name>` values (`ToolchainGodot`, `ToolchainZig`, ...).
  Category slices (`SharedToolchains`, `AndroidToolchains`,
  `LibGodotToolchains`) decide which jobs each target picks up.
- `product/toolchain.go` — the `Toolchain` struct definition. All
  fields are pure data; the runtime side (download, verify, install,
  uninstall, exec) lives in `cmd/gdnext/internal/tooling`.

The catalog is the source of truth. Don't hard-code versions or URLs
in the tooling package — add or extend a `Toolchain` field instead.

### Bumping a pinned version

1. Edit `Version` on the matching `Toolchain<Name>` value in
   `product/matrix.go`. If upstream's URL template changed, update
   `DownloadURL` / `DownloadOS` / `DownloadARCH` / `Unzip` to match.
2. Rebuild gdnext and harvest the new hash locally:

   ```
   go -C cmd/gdnext build -o /tmp/gdnext .
   /tmp/gdnext toolchain install <slug> --force --skip-checksum
   ```

   `--force` re-downloads even when an older copy is on disk.
   `--skip-checksum` is required because `KnownChecksums` still pins
   the old hash. The install log prints `sha256:<hex>` for the new
   artefact.
3. **Verify the printed hash matches the one upstream publishes**
   (release notes, signed manifest, etc.) before pasting it in. The
   point of `KnownChecksums` is to anchor trust at the catalog
   level; copying a hash you didn't verify defeats it.
4. Replace the entry in `Toolchain<Name>.KnownChecksums` for **every**
   download tuple the tool covers. Host-scoped tools need one entry
   per `AvailableHosts` row that downloads a distinct artefact;
   `IsLibrary` tools need one per target the artefact serves.
5. Re-run without the escape hatch to confirm strict verify passes:

   ```
   /tmp/gdnext toolchain install <slug> --force
   ```

6. `go -C cmd/gdnext test ./...` and commit the catalog edit + the
   checksum update together.

### Updating known checksums

`KnownChecksums` is the catalog-level trust anchor. The sidecar in
`$GDPATH/checksums/` is only a per-host TOFU pin — it doesn't help
other contributors or CI. Always land catalog hashes when you bump
or add a tool.

CI uploads `toolchain-audit-<runner>` artefacts containing the full
`doctor --format=json` output for every matrix row.
`gdnext ci toolchain-checksums` walks the most recent successful
workflow run, downloads every audit artefact, and renders the
aggregated `(slug, goos, goarch) → sha256` view ready to paste into
`product/matrix.go`:

```
gdnext ci toolchain-checksums --repo <owner>/<repo>
gdnext ci toolchain-checksums --repo <owner>/<repo> --format=go
gdnext ci toolchain-checksums --repo <owner>/<repo> --run-id <id>
```

`--format=go` emits a `Toolchain<Name>.KnownChecksums = []string{…}`
block per slug; defaults to `--format=table`. Hash conflicts across
runners (which should never happen for content-addressed artefacts)
are flagged inline so a tampered mirror surfaces immediately. The
maintainer still owns the paste and MUST verify each hash matches
what upstream publishes before committing.

See [Seeding `KnownChecksums` from CI](#seeding-knownchecksums-from-ci)
for the bootstrap variant (when `KnownChecksums` is still empty).

### Adding a new tool

1. Declare a `Toolchain<Name>` var in `product/matrix.go`. Required
   fields: `Slug`, `Version`, `RequiredFor`, `AvailableHosts`, and
   either `DownloadURL` (with `$(VERSION)` / `$(OS)` / `$(ARCH)`
   placeholders) or `Downloads` for per-host overrides. Set `IsApp`
   for executables, `IsLibrary` for artefacts that belong under
   `lib/`.
2. Append the var to `ToolchainMatrix` and, if it's category-shared
   (android, libgodot, ...), to the matching slice
   (`AndroidToolchains`, `LibGodotToolchains`, ...). This is what
   makes `doctor` and `install` pick it up.
3. Harvest and pin checksums via the
   [Bumping a pinned version](#bumping-a-pinned-version) flow above.
4. Run `/tmp/gdnext toolchain doctor` and confirm the new entry
   shows up for every target that needs it.
