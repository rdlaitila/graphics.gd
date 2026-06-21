package internal

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

	"graphics.gd/product"

	"github.com/urfave/cli/v3"
)

// WorkflowSummaryCmd fetches the last N gdnext-ci runs and renders a
// markdown report to stdout. The workflow step pipes stdout into
// $GITHUB_STEP_SUMMARY.
func WorkflowSummaryCmd() *cli.Command {
	return &cli.Command{
		Name:  "workflow-summary",
		Usage: "render a rolling markdown summary of recent runs",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "repo", Required: true, Usage: "owner/repo"},
			&cli.StringFlag{Name: "workflow", Required: true, Usage: "workflow filename, e.g. gdnext-ci.yml"},
			&cli.IntFlag{Name: "runs", Value: 14, Usage: "max runs to include"},
			&cli.IntFlag{Name: "log-tail", Value: 60, Usage: "lines of log to tail per failed job in the latest run"},
			&cli.StringFlag{Name: "branch", Usage: "limit to a single branch (e.g. gdnext-cli)"},
		},
		Action: workflowSummaryAction,
	}
}

func workflowSummaryAction(_ context.Context, cmd *cli.Command) error {
	repo := cmd.String("repo")
	workflow := cmd.String("workflow")
	n := int(cmd.Int("runs"))
	tail := int(cmd.Int("log-tail"))
	branch := cmd.String("branch")
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
	return renderMarkdown(os.Stdout, s)
}

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

// ghaTimestamp matches the ISO timestamp GHA prepends to every log
// line; stripping it claws back ~30 columns.
var ghaTimestamp = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T[\d:.]+Z\s`)

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
// Output of collect() and input of every renderer. Adding a new format
// (JSON, HTML, ANSI) means writing a new render*() against summary —
// the collector stays untouched.

// outcome normalises GitHub's (status, conclusion) pair into a single
// enum so renderers and stats only switch on one value.
type outcome uint8

const (
	outcomeMissing outcome = iota
	outcomeSuccess
	outcomeFailure
	outcomeSkipped
	outcomeRunning
)

func outcomeFromJob(j ghJob) outcome {
	switch j.Conclusion {
	case "success":
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
type entry struct {
	Outcome  outcome
	Duration time.Duration
	URL      string
}

func entryFromJob(j ghJob) entry {
	var d time.Duration
	if !j.StartedAt.IsZero() && !j.CompletedAt.IsZero() && j.CompletedAt.After(j.StartedAt) {
		d = j.CompletedAt.Sub(j.StartedAt)
	}
	return entry{Outcome: outcomeFromJob(j), Duration: d, URL: j.HTMLURL}
}

// history is one row's chronological appearances, oldest first.
// Indices align across rows so sparklines line up vertically.
type history []entry

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

// passPercentChange compares the older half of the row's appearances
// to the newer half and returns the signed delta in percentage
// points. Missing slots are filtered first so young rows still get
// a trend. ok=false when fewer than two appearances exist or either
// half is non-decisive.
func (t history) passPercentChange() (delta int, ok bool) {
	seen := make(history, 0, len(t))
	for _, e := range t {
		if e.Outcome != outcomeMissing {
			seen = append(seen, e)
		}
	}
	if len(seen) < 2 {
		return 0, false
	}
	mid := len(seen) / 2
	older, oldOk := seen[:mid].passPercent()
	newer, newOk := seen[mid:].passPercent()
	if !oldOk || !newOk {
		return 0, false
	}
	return newer - older, true
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

// summary is the typed snapshot a renderer consumes. Markdown is the
// only renderer today; JSON / HTML / ANSI slot in next to it without
// touching the collector or the GitHub API layer.
type summary struct {
	Branch       string
	Window       []runMeta
	Checks       []checkRow
	Builds       []buildRow
	Plays        []playRow
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

// failureRow is one failed job from the most recent run, with enough
// context (title, link, log tail) to triage from the summary alone.
type failureRow struct {
	Job     string
	Title   string
	Step    string
	URL     string
	LogTail []string
}

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

// checkRow is one host's smoke-check job across the window.
type checkRow struct {
	Host    string
	History history
}

// buildRow is one (host, example, target, link) build cell across the
// window.
type buildRow struct {
	Host         string
	Example      string
	Target       string
	Link         string
	Experimental bool
	History      history
}

// playRow is one (play-host, build-host, target, link) play cell
// across the window. Populated from `gdnext-ci-play` jobs only.
type playRow struct {
	PlayHost     string
	BuildHost    string
	Example      string
	Target       string
	Link         string
	Experimental bool
	History      history
}

// --- Collector -------------------------------------------------------
// gh API → summary. Sorts everything into deterministic order so
// renderers can be straight-line printers.

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

func collectChecks(asc []runWithJobs) []checkRow {
	rows := map[string]*checkRow{}
	var keys []string
	for i, r := range asc {
		for _, j := range r.Jobs {
			head, axes, ok := splitJobName(j.Name)
			if !ok || head != "gdnext-ci-checks" || len(axes) < 1 {
				continue
			}
			host := axes[0]
			row, exists := rows[host]
			if !exists {
				row = &checkRow{Host: host, History: make(history, len(asc))}
				rows[host] = row
				keys = append(keys, host)
			}
			row.History[i] = entryFromJob(j)
		}
	}
	sort.Slice(keys, func(a, b int) bool { return hostRank(keys[a]) < hostRank(keys[b]) })
	out := make([]checkRow, 0, len(keys))
	for _, k := range keys {
		out = append(out, *rows[k])
	}
	return out
}

type buildKey struct{ host, example, target, link string }

func collectBuilds(asc []runWithJobs) []buildRow {
	rows := map[buildKey]*buildRow{}
	var order []buildKey
	for i, r := range asc {
		for _, j := range r.Jobs {
			head, axes, ok := splitJobName(j.Name)
			if !ok || head != "gdnext-ci-build" || len(axes) < 3 {
				continue
			}
			link, exp := parseLinkExp(axes[3:])
			k := buildKey{host: axes[0], example: axes[1], target: axes[2], link: link}
			row, exists := rows[k]
			if !exists {
				row = &buildRow{
					Host:    k.host,
					Example: k.example,
					Target:  k.target,
					Link:    k.link,
					History: make(history, len(asc)),
				}
				rows[k] = row
				order = append(order, k)
			}
			// asc is oldest-first, so this leaves the newest run's
			// experimental flag in place — flipping a platform between
			// experimental and stable in product.PlatformMatrix is
			// reflected immediately, not on a window-rotation delay.
			row.Experimental = exp
			row.History[i] = entryFromJob(j)
		}
	}
	sort.Slice(order, func(a, b int) bool {
		return buildRank(order[a]) < buildRank(order[b])
	})
	out := make([]buildRow, 0, len(order))
	for _, k := range order {
		out = append(out, *rows[k])
	}
	return out
}

// parseLinkExp pulls (link, experimental) out of the suffix axes
// after (host, example, target). Pinned build job names render the
// suffix as `(link, experimental)`; pre-rename runs in the window
// include extra trailing axes — content detection (LinkMode catalog
// for link, bool literal for experimental) stays robust to both.
func parseLinkExp(tail []string) (link string, experimental bool) {
	if len(tail) > 0 && isLinkMode(tail[0]) {
		link = tail[0]
	}
	for _, t := range tail {
		if t == "true" {
			experimental = true
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

type playKey struct{ playHost, buildHost, example, target, link string }

func collectPlays(asc []runWithJobs) []playRow {
	rows := map[playKey]*playRow{}
	var order []playKey
	for i, r := range asc {
		for _, j := range r.Jobs {
			head, axes, ok := splitJobName(j.Name)
			if !ok || head != "gdnext-ci-play" || len(axes) < 5 {
				continue
			}
			link, exp := parseLinkExp(axes[4:])
			k := playKey{playHost: axes[0], buildHost: axes[1], example: axes[2], target: axes[3], link: link}
			row, exists := rows[k]
			if !exists {
				row = &playRow{
					PlayHost:  k.playHost,
					BuildHost: k.buildHost,
					Example:   k.example,
					Target:    k.target,
					Link:      k.link,
					History:   make(history, len(asc)),
				}
				rows[k] = row
				order = append(order, k)
			}
			row.Experimental = exp
			row.History[i] = entryFromJob(j)
		}
	}
	sort.Slice(order, func(a, b int) bool {
		return playRank(order[a]) < playRank(order[b])
	})
	out := make([]playRow, 0, len(order))
	for _, k := range order {
		out = append(out, *rows[k])
	}
	return out
}

func playRank(k playKey) int {
	return targetRank(k.target)*1_000_000_000 + linkSubrank(k.link)*10_000_000 + hostRank(k.buildHost)*10_000 + hostRank(k.playHost)*10
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

// collectFailures walks the newest run's failed jobs, fetches the log
// of the step that actually failed, and returns one failureRow per.
// Returns an empty slice (not nil) when the run had no failures so
// renderers can distinguish "no failures" from "no run".
func collectFailures(repo string, window []runWithJobs, tail int) []failureRow {
	latest, ok := latestRun(window)
	if !ok {
		return nil
	}
	out := make([]failureRow, 0)
	for _, j := range latest.Jobs {
		if j.Conclusion != "failure" {
			continue
		}
		step, stepOK := firstFailedStep(j)
		log, err := fetchJobLog(repo, j.ID)
		var lines []string
		switch {
		case err != nil:
			lines = []string{fmt.Sprintf("(log unavailable: %v)", err)}
		case !stepOK:
			lines = tailLines(log, tail)
		default:
			lines = tailLines(extractStepLog(log, step.Number), tail)
		}
		name := ""
		if stepOK {
			name = step.Name
		}
		out = append(out, failureRow{
			Job:     j.Name,
			Title:   failureTitle(j.Name),
			Step:    name,
			URL:     j.HTMLURL,
			LogTail: lines,
		})
	}
	sort.SliceStable(out, func(a, b int) bool {
		return failureRank(out[a]) < failureRank(out[b])
	})
	return out
}

// firstFailedStep returns the lowest-numbered step in the job that
// GitHub marked as failed. ok=false when the job has no per-step
// data (e.g. setup failure before any user step ran).
func firstFailedStep(j ghJob) (ghStep, bool) {
	steps := append([]ghStep(nil), j.Steps...)
	sort.Slice(steps, func(a, b int) bool { return steps[a].Number < steps[b].Number })
	for _, s := range steps {
		if s.Conclusion == "failure" {
			return s, true
		}
	}
	return ghStep{}, false
}

// extractStepLog returns just the failing step's section of the
// per-job log. The runner wraps each step's metadata in a top-level
// `##[group]Run …` but the step's actual command output runs at
// depth 0 between that group's endgroup and the next step's
// boundary marker. Nested user-emitted `::group::` is depth-tracked.
// API step numbering starts at 1 for "Set up job", so (number-1)
// counts user `Run ` markers.
func extractStepLog(log string, stepNumber int) string {
	target := stepNumber - 1
	if target < 1 {
		return log
	}
	lines := strings.Split(log, "\n")
	depth, runs := 0, 0
	start := -1
	for i, l := range lines {
		s := strings.TrimSpace(stripTimestamp(l))
		if strings.HasPrefix(s, "##[group]") {
			if depth == 0 && isStepBoundary(s) {
				if start >= 0 {
					return strings.Join(lines[start:i], "\n")
				}
				if strings.HasPrefix(s, "##[group]Run ") {
					runs++
					if runs == target {
						start = i
					}
				}
			}
			depth++
			continue
		}
		if s == "##[endgroup]" {
			depth--
		}
	}
	if start >= 0 {
		return strings.Join(lines[start:], "\n")
	}
	return log
}

func isStepBoundary(line string) bool {
	body := strings.TrimPrefix(line, "##[group]")
	return strings.HasPrefix(body, "Run ") ||
		strings.HasPrefix(body, "Post Run ") ||
		body == "Complete job"
}

// failureTitle formats a job name into a vertical-friendly identifier.
// Falls back to the raw name when the job isn't matrix-shaped.
func failureTitle(name string) string {
	head, axes, ok := splitJobName(name)
	if !ok {
		return name
	}
	switch head {
	case "gdnext-ci-checks":
		if len(axes) >= 1 {
			return "Check on " + axes[0]
		}
	case "gdnext-ci-build":
		if len(axes) >= 4 {
			return fmt.Sprintf("Build %s [%s] on %s", axes[2], axes[3], axes[0])
		}
	case "gdnext-ci-play":
		if len(axes) >= 5 {
			return fmt.Sprintf("Play %s [%s] on %s (built on %s)", axes[3], axes[4], axes[0], axes[1])
		}
	}
	return name
}

func failureRank(f failureRow) int {
	head, axes, ok := splitJobName(f.Job)
	if !ok {
		return 1_000_000_000
	}
	switch head {
	case "gdnext-ci-checks":
		if len(axes) >= 1 {
			return hostRank(axes[0])
		}
	case "gdnext-ci-build":
		if len(axes) >= 4 {
			return 1_000_000 + buildRank(buildKey{host: axes[0], example: axes[1], target: axes[2], link: axes[3]})
		}
	case "gdnext-ci-play":
		return 500_000_000
	}
	return 1_000_000_000
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

func buildRank(k buildKey) int {
	return hostRank(k.host)*1_000_000 + targetRank(k.target)*1000 + linkSubrank(k.link)
}

// --- Markdown renderer -----------------------------------------------

const (
	iconPass    = "🟩"
	iconFail    = "🟥"
	iconSkip    = "⬜"
	iconRunning = "🟨"
	iconMissing = "·"
)

func renderMarkdown(w io.Writer, s summary) error {
	fmt.Fprintln(w, "# gdnext-ci")
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
	renderToolchainsMarkdown(w, s.Toolchains)
	renderFailuresMarkdown(w, s.LastFailures)
	renderCommitsMarkdown(w, s.Window)
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
	fmt.Fprintln(w, "**Contents:** [Checks](#checks) · [Builds](#builds) · [Plays](#plays) · [Toolchains](#toolchains) · [Latest run failures](#latest-run-failures) · [Commits](#commits)")
	fmt.Fprintln(w)
}

func renderChecksMarkdown(w io.Writer, rows []checkRow) {
	var histories []history
	for _, r := range rows {
		histories = append(histories, r.History)
	}
	writeSectionHeader(w, "checks", "Checks", histories)
	if len(rows) == 0 {
		fmt.Fprintln(w, "_No `gdnext-ci-checks` jobs in this window._")
		fmt.Fprintln(w)
		return
	}
	writeMarkdownHeader(w, "Host")
	for _, r := range rows {
		writeMarkdownRow(w, []string{r.Host}, r.History)
	}
	fmt.Fprintln(w)
}

func renderBuildsMarkdown(w io.Writer, rows []buildRow) {
	var histories []history
	for _, r := range rows {
		histories = append(histories, r.History)
	}
	writeSectionHeader(w, "builds", "Builds", histories)
	if len(rows) == 0 {
		fmt.Fprintln(w, "_No `gdnext-ci-build` jobs in this window._")
		fmt.Fprintln(w)
		return
	}
	writeMarkdownHeader(w, "Example", "Target", "Link", "Build host")
	for _, r := range rows {
		link := r.Link
		if link == "" {
			link = "—"
		}
		if r.Experimental {
			link += " (exp.)"
		}
		writeMarkdownRow(w, []string{r.Example, r.Target, link, r.Host}, r.History)
	}
	fmt.Fprintln(w)
}

func renderPlaysMarkdown(w io.Writer, rows []playRow) {
	var histories []history
	for _, r := range rows {
		histories = append(histories, r.History)
	}
	writeSectionHeader(w, "plays", "Plays", histories)
	if len(rows) == 0 {
		fmt.Fprintln(w, "_No `gdnext-ci-play` jobs in this window._")
		fmt.Fprintln(w)
		return
	}
	writeMarkdownHeader(w, "Example", "Target", "Link", "Build host", "Play host")
	for _, r := range rows {
		link := r.Link
		if link == "" {
			link = "—"
		}
		if r.Experimental {
			link += " (exp.)"
		}
		writeMarkdownRow(w, []string{r.Example, r.Target, link, r.BuildHost, r.PlayHost}, r.History)
	}
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

func renderFailuresMarkdown(w io.Writer, rows []failureRow) {
	fmt.Fprintln(w, `<h2 id="latest-run-failures">Latest run failures</h2>`)
	fmt.Fprintln(w)
	if rows == nil {
		fmt.Fprintln(w, "_No runs in the window._")
		fmt.Fprintln(w)
		return
	}
	if len(rows) == 0 {
		fmt.Fprintln(w, "_No failures in the most recent run._ 🎉")
		fmt.Fprintln(w)
		return
	}
	fmt.Fprintf(w, "**%d** job(s) failed in the most recent run. Expand to see the log tail.\n\n", len(rows))
	for _, r := range rows {
		fmt.Fprintln(w, "<details>")
		title := htmlEscape(r.Title)
		if r.URL != "" {
			title += fmt.Sprintf(" (<a href=\"%s\">open job</a>)", r.URL)
		}
		fmt.Fprintf(w, "<summary><b>%s</b></summary>\n\n", title)
		if len(r.LogTail) == 0 {
			fmt.Fprintln(w, "_log empty_")
		} else {
			fmt.Fprintln(w, "```log")
			for _, line := range r.LogTail {
				fmt.Fprintln(w, line)
			}
			fmt.Fprintln(w, "```")
		}
		fmt.Fprintln(w)
		fmt.Fprintln(w, "</details>")
		fmt.Fprintln(w)
	}
}

// renderCommitsMarkdown prints one row per run in the window: the
// commit that triggered it, the event type, the run's conclusion, and
// a link to the run page. Newest-first to match how a reader scans
// the history (most recent activity first).
func renderCommitsMarkdown(w io.Writer, runs []runMeta) {
	fmt.Fprintln(w, `<h2 id="commits">Commits</h2>`)
	fmt.Fprintln(w)
	if len(runs) == 0 {
		fmt.Fprintln(w, "_No runs in the window._")
		fmt.Fprintln(w)
		return
	}
	fmt.Fprintln(w, "| When | SHA | Event | Branch | Pass | Subject |")
	fmt.Fprintln(w, "| --- | --- | --- | --- | ---: | --- |")
	for i := len(runs) - 1; i >= 0; i-- {
		r := runs[i]
		sha := r.HeadSHA
		if len(sha) > 7 {
			sha = sha[:7]
		}
		shaCell := "`" + sha + "`"
		if r.URL != "" {
			shaCell = fmt.Sprintf("[`%s`](%s)", sha, r.URL)
		}
		passCell := runPassCell(r)
		fmt.Fprintf(w, "| %s | %s | %s | `%s` | %s | %s |\n",
			r.CreatedAt.Format("2006-01-02 15:04"),
			shaCell,
			r.Event,
			r.HeadBranch,
			passCell,
			mdEscape(r.Title),
		)
	}
	fmt.Fprintln(w)
}

// runPassCell renders "P/N" plus a pass percentage for a run. Falls
// back to the run-level conclusion ("in progress", "cancelled") when
// no decisive jobs are present, so still-running or aborted runs are
// distinguishable from runs with zero passing jobs.
func runPassCell(r runMeta) string {
	total := r.Pass + r.Fail
	if total == 0 {
		label := r.Conclusion
		if label == "" {
			label = "—"
		}
		return label
	}
	pct := int(float64(r.Pass) / float64(total) * 100)
	return fmt.Sprintf("%d%% (%d/%d)", pct, r.Pass, total)
}
