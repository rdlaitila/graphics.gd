package ci

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// pushedShot is one row from a pushShotsToBranch round-trip. When
// Error is non-empty the upload failed for this shot (or for the
// whole batch) — URL is empty and the renderer prints the message
// in the image cell instead of dropping the row, so the summary
// stays honest about what was supposed to be there.
type pushedShot struct {
	Label string
	Path  string
	URL   string
	Error string
}

// pushShotsToBranch publishes every shot to the `screenshots` branch
// at <today>/<runID>/<artefact>.png using only the GitHub Git Data
// REST API (no checkout, no submodule). The branch is created as an
// orphan on first use so the screenshot history doesn't grow off the
// main project commit graph.
//
// Layout is intentionally date-major so a maintainer who wants to
// prune old runs can delete a single dated directory:
//
//	screenshots/
//	├── README.md           # overview + cleavage instructions
//	├── 2026-06-23/
//	│   ├── 28074123456/
//	│   │   ├── manifest.json
//	│   │   ├── linux-amd64+gdextension-ubuntu-latest-ubuntu-latest.png
//	│   │   └── ...
//	│   └── ...
//	└── 2026-06-24/
//	    └── ...
//
// repo is "<owner>/<name>"; branch is the orphan branch name (default
// "screenshots"); runID identifies the workflow run (typically
// $GITHUB_RUN_ID). Returns the per-shot rendered URLs or an error.
func pushShotsToBranch(repo, branch string, runID int64, rows []shotRow) ([]pushedShot, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	owner, name, ok := strings.Cut(repo, "/")
	if !ok {
		return nil, fmt.Errorf("shots: --repo %q is not <owner>/<name>", repo)
	}
	// 1. Resolve the parent commit. Create the orphan branch with a
	//    seed commit (README + .gitkeep) when the ref doesn't exist
	//    yet, so the rest of the upload path can assume a parent.
	parent, err := resolveOrCreateOrphanBranch(repo, branch)
	if err != nil {
		return nil, fmt.Errorf("shots: ensure branch %s: %w", branch, err)
	}
	day := time.Now().UTC().Format("2006-01-02")
	pathFor := func(r shotRow) string {
		// Map the human label into a filesystem-safe slug. The
		// caption shape is `<target>+<link> (b: <build> p: <play>)`;
		// replace separators that confuse paths or URLs (slash,
		// paren, colon, space) with dashes and then collapse runs.
		s := r.Label
		for _, ch := range []string{"/", "(", ")", ":", " ", "+"} {
			s = strings.ReplaceAll(s, ch, "-")
		}
		for strings.Contains(s, "--") {
			s = strings.ReplaceAll(s, "--", "-")
		}
		s = strings.Trim(s, "-")
		return fmt.Sprintf("%s/%d/%s.png", day, runID, s)
	}
	// 2. Push every PNG as a blob.
	type entry struct {
		Path string `json:"path"`
		Mode string `json:"mode"`
		Type string `json:"type"`
		SHA  string `json:"sha"`
	}
	var entries []entry
	out := make([]pushedShot, 0, len(rows))
	for _, r := range rows {
		sha, err := uploadBlob(repo, r.PNG)
		if err != nil {
			return nil, fmt.Errorf("shots: upload blob for %s: %w", r.Label, err)
		}
		p := pathFor(r)
		entries = append(entries, entry{Path: p, Mode: "100644", Type: "blob", SHA: sha})
		out = append(out, pushedShot{
			Label: r.Label,
			Path:  p,
			URL:   rawURL(owner, name, branch, p),
		})
	}
	// 3. Manifest + README maintenance. Both keep the branch readable
	//    when a human stumbles onto it.
	manifest, err := json.MarshalIndent(buildManifest(day, runID, out), "", "  ")
	if err != nil {
		return nil, err
	}
	manifestSHA, err := uploadBlob(repo, manifest)
	if err != nil {
		return nil, fmt.Errorf("shots: upload manifest: %w", err)
	}
	entries = append(entries, entry{
		Path: fmt.Sprintf("%s/%d/manifest.json", day, runID),
		Mode: "100644", Type: "blob", SHA: manifestSHA,
	})
	readmeSHA, err := uploadBlob(repo, []byte(screenshotsReadme))
	if err != nil {
		return nil, fmt.Errorf("shots: upload README: %w", err)
	}
	entries = append(entries, entry{
		Path: "README.md", Mode: "100644", Type: "blob", SHA: readmeSHA,
	})
	// 4. Tree + commit + ref-update. base_tree lets the API merge
	//    these entries with the existing branch state so prior runs
	//    aren't clobbered.
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	treeBody := map[string]any{"tree": entries}
	if parent.tree != "" {
		treeBody["base_tree"] = parent.tree
	}
	treeSHA, err := createTree(repo, treeBody)
	if err != nil {
		return nil, fmt.Errorf("shots: create tree: %w", err)
	}
	msg := fmt.Sprintf("run %d: %d screenshot(s) on %s", runID, len(rows), day)
	parents := []string{}
	if parent.commit != "" {
		parents = []string{parent.commit}
	}
	commitSHA, err := createCommit(repo, msg, treeSHA, parents)
	if err != nil {
		return nil, fmt.Errorf("shots: create commit: %w", err)
	}
	if err := updateRef(repo, branch, commitSHA); err != nil {
		return nil, fmt.Errorf("shots: update ref: %w", err)
	}
	return out, nil
}

// branchParent captures the state needed to commit on top of a
// branch: the commit it currently points at and the tree that commit
// references. Both are empty when the branch was just created with
// the orphan seed and the seed is being replaced.
type branchParent struct {
	commit string
	tree   string
}

// resolveOrCreateOrphanBranch returns the (commit, tree) the next
// commit should build on top of. When the branch doesn't exist it is
// created as a true orphan (no parent of the main project tree) with
// a single seed commit holding just the README and a .gitkeep so the
// upload path always has a parent to attach to. Subsequent runs reuse
// the branch.
func resolveOrCreateOrphanBranch(repo, branch string) (branchParent, error) {
	ref, err := getRef(repo, branch)
	if err == nil {
		commit, err := getCommit(repo, ref)
		if err != nil {
			return branchParent{}, err
		}
		return branchParent{commit: ref, tree: commit.Tree.SHA}, nil
	}
	if !errors.Is(err, errNotFound) {
		return branchParent{}, err
	}
	// Bootstrap: seed README + .gitkeep, point a fresh ref at the
	// commit. updateRef can create as well as update via the
	// PATCH-vs-POST switch in its body.
	readmeSHA, err := uploadBlob(repo, []byte(screenshotsReadme))
	if err != nil {
		return branchParent{}, err
	}
	gitkeepSHA, err := uploadBlob(repo, []byte{})
	if err != nil {
		return branchParent{}, err
	}
	treeSHA, err := createTree(repo, map[string]any{
		"tree": []map[string]any{
			{"path": "README.md", "mode": "100644", "type": "blob", "sha": readmeSHA},
			{"path": ".gitkeep", "mode": "100644", "type": "blob", "sha": gitkeepSHA},
		},
	})
	if err != nil {
		return branchParent{}, err
	}
	commitSHA, err := createCommit(repo, "initial screenshots branch", treeSHA, nil)
	if err != nil {
		return branchParent{}, err
	}
	if err := createRef(repo, branch, commitSHA); err != nil {
		return branchParent{}, err
	}
	return branchParent{commit: commitSHA, tree: treeSHA}, nil
}

// uploadBlob posts the bytes as a base64-encoded blob and returns the
// resulting SHA. base64 is required by the API; for already-text
// payloads (manifest.json, README.md) it's still cheaper than
// negotiating utf-8 escapes against the JSON body.
func uploadBlob(repo string, body []byte) (string, error) {
	in := map[string]string{
		"content":  base64.StdEncoding.EncodeToString(body),
		"encoding": "base64",
	}
	var resp struct {
		SHA string `json:"sha"`
	}
	if err := ghPost(repo, "git/blobs", in, &resp); err != nil {
		return "", err
	}
	return resp.SHA, nil
}

// createTree posts a tree body (caller-built {"tree": [...], optional
// "base_tree": "..."}) and returns the resulting tree SHA.
func createTree(repo string, body map[string]any) (string, error) {
	var resp struct {
		SHA string `json:"sha"`
	}
	if err := ghPost(repo, "git/trees", body, &resp); err != nil {
		return "", err
	}
	return resp.SHA, nil
}

// createCommit posts a commit with the given message + tree + parents
// and returns the resulting commit SHA.
func createCommit(repo, message, tree string, parents []string) (string, error) {
	body := map[string]any{"message": message, "tree": tree}
	if len(parents) > 0 {
		body["parents"] = parents
	}
	var resp struct {
		SHA string `json:"sha"`
	}
	if err := ghPost(repo, "git/commits", body, &resp); err != nil {
		return "", err
	}
	return resp.SHA, nil
}

// getCommit returns the tree SHA the named commit points at.
type ghCommit struct {
	Tree struct {
		SHA string `json:"sha"`
	} `json:"tree"`
}

func getCommit(repo, commitSHA string) (ghCommit, error) {
	var resp ghCommit
	if err := ghGet(repo, "git/commits/"+commitSHA, &resp); err != nil {
		return ghCommit{}, err
	}
	return resp, nil
}

// getRef returns the commit SHA the branch ref currently points at,
// or errNotFound when the ref doesn't exist on the remote yet.
func getRef(repo, branch string) (string, error) {
	var resp struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := ghGet(repo, "git/refs/heads/"+branch, &resp); err != nil {
		return "", err
	}
	return resp.Object.SHA, nil
}

// createRef POSTs a brand-new ref pointing at commitSHA.
func createRef(repo, branch, commitSHA string) error {
	body := map[string]any{
		"ref": "refs/heads/" + branch,
		"sha": commitSHA,
	}
	return ghPost(repo, "git/refs", body, nil)
}

// updateRef fast-forwards (or force-moves) the branch tip to
// commitSHA. force=true so re-pushes from the same run are tolerated
// when the workflow re-renders the summary step (e.g. after a retry).
func updateRef(repo, branch, commitSHA string) error {
	body := map[string]any{"sha": commitSHA, "force": true}
	return ghPatch(repo, "git/refs/heads/"+branch, body, nil)
}

// rawURL returns the raw.githubusercontent.com URL for path under
// owner/repo at the given branch. raw.githubusercontent.com is the
// host GitHub's step-summary sanitiser allow-lists for <img src>.
func rawURL(owner, repo, branch, path string) string {
	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/refs/heads/%s/%s",
		owner, repo, branch, path)
}

// buildManifest stamps a small JSON record next to each run's PNGs
// so external tooling can map artefacts back to play-cell labels
// without re-parsing the filename.
type manifestEntry struct {
	Label string `json:"label"`
	Path  string `json:"path"`
	URL   string `json:"url"`
}
type manifest struct {
	Day   string          `json:"day"`
	RunID int64           `json:"run_id"`
	Shots []manifestEntry `json:"shots"`
}

func buildManifest(day string, runID int64, rows []pushedShot) manifest {
	m := manifest{Day: day, RunID: runID, Shots: make([]manifestEntry, 0, len(rows))}
	for _, r := range rows {
		m.Shots = append(m.Shots, manifestEntry{Label: r.Label, Path: r.Path, URL: r.URL})
	}
	return m
}

// screenshotsReadme is the branch's top-level README. It documents
// what the branch is for and how to prune it, so a maintainer who
// stumbles onto it via git log can clean up without source-diving.
const screenshotsReadme = `# screenshots

Auto-generated screenshots from every play-cell of the gdnext workflow.

Layout: ` + "`<YYYY-MM-DD>/<run-id>/<cell-label>.png`" + ` plus a
` + "`manifest.json`" + ` per run with the label-to-path mapping.

This branch is orphaned from the main project tree and exists only
to host inline-renderable images for the rolling step-summary. Each
PNG is referenced from the summary via raw.githubusercontent.com.

## Pruning

The layout is date-major so old runs can be cleaved at any granularity:

` + "```bash" + `
# Drop a single run
git rm -r 2026-06-23/28074123456 && git commit -m "prune run"

# Drop a whole day
git rm -r 2026-06-23 && git commit -m "prune day"

# Drop everything older than two weeks
git ls-tree --name-only HEAD | \
  awk -v cutoff="$(date -d '14 days ago' +%Y-%m-%d)" '$1 < cutoff' | \
  xargs -r git rm -r && git commit -m "prune >14d"
` + "```" + `

## Provenance

Generated by ` + "`gdnext ci workflow-summary --shots-branch`" + ` from the
summary job of ` + "`.github/workflows/gdnext.yml`" + `.
`

// errNotFound signals a 404 from the GitHub API. Wrapped errors
// returned by gh* helpers below test against this so resolveOrCreate
// can branch on "create the orphan" vs "real failure".
var errNotFound = errors.New("github: not found")

// ghGet / ghPost / ghPatch are thin shells over `gh api` (the same
// transport every other workflow_sum_*.go file uses). gh follows the
// canonical retry policy, uses the configured token, and handles the
// User-Agent and Accept headers so the verb doesn't have to.
func ghGet(repo, path string, into any) error {
	return ghCall(repo, "GET", path, nil, into)
}
func ghPost(repo, path string, body any, into any) error {
	return ghCall(repo, "POST", path, body, into)
}
func ghPatch(repo, path string, body any, into any) error {
	return ghCall(repo, "PATCH", path, body, into)
}

func ghCall(repo, method, path string, body any, into any) error {
	args := []string{"api", "-X", method, "repos/" + repo + "/" + path}
	var stdin []byte
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		stdin = raw
		// `gh api --input -` reads JSON body from stdin.
		args = append(args, "--input", "-")
	}
	cmd := exec.Command("gh", args...)
	if len(stdin) > 0 {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			msg := strings.TrimSpace(string(ee.Stderr))
			if strings.Contains(msg, "Not Found") || strings.Contains(msg, "404") {
				return errNotFound
			}
			return fmt.Errorf("gh api %s %s: %s", method, path, msg)
		}
		return fmt.Errorf("gh api %s %s: %w", method, path, err)
	}
	if into == nil {
		return nil
	}
	return json.Unmarshal(out, into)
}
