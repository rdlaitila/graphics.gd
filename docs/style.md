# graphics.gd style

Conventions for new code. Existing code may differ; do not churn-format
unrelated files when making a change.

- [graphics.gd style](#graphicsgd-style)
  - [General](#general)
    - [Prefer consistency over variants](#prefer-consistency-over-variants)
  - [Go](#go)
    - [Order declarations top-down](#order-declarations-top-down)
    - [No spurious blank lines](#no-spurious-blank-lines)
    - [Name method receivers `t`](#name-method-receivers-t)
    - [Don't over-comment](#dont-over-comment)
    - [Align struct field tags](#align-struct-field-tags)
  - [Git](#git)
    - [Use Conventional Commits](#use-conventional-commits)


## General

### Prefer consistency over variants

When writing new code or conducting a refactor, follow the shape the
neighbouring code already uses — even when a different shape would be
marginally cleaner for the one case in front of you. One shared
pattern across a directory, package, or subsystem is worth more than a
locally-optimal variant.

Concretely:

- Match the names, field order, constructor signatures, and method
  groupings of sibling files. 
- Match the file layout of the rest of the directory (see
  [the Go ordering rule](#order-declarations-top-down)
  for the in-file shape).
- Match the existing naming scheme (verbs vs. nouns, plural vs.
  singular, `Catalog` vs. `Registry`) rather than introducing a
  near-synonym for one new entry.
- Match the existing error-handling, logging, and DI conventions even
  when a one-off would be terser.

When a refactor changes the pattern, apply the new shape to every
sibling already in scope of the change — don't leave a half-converted
file behind. This is not a licence to churn unrelated files: the
[intro](#graphicsgd-style) still applies. The rule is "stay
consistent within the blast radius of the diff you're already
making," not "rewrite the world to match."

**Rationale:** every variant is a small tax on every future reader.
They have to notice the difference, decide whether it's meaningful,
and remember which variant applies where. Consistency makes the
codebase skimmable: pick up one file, and the next twelve read the
same way. It also makes large-scale edits (cross-file renames, codemod
passes, AI-assisted refactors) reliable instead of a per-file
adventure.

The escape hatch is "when prudent". If matching the existing pattern
would force a clearly worse design — duplicating substantial logic,
introducing a real bug, papering over a genuine semantic difference —
don't. But the bar is "clearly worse," not "marginally less elegant."


## Go

### Order declarations top-down

A reader (human or agent) opening an unfamiliar `.go` file should be
able to skim it once, top to bottom, and pick up the shape before the
behaviour. Order declarations the way they're discovered:

1. **Types** — the nouns of the file. Group related types together
   (e.g. a `*XxxCommand` and its companion `*XxxActions`).
2. **Package-level vars and consts** — fixed state and tables the
   types and functions below operate on.
3. **Constructors** — the `NewXxx` factories, one per type, in the
   same order the types appear above.
4. **Methods** — grouped by receiver type, with receivers appearing
   in the same order as the type declarations. Within one receiver,
   public methods before private; otherwise call order or logical
   pairing (e.g. `Encode` next to `Decode`).
5. **Unexported helpers** — package-level functions used by the
   methods above. Last, so the load-bearing API stays at the top of
   the file.

Deviate when it actively helps the reader: a tiny helper used by
exactly one function can sit immediately below that function; a
cohesive trio (type + constructor + its two methods) can stay
clustered even if it breaks the global ordering. The rule is a
default, not a straitjacket.

**Rationale:** the layout mirrors the dependency direction —
constructors reference types, methods reference constructors and
types, helpers reference everything. Reading top-down is reading in
dependency order, so each declaration is fully defined by the time it
appears. It also collapses the "where does Foo live?" search: types at
the top, factories next, behaviour in the middle, plumbing at the
bottom. Every file in the repo following the same shape compounds the
benefit.

**Avoid**

```go
func helperDecode(b []byte) string { /* ... */ }

func (t *BuildActions) build(ctx context.Context, cmd *cli.Command) error {
    /* ... */
}

func NewBuildActions(di do.Injector) (*BuildActions, error) {
    return do.InvokeStruct[*BuildActions](di)
}

type BuildActions struct {
    Injector do.Injector `do:""`
}

var defaultLDFlags = []string{"-s", "-w"}

type BuildCommand struct {
    *cli.Command
    Injector do.Injector `do:""`
}

func NewBuildCommand(di do.Injector) (*BuildCommand, error) {
    /* ... */
}
```

**Prefer**

```go
// types
type BuildCommand struct {
    *cli.Command
    Injector do.Injector `do:""`
}

type BuildActions struct {
    Injector do.Injector `do:""`
}

// vars
var defaultLDFlags = []string{"-s", "-w"}

// constructors (types order)
func NewBuildCommand(di do.Injector) (*BuildCommand, error) { /* ... */ }

func NewBuildActions(di do.Injector) (*BuildActions, error) {
    return do.InvokeStruct[*BuildActions](di)
}

// methods (receivers in types order)
func (t *BuildActions) build(ctx context.Context, cmd *cli.Command) error {
    /* ... */
}

// helpers
func helperDecode(b []byte) string { /* ... */ }
```

### No spurious blank lines

Spacing is subjective — the same blank line reads as a logical
boundary to one author and as noise to another. Inside a type or
function body, drop blank lines used as breathing room. When a section
genuinely needs separation, name it with a single or multiline comment instead.

**Rationale:** code is read far more often than it is written; a
consistent block layout lowers the mental cost of skimming and reserves
visual structure for moments where an explicit comment also signals
intent.

**Avoid**

```go
type Canary struct {
    Node3D.Extension[Canary]

    Tweeted Signal.Solo[string]

    score int

    rng *rand.Rand
}

func (c *Canary) tick(delta Float.X) {
    c.velocity -= gravity * delta

    pos := c.bird.Position()
    pos.Y += c.velocity * delta
    c.bird.SetPosition(pos)

    if pos.Y < floorY {
        c.gameOver()
    }
}
```

**Prefer**

```go
type Canary struct {
    Node3D.Extension[Canary]
    Tweeted Signal.Solo[string]
    score   int
    rng     *rand.Rand
}

func (t *Canary) tick(delta Float.X) {
    // physics
    t.velocity -= gravity * delta
    pos := t.bird.Position()
    pos.Y += t.velocity * delta
    t.bird.SetPosition(pos)
    // collision
    if pos.Y < floorY {
        t.gameOver()
    }
}
```

### Name method receivers `t`

Every method receiver uses the single-letter name `t`, regardless of
the enclosing type. This ensures all usage sites are instantly reconizable. 

**Rationale:** receiver names communicate nothing about the type —
the signature already does. A per-type initial (`c` for `Canary`,
`s` for `Server`, etc.) forces the reader to mentally remap on every file. A
fixed name lets every method body across the codebase be skimmed the same way.

**Avoid**

```go
func (c *Canary) Tweet(song string) { c.chirper.Play() }
func (s *Server) Serve()             { s.listener.Accept() }
```

**Prefer**

```go
func (t *Canary) Tweet(song string) { t.chirper.Play() }
func (t *Server) Serve()             { t.listener.Accept() }
```

### Don't over-comment

Treat code as self-describing by default — Go's naming, types, and
control flow already tell a reader most of what's happening. Reach for
a comment when the language can't carry the meaning on its own, and
then be deliberate about it.

**Public interfaces** (exported types, functions, methods, package
docs) get a comment so the package-level API doc renders something
useful. Keep it concise: one or two sentences naming what the symbol
is and what a caller is expected to do with it. Don't reach for a
dedicated comment on every struct field — group related fields under a
shared header comment, and skip the comment entirely when the field
name is self-evident.

**Function bodies** are where comments most often go wrong. Reserve
them for moments that need to context-switch a human reader or
agent: a non-obvious workaround, a quirk in an upstream API, a
hidden invariant, a reference to the issue or paper that explains
the algorithm, or a real logical boundary inside a long routine.
Don't paraphrase the next line; if the code can be read straight
through, no annotation is helping.

When in doubt, ask whether deleting the comment would leave the
reader any worse off. If not, delete it.

**Rationale:** every comment is a second source of truth that drifts
out of sync the moment the code beneath it changes. Sparse, deliberate
comments stay accurate and earn the reader's trust; pervasive ones
become wallpaper, get skimmed past, and quietly start lying. For an AI
agent reading the code, redundant comments are doubly costly — they
inflate context, distract from the load-bearing signal, and amplify
any drift.

**Avoid**

```go
// Canary represents a canary bird.
type Canary struct {
    // chirper is the AudioStreamPlayer used to chirp.
    chirper AudioStreamPlayer.Instance
    // score is the current score.
    score int
}

func (t *Canary) tick(delta Float.X) {
    // update velocity using gravity
    t.velocity -= gravity * delta
    // get the current position
    pos := t.bird.Position()
    // advance position by velocity
    pos.Y += t.velocity * delta
    // write the new position back
    t.bird.SetPosition(pos)
}
```

**Prefer**

```go
// Canary is the player avatar: a yellow sphere that flaps through
// scrolling cloud obstacles.
type Canary struct {
    chirper AudioStreamPlayer.Instance
    score   int
}

func (t *Canary) tick(delta Float.X) {
    t.velocity -= gravity * delta
    pos := t.bird.Position()
    pos.Y += t.velocity * delta
    t.bird.SetPosition(pos)
}
```

### Align struct field tags

When a struct's fields carry tags — particularly multi-encoder tags like
`json` + `xml` + `yaml` — align them into columns when prudent. The eye
should be able to scan one encoder vertically without sliding sideways
for every row.

Pick a consistent encoder order for the struct and stick to it across
every row; separate consecutive tags with enough spaces that each one
starts in the same visual column. If a tag is missing for a given
encoder, leave the column empty where possible rather than collapsing
the spacing — the alignment is the point. `gofmt` preserves the inner
whitespace of the backtick literal, so the columns survive a save.

Use judgement: a single tag, very long values, or a struct with wildly
mismatched fields can make strict alignment hurt more than it helps.
Drop the columns there. Inline field comments or grouping comments
also break the visual flow — when one appears mid-struct, reflow the
alignment per-section so each group is internally consistent rather
than forcing the whole struct to share a single column width.

**Rationale:** struct tags are dense, low-information glue. Aligning
them turns the block into a small two-dimensional table the reader can
treat as data rather than prose; misalignment forces a per-row reparse
of which token belongs to which encoder.

**Avoid**

```go
type Platform struct {
    XMLName xml.Name `json:"-" xml:"platform" yaml:"-"`
    Title string `json:"title,omitempty" yaml:"title,omitempty" xml:"title,attr,omitempty"`
    GOOS string `xml:"goos" json:"goos" yaml:"goos"`
    GOARCH string `json:"goarch" xml:"goarch" yaml:"goarch"`
}
```

**Prefer**

```go
type Platform struct {
    XMLName xml.Name `json:"-"               xml:"platform"             yaml:"-"`
    Title   string   `json:"title,omitempty" xml:"title,attr,omitempty" yaml:"title,omitempty"`
    GOOS    string   `json:"goos"            xml:"goos"                 yaml:"goos"`
    GOARCH  string   `json:"goarch"          xml:"goarch"               yaml:"goarch"`
}
```

When a comment splits the struct, use best judgement to re-align each section locally so the
columns stay scannable inside the group even if they no longer match
between groups:

```go
type Platform struct {
    // canonical identity
    GOOS   string `json:"goos"   xml:"goos"   yaml:"goos"`
    GOARCH string `json:"goarch" xml:"goarch" yaml:"goarch"`
    // serialisation metadata
    Aliases   []string `json:"aliases,omitempty"   xml:"aliases>alias,omitempty"      yaml:"aliases,omitempty"`
    Renderers []string `json:"renderers,omitempty" xml:"renderers>renderer,omitempty" yaml:"renderers,omitempty"`
}
```

## Git

### Use Conventional Commits

Commit subjects follow [Conventional Commits](https://www.conventionalcommits.org/):
`type(scope): summary`. The type and scope make `git log --oneline` and
release tooling readable at a glance; the summary stays imperative,
lowercase, and under ~72 characters with no trailing period.

Common types in this repo:

- `feat` — user-facing capability added
- `fix` — bug fix
- `refactor` — code restructure with no behaviour change
- `docs` — documentation only
- `test` — tests only
- `ci` — workflow / CI driver changes
- `chore` — tooling, deps, gitignore, etc.

Pick the **narrowest accurate scope** — usually a package or component:
`feat(gdnext)`, `refactor(product)`, `fix(android)`, `ci(workflow)`,
`docs(plans)`. Skip the scope only when a change genuinely spans the
whole module.

**Avoid**

```
Added a flag to make platforms output vertical.
Update some files
WIP fixing the thing
```

**Prefer**

```
feat(gdnext): add --vertical flag to platforms subcommand
refactor(product): move Status to product.go for cross-entity reuse
ci(gdnext): derive install matrix from product.ToolchainMatrix
```

**Rationale:** consistent commit shapes turn history into a queryable
log. `git log --grep '^feat'` surfaces every feature in scope order;
`git log --grep '^fix(android)'` finds every android regression. Free-
form subjects break those queries and leave changelog generators with
nothing to anchor on.
