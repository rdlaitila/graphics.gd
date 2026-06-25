package ci

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"text/tabwriter"

	"graphics.gd/cmd/gdnext/internal/shared"
	"graphics.gd/product"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
)

// ToolchainChecksumsCommand wires `gdnext ci check-toolchain-checksums`,
// the maintainer-side harvest helper for product.Toolchain.KnownChecksums.
// It scans recent workflow runs for the first successful one, downloads
// every `toolchain-audit-<runner>` artefact, aggregates the
// (slug, goos, goarch) → sha256 view, and renders it for paste into
// product/matrix.go. Maintainers MUST still verify each printed hash
// matches what upstream publishes before committing the catalog edit.
type ToolchainChecksumsCommand struct {
	*cli.Command
	Injector do.Injector `do:""`
}

// ToolchainChecksumsActions carries the runtime state.
type ToolchainChecksumsActions struct{}

// NewToolchainChecksumsCommand constructs the toolchain-checksums subcommand.
func NewToolchainChecksumsCommand(di do.Injector) (*ToolchainChecksumsCommand, error) {
	t := do.MustInvokeStruct[*ToolchainChecksumsCommand](di)
	t.Command = &cli.Command{
		Name:  "check-toolchain-checksums",
		Usage: "harvest toolchain SHA256s from the latest successful workflow run",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "repo", Required: true, Usage: "owner/repo (e.g. grow-graphics/gd)"},
			&cli.StringFlag{Name: "workflow", Value: "gdnext.yml", Usage: "workflow filename"},
			&cli.StringFlag{Name: "branch", Usage: "limit to a single branch (default: any)"},
			&cli.IntFlag{Name: "run-id", Usage: "override: pull this specific run instead of latest success"},
			&cli.IntFlag{Name: "scan", Value: 25, Usage: "max recent runs to scan for a successful one"},
			&cli.StringFlag{Name: "format", Value: "table", Usage: "table | json | go"},
		},
		Action: shared.BindAction(t.Injector, (*ToolchainChecksumsActions).action),
	}
	return t, nil
}

// NewToolchainChecksumsActions resolves the runtime state.
func NewToolchainChecksumsActions(di do.Injector) (*ToolchainChecksumsActions, error) {
	return do.InvokeStruct[*ToolchainChecksumsActions](di)
}

func (t *ToolchainChecksumsActions) action(_ context.Context, cmd *cli.Command) error {
	repo := cmd.String("repo")
	workflow := cmd.String("workflow")
	branch := cmd.String("branch")
	override := int64(cmd.Int("run-id"))
	scan := int(cmd.Int("scan"))
	format := strings.ToLower(cmd.String("format"))

	run, err := pickChecksumRun(repo, workflow, branch, override, scan)
	if err != nil {
		return err
	}
	rows := fetchAuditArtifacts(repo, run.ID)
	if len(rows) == 0 {
		return fmt.Errorf("no toolchain-audit-* artefacts on run %d (%s)", run.ID, run.HTMLURL)
	}
	fmt.Fprintf(os.Stderr, "harvesting run #%d %s@%s — %s\n",
		run.Number, branchOrAny(run.HeadBranch), shortSHA(run.HeadSHA), run.HTMLURL)

	entries := aggregateChecksums(rows)
	switch format {
	case "", "table":
		return renderChecksumsTable(os.Stdout, entries)
	case "json":
		return renderChecksumsJSON(os.Stdout, entries)
	case "go":
		return renderChecksumsGo(os.Stdout, entries)
	default:
		return fmt.Errorf("unknown --format %q (want table | json | go)", format)
	}
}

// pickChecksumRun returns the run to harvest from. --run-id forces a
// specific run; otherwise the most recent successful run within `scan`
// is used. Failing or in-progress runs are skipped — KnownChecksums
// should only be pinned from a green build.
func pickChecksumRun(repo, workflow, branch string, override int64, scan int) (ghRun, error) {
	if override != 0 {
		return fetchRunByID(repo, override)
	}
	runs, err := fetchRuns(repo, workflow, scan, branch)
	if err != nil {
		return ghRun{}, fmt.Errorf("fetch runs: %w", err)
	}
	for _, r := range runs {
		if r.Status == "completed" && r.Conclusion == "success" {
			return r, nil
		}
	}
	scope := branchOrAny(branch)
	return ghRun{}, fmt.Errorf("no successful %s run found on %s in the last %d", workflow, scope, scan)
}

// fetchRunByID retrieves a single run via the GitHub API.
func fetchRunByID(repo string, runID int64) (ghRun, error) {
	out, err := exec.Command("gh", "api",
		fmt.Sprintf("repos/%s/actions/runs/%d", repo, runID),
		"-X", "GET",
	).Output()
	if err != nil {
		return ghRun{}, ghError(err)
	}
	var run ghRun
	if err := json.Unmarshal(out, &run); err != nil {
		return ghRun{}, fmt.Errorf("decode run %d: %w", runID, err)
	}
	return run, nil
}

func branchOrAny(b string) string {
	if b == "" {
		return "any-branch"
	}
	return b
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// checksumEntry is one (slug, goos, goarch) result. Source is the
// catalog DownloadURL the artefact was fetched from; it's informational
// provenance and not part of the trust anchor.
type checksumEntry struct {
	Slug    string `json:"slug"`
	Version string `json:"version,omitempty"`
	GOOS    string `json:"goos"`
	GOARCH  string `json:"goarch"`
	SHA256  string `json:"sha256"`
	Source  string `json:"source,omitempty"`
	Host    string `json:"host,omitempty"`
	// Conflict carries the disagreeing SHAs when multiple runners
	// observed the same (slug, target) tuple but reported different
	// hashes. The primary SHA256 field carries the first seen value
	// to keep paste-ready outputs deterministic; the maintainer must
	// resolve the conflict before pinning.
	Conflict []string `json:"conflict,omitempty"`
}

// aggregateChecksums collapses the per-host audit rows into one entry
// per (slug, goos, goarch) tuple. Rows with no SHA (user-managed tools
// or missing entries) are skipped. When two runners disagree on the
// SHA for the same tuple, the conflict is recorded — content-addressed
// artefacts should never differ, so a mismatch is a supply-chain signal.
func aggregateChecksums(rows []doctorAuditRow) []checksumEntry {
	type key struct{ slug, goos, goarch string }
	idx := map[key]*checksumEntry{}
	seen := map[key]map[string]bool{}
	for _, r := range rows {
		if r.SHA256 == "" {
			continue
		}
		k := key{slug: r.Slug, goos: r.GOOS, goarch: r.GOARCH}
		if _, ok := idx[k]; !ok {
			idx[k] = &checksumEntry{
				Slug:    r.Slug,
				Version: r.Version,
				GOOS:    r.GOOS,
				GOARCH:  r.GOARCH,
				SHA256:  r.SHA256,
				Source:  r.Source,
				Host:    r.Host,
			}
			seen[k] = map[string]bool{r.SHA256: true}
			continue
		}
		seen[k][r.SHA256] = true
	}
	out := make([]checksumEntry, 0, len(idx))
	for k, e := range idx {
		if len(seen[k]) > 1 {
			shas := make([]string, 0, len(seen[k]))
			for s := range seen[k] {
				shas = append(shas, s)
			}
			sort.Strings(shas)
			e.Conflict = shas
		}
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Slug != out[j].Slug {
			return out[i].Slug < out[j].Slug
		}
		if out[i].GOOS != out[j].GOOS {
			return out[i].GOOS < out[j].GOOS
		}
		return out[i].GOARCH < out[j].GOARCH
	})
	return out
}

func renderChecksumsTable(w io.Writer, entries []checksumEntry) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SLUG\tVERSION\tTARGET\tSHA256\tCONFLICT")
	for _, e := range entries {
		ver := e.Version
		if ver == "" {
			ver = "-"
		}
		conflict := ""
		if len(e.Conflict) > 0 {
			conflict = "!!  " + strings.Join(shortSHAs(e.Conflict), " vs ")
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			e.Slug, ver, product.Tuple(e.GOOS, e.GOARCH), e.SHA256, conflict)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	for _, e := range entries {
		if len(e.Conflict) > 0 {
			fmt.Fprintf(w, "\nCONFLICT %s (%s): %d distinct hashes — investigate before pinning\n",
				e.Slug, product.Tuple(e.GOOS, e.GOARCH), len(e.Conflict))
		}
	}
	return nil
}

func renderChecksumsJSON(w io.Writer, entries []checksumEntry) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(entries)
}

// renderChecksumsGo groups by slug and prints a paste-ready
// `KnownChecksums: []string{...}` literal per tool. Slug → var name
// resolution uses an explicit override table (toolchainVarOverrides)
// plus title-case fallback. The maintainer still owns the final paste
// and MUST verify each hash matches what upstream publishes before
// committing the catalog edit.
func renderChecksumsGo(w io.Writer, entries []checksumEntry) error {
	type group struct {
		entries     []checksumEntry
		anyConflict bool
	}
	idx := map[string]*group{}
	var order []string
	for _, e := range entries {
		if _, ok := idx[e.Slug]; !ok {
			idx[e.Slug] = &group{}
			order = append(order, e.Slug)
		}
		idx[e.Slug].entries = append(idx[e.Slug].entries, e)
		if len(e.Conflict) > 0 {
			idx[e.Slug].anyConflict = true
		}
	}
	for _, slug := range order {
		g := idx[slug]
		varName := toolchainVarName(slug)
		fmt.Fprintf(w, "// %s — %d artefact(s)\n", slug, len(g.entries))
		if g.anyConflict {
			fmt.Fprintln(w, "// CONFLICT: at least one target reported multiple hashes — resolve before pinning.")
		}
		fmt.Fprintf(w, "%s.KnownChecksums = []string{\n", varName)
		for _, e := range g.entries {
			line := fmt.Sprintf("\t%q, // %s", e.SHA256, product.Tuple(e.GOOS, e.GOARCH))
			if len(e.Conflict) > 0 {
				line += " // CONFLICT: " + strings.Join(shortSHAs(e.Conflict), " vs ")
			}
			fmt.Fprintln(w, line)
		}
		fmt.Fprintln(w, "}")
		fmt.Fprintln(w)
	}
	return nil
}

// toolchainVarName maps a slug to the conventional Toolchain<Name> var
// in product/matrix.go. Most slugs follow simple title-case; the
// overrides handle catalog-specific acronym capitalisation (LLVM, ADB,
// AAPT2, UPX, VPK, LDD) and concatenated multi-word names (ApkSigner,
// ApkTool, BundleTool, LibGodot, LibGodotEditor).
func toolchainVarName(slug string) string {
	if name, ok := toolchainVarOverrides[slug]; ok {
		return "Toolchain" + name
	}
	parts := strings.FieldsFunc(slug, func(r rune) bool {
		return r == '.' || r == '-' || r == '_'
	})
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return "Toolchain" + strings.Join(parts, "")
}

// toolchainVarOverrides is the slug → Go identifier map for tools
// whose var name doesn't match a naive title-case of the slug. Keep
// in sync with product/matrix.go when adding a new catalog entry that
// breaks the default rule.
var toolchainVarOverrides = map[string]string{
	"llvm":            "LLVM",
	"adb":             "ADB",
	"aapt2":           "AAPT2",
	"upx":             "UPX",
	"vpk":             "VPK",
	"ldd":             "LDD",
	"apksigner":       "ApkSigner",
	"apktool":         "ApkTool",
	"bundletool":      "BundleTool",
	"libgodot":        "LibGodot",
	"libgodot-editor": "LibGodotEditor",
}

func shortSHAs(shas []string) []string {
	out := make([]string, len(shas))
	for i, s := range shas {
		out[i] = shortSHA(s)
	}
	return out
}
