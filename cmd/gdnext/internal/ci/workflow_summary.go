package ci

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"graphics.gd/cmd/gdnext/internal/shared"
	"graphics.gd/product"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
)

// WorkflowSummaryCommand wires `gdnext ci workflow-summary`. Runtime
// state lives on *WorkflowSummaryActions. The workflow step pipes
// stdout into $GITHUB_STEP_SUMMARY.
type WorkflowSummaryCommand struct {
	*cli.Command
	Injector do.Injector `do:""`
}

// WorkflowSummaryActions carries the runtime state.
type WorkflowSummaryActions struct{}

// --- GitHub API DTOs -------------------------------------------------
// Raw shapes returned by `gh api`. Network-only — domain code reads
// from the typed model below, not these.

type ghRun struct {
	ID           int64     `json:"id"`
	Number       int       `json:"run_number"`
	Status       string    `json:"status"`
	Conclusion   string    `json:"conclusion"`
	HeadBranch   string    `json:"head_branch"`
	HeadSHA      string    `json:"head_sha"`
	DisplayTitle string    `json:"display_title"`
	Event        string    `json:"event"`
	CreatedAt    time.Time `json:"created_at"`
	HTMLURL      string    `json:"html_url"`
}

type ghJob struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	Conclusion  string    `json:"conclusion"`
	HTMLURL     string    `json:"html_url"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
	Steps       []ghStep  `json:"steps"`
}

type ghStep struct {
	Name       string `json:"name"`
	Number     int    `json:"number"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
}

type runWithJobs struct {
	Run  ghRun
	Jobs []ghJob
}

// --- Domain model ----------------------------------------------------
// Output of collect() and input of every renderer. Adding a new format
// (JSON, HTML, ANSI) means writing a new render*() against summary —
// the collector stays untouched.

// outcome normalises GitHub's (status, conclusion) pair into a single
// enum so renderers and stats only switch on one value.
type outcome uint8

// entry is one row's data point for one run: the outcome plus how
// long the job ran. Zero Duration means timing data is missing. URL
// links the pip back to the GHA job page.
type entry struct {
	Outcome  outcome
	Duration time.Duration
	URL      string
}

// history is one row's chronological appearances, oldest first.
// Indices align across rows so sparklines line up vertically.
type history []entry

// summary is the typed snapshot a renderer consumes. Markdown is the
// only renderer today; JSON / HTML / ANSI slot in next to it without
// touching the collector or the GitHub API layer.
type summary struct {
	Branch       string
	Window       []runMeta
	Checks       []checkRow
	Builds       []buildRow
	Plays        []playRow
	Shots        []pushedShot
	Toolchains   []toolchainRow
	LastFailures []failureRow
}

// summaryCounts is the at-a-glance rollup of summary numbers a
// renderer can show without re-walking the typed model. Computed by
// summary.counts().
type summaryCounts struct {
	LatestFailures      int
	DecisiveJobs        int
	WindowPasses        int
	WindowFailures      int
	Hosts               int
	BuildCells          int
	PlayCells           int
	PlatformsCatalogued int
}

// failureRow definition lives in workflow_sum_failures.go.

type runMeta struct {
	Number     int
	CreatedAt  time.Time
	HeadBranch string
	HeadSHA    string
	Title      string
	Event      string
	Conclusion string
	URL        string
	Pass       int
	Fail       int
}

// checkRow / buildRow / playRow / failureRow definitions and their
// collectors live in workflow_sum_*.go. summary stays here because the
// orchestrator does, and renderMarkdown dispatches into each section.

const (
	outcomeMissing outcome = iota
	outcomeSuccess
	outcomeFailure
	outcomeSkipped
	outcomeRunning
)

const (
	iconPass    = "🟩"
	iconFail    = "🟥"
	iconSkip    = "⬜"
	iconRunning = "🟨"
	iconMissing = "□ "
)

// ghaTimestamp matches the ISO timestamp GHA prepends to every log
// line; stripping it claws back ~30 columns.
var ghaTimestamp = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T[\d:.]+Z\s`)

// NewWorkflowSummaryCommand constructs the workflow-summary subcommand.
func NewWorkflowSummaryCommand(di do.Injector) (*WorkflowSummaryCommand, error) {
	t := do.MustInvokeStruct[*WorkflowSummaryCommand](di)
	t.Command = &cli.Command{
		Name:  "workflow-summary",
		Usage: "render a rolling markdown summary of recent runs",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "repo", Required: true, Usage: "owner/repo"},
			&cli.StringFlag{Name: "workflow", Required: true, Usage: "workflow filename, e.g. gdnext.yml"},
			&cli.IntFlag{Name: "runs", Value: 14, Usage: "max runs to include"},
			&cli.IntFlag{Name: "log-tail", Value: 60, Usage: "lines of log to tail per failed job in the latest run"},
			&cli.StringFlag{Name: "branch", Usage: "limit to a single branch (e.g. gdnext-cli)"},
			&cli.StringFlag{Name: "shots", Usage: "directory containing screenshot-<cell>/play-screenshot.png artefacts to inline as a grid"},
			&cli.StringFlag{Name: "shots-branch", Usage: "push collected screenshots to this orphan branch and embed raw.githubusercontent.com URLs (creates the branch if missing); falls back to inline data: URIs when unset"},
			&cli.IntFlag{Name: "shots-run-id", Usage: "workflow run id used in the on-branch path (default: $GITHUB_RUN_ID)"},
		},
		Action: shared.BindAction(t.Injector, (*WorkflowSummaryActions).action),
	}
	return t, nil
}

// NewWorkflowSummaryActions resolves the runtime state.
func NewWorkflowSummaryActions(di do.Injector) (*WorkflowSummaryActions, error) {
	return do.InvokeStruct[*WorkflowSummaryActions](di)
}

func (t *WorkflowSummaryActions) action(_ context.Context, cmd *cli.Command) error {
	repo := cmd.String("repo")
	workflow := cmd.String("workflow")
	n := int(cmd.Int("runs"))
	tail := int(cmd.Int("log-tail"))
	branch := cmd.String("branch")
	shotsDir := cmd.String("shots")
	shotsBranch := cmd.String("shots-branch")
	shotsRunID := int64(cmd.Int("shots-run-id"))
	runs, err := fetchRuns(repo, workflow, n, branch)
	if err != nil {
		return fmt.Errorf("fetch runs: %w", err)
	}
	if len(runs) == 0 {
		fmt.Println("_No workflow runs found._")
		return nil
	}
	var window []runWithJobs
	for _, r := range runs {
		jobs, err := fetchJobs(repo, r.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "skip run %d: %v\n", r.ID, err)
			continue
		}
		window = append(window, runWithJobs{Run: r, Jobs: jobs})
	}
	s := collect(window, branch)
	s.LastFailures = collectFailures(repo, window, tail)
	if latest, ok := latestRun(window); ok {
		var prior map[string]string
		// Pick the most-recent prior run on the same branch for the
		// supply-chain diff. Falls back to nil (no Changed flags) when
		// the window only carries one run.
		for _, r := range window {
			if r.Run.ID == latest.Run.ID {
				continue
			}
			if r.Run.CreatedAt.Before(latest.Run.CreatedAt) {
				prior = priorToolchainSHAs(repo, r.Run.ID)
				break
			}
		}
		s.Toolchains = collectToolchains(repo, latest.Run.ID, prior)
	}
	s.Shots = resolveShots(repo, shotsDir, shotsBranch, shotsRunID)
	return renderMarkdown(os.Stdout, s)
}

// resolveShots picks the shot-rendering strategy based on the
// summary flags. When --shots-branch is set, every PNG is pushed to
// that orphan branch under <YYYY-MM-DD>/<run-id>/<label>.png and the
// returned rows carry raw.githubusercontent.com URLs (the only
// inline-image host GitHub's step-summary sanitiser allow-lists).
// Otherwise each row carries a `data:` URI so the verb stays usable
// offline. When a push to the branch fails the rows are returned
// with their Error field populated and an empty URL — the renderer
// surfaces the message inline so failures stay visible rather than
// silently flipping back to an unrenderable data URI.
func resolveShots(repo, shotsDir, branch string, runIDFlag int64) []pushedShot {
	raw := collectShots(shotsDir)
	if len(raw) == 0 {
		return nil
	}
	if branch == "" {
		return dataURIShots(raw)
	}
	runID := runIDFlag
	if runID == 0 {
		if v := os.Getenv("GITHUB_RUN_ID"); v != "" {
			fmt.Sscanf(v, "%d", &runID)
		}
	}
	if runID == 0 {
		runID = time.Now().UTC().Unix()
	}
	pushed, err := pushShotsToBranch(repo, branch, runID, raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "shots: push to %s failed: %v\n", branch, err)
		return errorShots(raw, fmt.Sprintf("push to %s failed: %v", branch, err))
	}
	return pushed
}

// --- GitHub API DTOs -------------------------------------------------
// Raw shapes returned by `gh api`. Network-only — domain code reads
// from the typed model below, not these.

func fetchRuns(repo, workflow string, n int, branch string) ([]ghRun, error) {
	args := []string{
		"api",
		fmt.Sprintf("repos/%s/actions/workflows/%s/runs", repo, workflow),
		"-X", "GET",
		"-F", fmt.Sprintf("per_page=%d", n),
	}
	if branch != "" {
		args = append(args, "-f", "branch="+branch)
	}
	out, err := exec.Command("gh", args...).Output()
	if err != nil {
		return nil, ghError(err)
	}
	var resp struct {
		Runs []ghRun `json:"workflow_runs"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, err
	}
	if len(resp.Runs) > n {
		resp.Runs = resp.Runs[:n]
	}
	return resp.Runs, nil
}

func fetchJobs(repo string, runID int64) ([]ghJob, error) {
	out, err := exec.Command("gh", "api",
		fmt.Sprintf("repos/%s/actions/runs/%d/jobs", repo, runID),
		"-X", "GET",
		"-F", "per_page=100",
	).Output()
	if err != nil {
		return nil, ghError(err)
	}
	var resp struct {
		Jobs []ghJob `json:"jobs"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, err
	}
	return resp.Jobs, nil
}

// fetchJobLog returns the raw text log for one GHA job. The API
// redirects to a signed URL; `gh api` follows it transparently.
func fetchJobLog(repo string, jobID int64) (string, error) {
	out, err := exec.Command("gh", "api",
		fmt.Sprintf("repos/%s/actions/jobs/%d/logs", repo, jobID),
		"-X", "GET",
	).Output()
	if err != nil {
		return "", ghError(err)
	}
	return string(out), nil
}

func ghError(err error) error {
	if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(ee.Stderr)))
	}
	return err
}

// tailLines returns the last n content lines of text with GHA
// timestamps and group markers stripped.
func tailLines(text string, n int) []string {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return nil
	}
	var lines []string
	for _, l := range strings.Split(text, "\n") {
		stripped := stripTimestamp(l)
		if strings.HasPrefix(stripped, "##[group]") || stripped == "##[endgroup]" {
			continue
		}
		lines = append(lines, stripped)
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

func stripTimestamp(s string) string { return ghaTimestamp.ReplaceAllString(s, "") }

// --- Domain model ----------------------------------------------------
// outcomeFromJob, entryFromJob and the methods on history live here;
// the type declarations sit at the top of the file.

func outcomeFromJob(j ghJob) outcome {
	switch j.Conclusion {
	case "success":
		// continue-on-error masks step failures as a job 'success';
		// surface them as red anyway so allow-fail rows are visible.
		for _, s := range j.Steps {
			if s.Conclusion == "failure" {
				return outcomeFailure
			}
		}
		return outcomeSuccess
	case "failure":
		return outcomeFailure
	case "":
		// fall through to status check
	default:
		return outcomeSkipped
	}
	switch j.Status {
	case "queued", "pending", "in_progress":
		return outcomeRunning
	}
	return outcomeSkipped
}

// entry is one row's data point for one run: the outcome plus how
// long the job ran. Zero Duration means timing data is missing. URL
// links the pip back to the GHA job page.
func entryFromJob(j ghJob) entry {
	var d time.Duration
	if !j.StartedAt.IsZero() && !j.CompletedAt.IsZero() && j.CompletedAt.After(j.StartedAt) {
		d = j.CompletedAt.Sub(j.StartedAt)
	}
	return entry{Outcome: outcomeFromJob(j), Duration: d, URL: j.HTMLURL}
}

func (t history) pass() int { return t.count(outcomeSuccess) }
func (t history) fail() int { return t.count(outcomeFailure) }

// ran reports the number of window slots where the row appeared in
// any form (anything but missing).
func (t history) ran() int {
	n := 0
	for _, e := range t {
		if e.Outcome != outcomeMissing {
			n++
		}
	}
	return n
}

func (t history) count(o outcome) int {
	n := 0
	for _, e := range t {
		if e.Outcome == o {
			n++
		}
	}
	return n
}

// passPercent returns the integer pass rate over decisive outcomes;
// ok=false when no decisive runs are present so callers can render "—".
func (t history) passPercent() (pct int, ok bool) {
	p, f := t.pass(), t.fail()
	if p+f == 0 {
		return 0, false
	}
	return int(float64(p) / float64(p+f) * 100), true
}

// passPercentChange compares the row's cumulative pass% across all
// decisive appearances against the cumulative pass% as of one run
// ago, returning the signed delta in percentage points. ok=false
// when fewer than three decisive appearances exist — with two, the
// prior point is a single run and any change is noise.
func (t history) passPercentChange() (delta int, ok bool) {
	seen := make(history, 0, len(t))
	for _, e := range t {
		if e.Outcome != outcomeMissing {
			seen = append(seen, e)
		}
	}
	if len(seen) < 3 {
		return 0, false
	}
	prior, priorOk := seen[:len(seen)-1].passPercent()
	current, currentOk := seen.passPercent()
	if !priorOk || !currentOk {
		return 0, false
	}
	return current - prior, true
}

// durations returns the row's nonzero durations sorted ascending,
// suitable for best/median/worst aggregation.
func (t history) durations() []time.Duration {
	outs := make([]time.Duration, 0, len(t))
	for _, e := range t {
		if e.Duration > 0 {
			outs = append(outs, e.Duration)
		}
	}
	sort.Slice(outs, func(a, b int) bool { return outs[a] < outs[b] })
	return outs
}

// best, median, worst return the row's runtime quartile picks; ok=false
// when no timing data is present.
func (t history) best() (time.Duration, bool) {
	d := t.durations()
	if len(d) == 0 {
		return 0, false
	}
	return d[0], true
}

func (t history) worst() (time.Duration, bool) {
	d := t.durations()
	if len(d) == 0 {
		return 0, false
	}
	return d[len(d)-1], true
}

func (t history) median() (time.Duration, bool) {
	d := t.durations()
	if len(d) == 0 {
		return 0, false
	}
	if len(d)%2 == 1 {
		return d[len(d)/2], true
	}
	return (d[len(d)/2-1] + d[len(d)/2]) / 2, true
}

// --- Summary collector -----------------------------------------------
// gh API → summary. Sorts everything into deterministic order so
// renderers can be straight-line printers.

func (t summary) counts() summaryCounts {
	c := summaryCounts{
		LatestFailures:      len(t.LastFailures),
		PlatformsCatalogued: len(product.PlatformMatrix),
		BuildCells:          len(t.Builds),
		PlayCells:           len(t.Plays),
	}
	hosts := map[string]struct{}{}
	tally := func(h history) {
		c.WindowPasses += h.pass()
		c.WindowFailures += h.fail()
	}
	for _, r := range t.Checks {
		hosts[r.Host] = struct{}{}
		tally(r.History)
	}
	for _, r := range t.Builds {
		hosts[r.Host] = struct{}{}
		tally(r.History)
	}
	for _, r := range t.Plays {
		tally(r.History)
	}
	c.Hosts = len(hosts)
	c.DecisiveJobs = c.WindowPasses + c.WindowFailures
	return c
}

func collect(window []runWithJobs, branch string) summary {
	asc := append([]runWithJobs(nil), window...)
	sort.Slice(asc, func(i, j int) bool {
		return asc[i].Run.CreatedAt.Before(asc[j].Run.CreatedAt)
	})
	s := summary{Branch: branch}
	for _, r := range asc {
		c := r.Run.Conclusion
		if c == "" {
			c = r.Run.Status
		}
		var pass, fail int
		for _, j := range r.Jobs {
			switch j.Conclusion {
			case "success":
				pass++
			case "failure":
				fail++
			}
		}
		s.Window = append(s.Window, runMeta{
			Number:     r.Run.Number,
			CreatedAt:  r.Run.CreatedAt,
			HeadBranch: r.Run.HeadBranch,
			HeadSHA:    r.Run.HeadSHA,
			Title:      r.Run.DisplayTitle,
			Event:      r.Run.Event,
			Conclusion: c,
			URL:        r.Run.HTMLURL,
			Pass:       pass,
			Fail:       fail,
		})
	}
	s.Checks = collectChecks(asc)
	s.Builds = collectBuilds(asc)
	s.Plays = collectPlays(asc)
	return s
}

// parseLinkExp pulls (link, allow-fail) out of the suffix axes after
// (host, example, target). Pinned build job names render the suffix
// as `(link, allow-fail)`; pre-rename runs in the window include
// extra trailing axes — content detection (LinkMode catalog for link,
// bool literal for allow-fail) stays robust to both.
func parseLinkExp(tail []string) (link string, allowFail bool) {
	if len(tail) > 0 && isLinkMode(tail[0]) {
		link = tail[0]
	}
	for _, t := range tail {
		if t == "true" {
			allowFail = true
			return
		}
		if t == "false" {
			return
		}
	}
	return
}

func isLinkMode(s string) bool {
	for _, m := range product.LinkModeMatrix {
		if s == m {
			return true
		}
	}
	return false
}

// latestRun returns the newest run in window by created_at, ok=false
// when the window is empty.
func latestRun(window []runWithJobs) (runWithJobs, bool) {
	if len(window) == 0 {
		return runWithJobs{}, false
	}
	best := window[0]
	for _, r := range window[1:] {
		if r.Run.CreatedAt.After(best.Run.CreatedAt) {
			best = r
		}
	}
	return best, true
}

// splitJobName splits "head (axis1, axis2, …)" into the leading job id
// and its matrix-axes slice. Non-matrix jobs (discover-targets, the
// summary step itself) return ok=false.
func splitJobName(name string) (string, []string, bool) {
	open := strings.LastIndex(name, " (")
	if open < 0 || !strings.HasSuffix(name, ")") {
		return name, nil, false
	}
	return name[:open], strings.Split(name[open+2:len(name)-1], ", "), true
}

// hostRank maps a GHA runner label to its position in the gha table
// (defined in matrix.go).
func hostRank(runner string) int {
	for i, g := range gha {
		if g.Runner == runner {
			return i
		}
	}
	return len(gha)
}

// targetRank maps "goos/goarch" to its position in product.PlatformMatrix
// so builds render in the same order as `gdnext platform`.
func targetRank(target string) int {
	slash := strings.IndexByte(target, '/')
	if slash < 0 {
		return len(product.PlatformMatrix)
	}
	goos, goarch := target[:slash], target[slash+1:]
	for i, p := range product.PlatformMatrix {
		if p.GOOS == goos && p.GOARCH == goarch {
			return i
		}
	}
	return len(product.PlatformMatrix)
}

func linkSubrank(link string) int {
	switch link {
	case "gdextension":
		return 0
	case "libgodot":
		return 1
	default:
		return 9
	}
}

// --- Markdown renderer -----------------------------------------------

func renderMarkdown(w io.Writer, s summary) error {
	fmt.Fprintln(w, "# gdnext")
	fmt.Fprintln(w)
	if len(s.Window) > 0 {
		fmt.Fprintf(w, "Last **%d** runs (`%s` → `%s`)",
			len(s.Window),
			s.Window[0].CreatedAt.Format("2006-01-02"),
			s.Window[len(s.Window)-1].CreatedAt.Format("2006-01-02"))
		if s.Branch != "" {
			fmt.Fprintf(w, " on `%s`", s.Branch)
		}
		fmt.Fprintln(w, ".")
		fmt.Fprintln(w)
	}
	renderCountsMarkdown(w, s)
	renderTOCMarkdown(w, s)
	renderChecksMarkdown(w, s.Checks)
	renderBuildsMarkdown(w, s.Builds)
	renderPlaysMarkdown(w, s.Plays)
	renderShotsMarkdown(w, s.Shots)
	renderFailuresMarkdown(w, s.LastFailures)
	renderCommitsMarkdown(w, s.Window)
	renderToolchainsMarkdown(w, s.Toolchains)
	return nil
}

// renderCountsMarkdown prints a single-line word-cloud of the
// quantities that read at-a-glance: jobs, hosts, build cells,
// catalogued platforms, and the failure tallies.
func renderCountsMarkdown(w io.Writer, s summary) {
	c := s.counts()
	parts := []string{
		fmt.Sprintf("**%d** failures in latest run", c.LatestFailures),
		fmt.Sprintf("**%d** ✓", c.WindowPasses),
		fmt.Sprintf("**%d** ✗", c.WindowFailures),
		fmt.Sprintf("**%d** decisive jobs", c.DecisiveJobs),
		fmt.Sprintf("**%d** hosts", c.Hosts),
		fmt.Sprintf("**%d** build cells", c.BuildCells),
		fmt.Sprintf("**%d** play cells", c.PlayCells),
		fmt.Sprintf("**%d** platforms in catalog", c.PlatformsCatalogued),
	}
	fmt.Fprintln(w, strings.Join(parts, " · "))
	fmt.Fprintln(w)
}

func renderTOCMarkdown(w io.Writer, s summary) {
	fmt.Fprintln(w, "**Contents:** [Checks](#checks) · [Builds](#builds) · [Plays](#plays) · [Latest run failures](#latest-run-failures) · [Commits](#commits) · [Toolchains](#toolchains)")
	fmt.Fprintln(w)
}

// writeSectionHeader emits an HTML-anchored heading so the TOC link
// (`#anchor`) stays stable when we append a pass-% suffix to the
// displayed text. Markdown `## foo (84%)` would slug to `#foo-84`
// and break the contents links.
func writeSectionHeader(w io.Writer, anchor, title string, hs []history) {
	suffix := ""
	if pct, pass, fail, ok := overallPass(hs); ok {
		suffix = fmt.Sprintf(" — %d%% (%d/%d)", pct, pass, pass+fail)
	}
	fmt.Fprintf(w, "<h2 id=\"%s\">%s%s</h2>\n\n", anchor, htmlEscape(title), suffix)
}

// overallPass aggregates pass/fail across many histories and returns
// the integer pass rate plus the underlying counts. ok=false when
// no decisive outcomes are present.
func overallPass(hs []history) (pct, pass, fail int, ok bool) {
	for _, h := range hs {
		pass += h.pass()
		fail += h.fail()
	}
	if pass+fail == 0 {
		return 0, 0, 0, false
	}
	return int(float64(pass) / float64(pass+fail) * 100), pass, fail, true
}

func writeMarkdownHeader(w io.Writer, leading ...string) {
	headers := append([]string{}, leading...)
	headers = append(headers, "Runs", "✓", "✗", "Pass %", "Change", "RT (b/m/w)", "History")
	fmt.Fprintln(w, "| "+strings.Join(headers, " | ")+" |")
	sep := make([]string, 0, len(headers))
	for i := range headers {
		if i < len(leading) || i == len(headers)-1 {
			sep = append(sep, "---")
		} else {
			sep = append(sep, "---:")
		}
	}
	fmt.Fprintln(w, "| "+strings.Join(sep, " | ")+" |")
}

func writeMarkdownRow(w io.Writer, leading []string, h history) {
	pct := "—"
	if p, ok := h.passPercent(); ok {
		pct = fmt.Sprintf("%d%%", p)
	}
	change := "—"
	if d, ok := h.passPercentChange(); ok {
		switch {
		case d > 0:
			change = fmt.Sprintf("▲ +%d%%", d)
		case d < 0:
			change = fmt.Sprintf("▼ %d%%", d)
		default:
			change = "0%"
		}
	}
	cells := make([]string, 0, len(leading)+7)
	for _, c := range leading {
		cells = append(cells, mdEscape(c))
	}
	cells = append(cells,
		fmt.Sprintf("%d", h.ran()),
		fmt.Sprintf("%d", h.pass()),
		fmt.Sprintf("%d", h.fail()),
		pct,
		change,
		runtimeCell(h),
		sparklineMarkdown(h),
	)
	fmt.Fprintln(w, "| "+strings.Join(cells, " | ")+" |")
}

// runtimeCell formats best/median/worst as a single dot-separated cell.
// Returns "—" when no timing data exists.
func runtimeCell(h history) string {
	best, ok := h.best()
	if !ok {
		return "—"
	}
	med, _ := h.median()
	worst, _ := h.worst()
	return fmt.Sprintf("%s · %s · %s",
		formatDuration(best), formatDuration(med), formatDuration(worst))
}

// formatDuration renders a duration as "42s" under a minute and as
// minute/hour granularity otherwise: "3m", "1h04m". Sub-minute
// precision is dropped above a minute since real CI runs are minutes
// long and the extra seconds add noise.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Round(time.Second)/time.Second))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Round(time.Minute)/time.Minute))
	}
	h := int(d / time.Hour)
	m := int((d % time.Hour).Round(time.Minute) / time.Minute)
	return fmt.Sprintf("%dh%02dm", h, m)
}

func sparklineMarkdown(h history) string {
	var b strings.Builder
	for _, e := range h {
		icon := outcomeIcon(e.Outcome)
		if e.URL == "" {
			b.WriteString(icon)
			continue
		}
		fmt.Fprintf(&b, "[%s](%s)", icon, e.URL)
	}
	return b.String()
}

func outcomeIcon(o outcome) string {
	switch o {
	case outcomeSuccess:
		return iconPass
	case outcomeFailure:
		return iconFail
	case outcomeRunning:
		return iconRunning
	case outcomeSkipped:
		return iconSkip
	default:
		return iconMissing
	}
}

func mdEscape(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}
