# darwin host -> linux/* libgodot: Go emits empty c-archive

Status: **unresolved**, seen on `macos-latest` (arm64) GitHub runner,
Go 1.26.4, zig 0.15.2.

## Symptom

`gdnext -goos linux -goarch amd64 -link libgodot build` (or `arm64`)
on a darwin/arm64 host runs `go build -tags archive -buildmode=c-archive`
without error, then the subsequent zig-cc link fails with:

```
ld.lld: error: undefined symbol: main
>>> referenced by start-2.33.S:104 (…/glibc/sysdeps/x86_64/start-2.33.S:104)
>>>               …/crt1.o:(_start)
```

The Go step exits 0 and produces a 96-byte archive whose entire content
is one BSD symbol-table sentinel:

```
0000  21 3c 61 72 63 68 3e 0a 23 31 2f 32 30 20 20 20  |!<arch>.#1/20   |
0010  20 20 20 20 20 20 20 20 31 37 38 33 31 34 32 35  |        17831425|
0020  33 33 20 20 35 30 31 20 20 20 32 30 20 20 20 20  |33  501   20    |
0030  31 30 30 36 34 34 20 20 32 38 20 20 20 20 20 20  |100644  28      |
0040  20 20 60 0a 5f 5f 2e 53 59 4d 44 45 46 20 53 4f  |  `.__.SYMDEF SO|
0050  52 54 45 44 00 00 00 00 00 00 00 00 00 00 00 00  |RTED............|
```

`zig ar t` returns 0 members. `nm --defined-only` finds no symbols.
Every cgo `.o` (including the cgo-emitted `main()` wrapper for
`package main` c-archive) that Go compiled during the same `-x` trace
never lands in the output archive.

## Repro

Darwin/arm64 host, Go 1.26.4, zig 0.15.2 on PATH:

```bash
cd examples/canarybird
export GOOS=linux GOARCH=amd64 CGO_ENABLED=1
export CC="/path/to/zig cc -target x86_64-linux-gnu.2.28"
go build -tags archive -buildmode=c-archive -o /tmp/x.a -ldflags='-s -w' .
ls -l /tmp/x.a          # 96 bytes on darwin, ~63 MB on linux
```

Linux/amd64 host, same commands: archive is ~63 MB, `nm` shows `T main`,
zig-cc link succeeds.

## What we know

- Env into `go build` is correct: `GOOS=linux GOARCH=amd64
  CGO_ENABLED=1 CC="zig cc -target x86_64-linux-gnu.2.28"`. Verified
  by our `setGoCrossEnv` diagnostic.
- Go's `-x` trace on darwin does run cgo for every reachable package
  (compiles `_cgo_gotypes.go`, `_cgo_main.c`, all `_xNNN.o` objects).
- Several packages report `# test for internal linking errors (failed)`
  during Go's internal-vs-external linker probe. These are informational
  — Go falls back to external linking, which we want.
- The final link step is:
  ```
  GOROOT=… /pkg/tool/darwin_arm64/link -o $WORK/b001/exe/a.out.a
    -buildmode=c-archive -s -w
    "-extld=/…/zig cc -target x86_64-linux-gnu.2.28"
    $WORK/b001/_pkg_.a
  ```
  Go's linker binary is `darwin_arm64` because Go ships one linker
  per host arch — that's normal. It should still produce a valid
  target-arch archive.
- The linker succeeds silently and moves a 96-byte file to the
  requested output.
- `file` on the output reports `current ar archive random library`
  (BSD format, standard on darwin).
- No object members. Not a `SYMDEF` corruption issue — there are no
  `.o` files at all, only the sentinel.

## What we ruled out

- Toolchain cache: `-a` (force full rebuild) reproduces.
- Stale cgo output: verbose `-x` shows cgo running, compiling `.o` files
  that should be included.
- Env mismatch: hostile-env variants (`unset CC`, host cc, plain gcc)
  all break earlier with clearer errors; the failure only manifests
  with `zig cc -target linux-gnu`.
- Post-processing corruption: our earlier `ar s` / `zig ar` repack
  wasn't the cause — the raw file straight out of Go is 96 bytes.
- Rosetta interference: build cell runs natively on arm64, not under
  Rosetta.

## What we tried, all without effect

- `os.Setenv("GOOS","linux")` + `GOARCH` + `CGO_ENABLED=1` before
  `go build` invocation.
- `ar s <archive>` to force darwin BSD ar to rewrite the symbol table.
- `-Wl,-u,main` on the subsequent zig-cc link (forces the linker to
  treat `main` as an initial undefined ref).
- `zig ar x <archive>` extract + `zig ar rcs <new> …` repack to
  normalise BSD → GNU-format archive.
- `-a -x` on `go build` to bust the build cache and trace every
  subcommand.

None of these help because the archive Go writes on darwin is
genuinely empty.

## Suspected root cause (unverified)

Go's `-buildmode=c-archive` on darwin with a non-darwin GOOS may treat
the output as "no host contribution needed" and skip packaging the
cgo `.o` set — deferring everything to `extld` — then run extld
against nothing. Same c-archive build on a linux host packages the
cgo `.o` files directly into the archive. Feels like a Go toolchain
bug or a documented "don't do that" for darwin cross-cgo we haven't
found yet.

Worth searching upstream Go issues for
`buildmode=c-archive darwin cross-compile empty` and
`buildmode=c-archive extld linux from darwin`.

## Workaround for CI

Add a `QuirkCIBuildBroken` on `PlatformLinuxAmd64` +
`PlatformLinuxArm64` scoped to `LinkModes: [LibGodot]` and
`Hosts: [darwin/*]`, so the (macos-latest, linux/*, libgodot) build
cells are omitted from the matrix. The gdextension link path on the
same (darwin -> linux) pair builds fine because it uses
`-buildmode=c-shared` (not c-archive), which packages the `.o` files
correctly.

`CIBlockedFor` currently ignores link mode; adding link-mode filtering
mirrors the existing `PlayBlockedFor` shape (uses `AppliesToLinkMode`).

## What a real fix would look like

Any one of:

- File an upstream Go bug and wait for a fix in cgo's c-archive
  output packaging on darwin.
- Cross-compile `gdnext libgodot build --link libgodot` on a linux
  host instead, then upload the resulting `linux_<arch>.libgodot.a`
  as an artefact the darwin cell can consume (adds cross-job
  dependency, big change).
- Skip `-buildmode=c-archive` entirely on darwin cross builds: emit
  a `.so` via `-buildmode=c-shared` and change the zig link to dlopen
  it. That defeats the point of the single-file libgodot binary
  though.

## Related

- `cmd/gdnext/internal/builder/linux.go` — `libgodotBuildGlibc`,
  `libgodotBuildMusl`, `normalizeGoArchive`, `diagLibgodotArchive`.
- `product/quirks.go` — `QuirkWindowsDarwinBuildAccessDenied` is the
  precedent for host-scoped `QuirkCIBuildBroken`.
- Failing run:
  `https://github.com/rdlaitila/graphics.gd/actions/runs/28696005483/job/85105126857`
