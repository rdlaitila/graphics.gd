package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/urfave/cli/v3"
)

// WorkflowSummaryCmd queries the GitHub Actions API for the last N runs
// of one workflow and writes a markdown report to stdout (typically
// redirected to $GITHUB_STEP_SUMMARY by the calling workflow step).
// No artefacts or caches are needed: the API already gives us every
// run, every job, and every conclusion.
func WorkflowSummaryCmd() *cli.Command {
	return &cli.Command{
		Name:  "workflow-summary",
		Usage: "render a markdown summary of recent runs to stdout",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "repo", Required: true, Usage: "owner/repo"},
			&cli.StringFlag{Name: "workflow", Required: true, Usage: "workflow filename, e.g. gdnext-ci.yml"},
			&cli.IntFlag{Name: "runs", Value: 14, Usage: "max runs to include"},
			&cli.StringFlag{Name: "branch", Usage: "limit to a single branch (e.g. gdnext-cli)"},
		},
		Action: workflowSummaryAction,
	}
}

func workflowSummaryAction(_ context.Context, cmd *cli.Command) error {
	repo := cmd.String("repo")
	workflow := cmd.String("workflow")
	n := int(cmd.Int("runs"))
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
	renderMarkdown(window, os.Stdout)
	return nil
}

type ghRun struct {
	ID         int64     `json:"id"`
	Number     int       `json:"run_number"`
	Status     string    `json:"status"`
	Conclusion string    `json:"conclusion"`
	Event      string    `json:"event"`
	HeadBranch string    `json:"head_branch"`
	HeadSHA    string    `json:"head_sha"`
	CreatedAt  time.Time `json:"created_at"`
	HTMLURL    string    `json:"html_url"`
}

type ghJob struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
}

type runWithJobs struct {
	Run  ghRun
	Jobs []ghJob
}

type jobSlot struct {
	name    string
	history []string
	group   int
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

func ghError(err error) error {
	if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(ee.Stderr)))
	}
	return err
}

const (
	iconPass = "🟩"
	iconFail = "🟥"
	iconSkip = "⬜"
	iconRun  = "🟨"
	iconNone = "·"

	// Stability glyphs (per row).
	iconStable  = "🟢" // every decisive run passed
	iconBroken  = "🔴" // every decisive run failed
	iconFlaky   = "🟡" // mix of pass + fail across the window
	iconUnknown = "⚪" // no decisive runs yet
)

type jobClass int

const (
	classOther jobClass = iota
	classDiscover
	classCheck
	classBuild
	classSummary
)

func classify(name string) jobClass {
	switch {
	case strings.HasPrefix(name, "discover"):
		return classDiscover
	case strings.HasPrefix(name, "gdnext-ci-checks"):
		return classCheck
	case strings.HasPrefix(name, "gdnext-ci-build"):
		return classBuild
	case name == "Workflow summary" || strings.HasPrefix(name, "summary"):
		return classSummary
	default:
		return classOther
	}
}

// stats summarises a single row's history.
type stats struct {
	seen        int    // runs in which the job actually ran
	pass        int
	fail        int
	other       int    // skipped, cancelled, in-progress, etc.
	transitions int    // pass↔fail flips in the window
	last        string // most recent conclusion ("" = job not in latest run)
	streakCount int    // length of the current trailing run of identical decisive outcomes
	streakKind  string // "success" or "failure" — only valid when streakCount > 0
}

func summarise(history []string) stats {
	var s stats
	var prevDecisive string
	for _, c := range history {
		if c == "" {
			continue
		}
		s.seen++
		switch c {
		case "success":
			s.pass++
		case "failure":
			s.fail++
		default:
			s.other++
			continue // non-decisive: don't update transitions/streak
		}
		if prevDecisive != "" && prevDecisive != c {
			s.transitions++
		}
		prevDecisive = c
	}
	// Trailing streak: walk backwards over decisive outcomes only.
	for i := len(history) - 1; i >= 0; i-- {
		c := history[i]
		if c == "" {
			continue
		}
		if c != "success" && c != "failure" {
			continue
		}
		if s.streakKind == "" {
			s.streakKind = c
		}
		if c != s.streakKind {
			break
		}
		s.streakCount++
	}
	// Latest non-empty entry (decisive or not) drives "last".
	for i := len(history) - 1; i >= 0; i-- {
		if history[i] != "" {
			s.last = history[i]
			break
		}
	}
	return s
}

func (s stats) passPercent() string {
	d := s.pass + s.fail
	if d == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%d%%", int(float64(s.pass)/float64(d)*100))
}

func (s stats) stability() string {
	switch {
	case s.pass+s.fail == 0:
		return iconUnknown
	case s.fail == 0:
		return iconStable
	case s.pass == 0:
		return iconBroken
	default:
		return iconFlaky
	}
}

// streakStr renders the trailing streak: "✓×5" / "✗×2" / "n/a".
func (s stats) streakStr() string {
	if s.streakCount == 0 {
		return "n/a"
	}
	glyph := iconFor(s.streakKind)
	return fmt.Sprintf("%s×%d", glyph, s.streakCount)
}

// flakeStr quantifies intermittent failure: transitions per decisive
// run. 0 = perfectly stable, higher = more flapping. Rendered as a
// small fraction so the reader can tell a 1/12 from a 4/12.
func (s stats) flakeStr() string {
	d := s.pass + s.fail
	if d < 2 {
		return "n/a"
	}
	return fmt.Sprintf("%d/%d", s.transitions, d-1)
}

func jobConclusionFor(jobs []ghJob, name string) string {
	for _, j := range jobs {
		if j.Name != name {
			continue
		}
		if j.Conclusion != "" {
			return j.Conclusion
		}
		return j.Status
	}
	return ""
}

func renderMarkdown(window []runWithJobs, w io.Writer) {
	asc := append([]runWithJobs(nil), window...)
	sort.Slice(asc, func(i, j int) bool {
		return asc[i].Run.CreatedAt.Before(asc[j].Run.CreatedAt)
	})

	slots := map[string]*jobSlot{}
	var names []string
	for _, r := range asc {
		for _, j := range r.Jobs {
			if _, ok := slots[j.Name]; ok {
				continue
			}
			slots[j.Name] = &jobSlot{name: j.Name, group: int(classify(j.Name))}
			names = append(names, j.Name)
		}
	}
	for _, name := range names {
		s := slots[name]
		for _, r := range asc {
			s.history = append(s.history, jobConclusionFor(r.Jobs, name))
		}
	}
	sort.SliceStable(names, func(i, j int) bool {
		if slots[names[i]].group != slots[names[j]].group {
			return slots[names[i]].group < slots[names[j]].group
		}
		return names[i] < names[j]
	})

	fmt.Fprintln(w, "# gdnext-ci workflow summary")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Tracking the last **%d** runs", len(window))
	if len(asc) > 0 {
		fmt.Fprintf(w, " (%s → %s).\n",
			asc[0].Run.CreatedAt.Format("2006-01-02"),
			asc[len(asc)-1].Run.CreatedAt.Format("2006-01-02"))
	} else {
		fmt.Fprintln(w, ".")
	}
	fmt.Fprintln(w)

	writeOverallPie(w, window)
	writePassRateChart(w, asc)
	writeChecksTable(w, names, slots, len(asc))
	writeBuildsTable(w, names, slots, len(asc))
	writeRunsTable(w, asc)
	writeLegend(w)
}

func writeOverallPie(w io.Writer, window []runWithJobs) {
	var pass, fail, other int
	for _, r := range window {
		for _, j := range r.Jobs {
			switch j.Conclusion {
			case "success":
				pass++
			case "failure":
				fail++
			default:
				other++
			}
		}
	}
	total := pass + fail + other
	if total == 0 {
		return
	}
	fmt.Fprintln(w, "## Overall job outcomes")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "**%d** jobs across %d runs — **%d** passed, **%d** failed",
		total, len(window), pass, fail)
	if other > 0 {
		fmt.Fprintf(w, ", %d other (skipped / cancelled / in progress)", other)
	}
	fmt.Fprintln(w, ".")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "```mermaid")
	fmt.Fprintln(w, "pie showData title Job outcomes")
	fmt.Fprintf(w, "    \"Passed\" : %d\n", pass)
	fmt.Fprintf(w, "    \"Failed\" : %d\n", fail)
	if other > 0 {
		fmt.Fprintf(w, "    \"Other\"  : %d\n", other)
	}
	fmt.Fprintln(w, "```")
	fmt.Fprintln(w)
}

func writePassRateChart(w io.Writer, asc []runWithJobs) {
	type point struct {
		label string
		pct   int
	}
	var pts []point
	for _, r := range asc {
		var p, f int
		for _, j := range r.Jobs {
			switch j.Conclusion {
			case "success":
				p++
			case "failure":
				f++
			}
		}
		if p+f == 0 {
			continue
		}
		pts = append(pts, point{
			label: fmt.Sprintf("%d", r.Run.Number),
			pct:   int(float64(p) / float64(p+f) * 100),
		})
	}
	if len(pts) < 2 {
		return
	}
	fmt.Fprintln(w, "## Pass rate per run")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "```mermaid")
	fmt.Fprintln(w, "xychart-beta")
	fmt.Fprintln(w, "    title \"Pass % per run (oldest → newest)\"")
	var labels, values []string
	for _, p := range pts {
		labels = append(labels, "\""+p.label+"\"")
		values = append(values, fmt.Sprintf("%d", p.pct))
	}
	fmt.Fprintf(w, "    x-axis [%s]\n", strings.Join(labels, ", "))
	fmt.Fprintln(w, "    y-axis \"Pass %\" 0 --> 100")
	fmt.Fprintf(w, "    line [%s]\n", strings.Join(values, ", "))
	fmt.Fprintln(w, "```")
	fmt.Fprintln(w)
}

// writeChecksTable: one row per gdnext-ci-checks (<os>) job. Smoke
// suite — failures here typically mean a CLI regression.
func writeChecksTable(w io.Writer, names []string, slots map[string]*jobSlot, runCount int) {
	var rows []string
	for _, name := range names {
		if classify(name) != classCheck {
			continue
		}
		s := summarise(slots[name].history)
		rows = append(rows, fmt.Sprintf("| %s | %d/%d | %d | %d | %s | %s | %s | %s | %s |",
			mdEscape(name),
			s.seen, runCount,
			s.pass, s.fail,
			s.passPercent(),
			s.stability(),
			s.flakeStr(),
			s.streakStr(),
			sparkline(slots[name].history),
		))
	}
	if len(rows) == 0 {
		return
	}
	fmt.Fprintln(w, "## Checks")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Per-host smoke jobs from `gdnext-ci-checks`. These run before any matrix build and gate the rest of the workflow.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| Job | Seen | ✓ | ✗ | Pass % | Stability | Flake | Streak | History |")
	fmt.Fprintln(w, "|---|---:|---:|---:|---:|:-:|:-:|:-:|---|")
	for _, r := range rows {
		fmt.Fprintln(w, r)
	}
	fmt.Fprintln(w)
}

// writeBuildsTable: one row per gdnext-ci-build matrix cell. The job
// name shape is "gdnext-ci-build (<runner>, <example>, <target>, <link>, <experimental>)".
// We parse those axes back out for legibility.
func writeBuildsTable(w io.Writer, names []string, slots map[string]*jobSlot, runCount int) {
	type row struct {
		name, runner, example, target, link, exp string
		s                                        stats
		history                                  []string
	}
	var rows []row
	for _, name := range names {
		if classify(name) != classBuild {
			continue
		}
		runner, example, target, link, exp := parseBuildAxes(name)
		rows = append(rows, row{
			name: name, runner: runner, example: example, target: target,
			link: link, exp: exp,
			s:       summarise(slots[name].history),
			history: slots[name].history,
		})
	}
	if len(rows) == 0 {
		return
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].runner != rows[j].runner {
			return rows[i].runner < rows[j].runner
		}
		if rows[i].target != rows[j].target {
			return rows[i].target < rows[j].target
		}
		return rows[i].link < rows[j].link
	})
	fmt.Fprintln(w, "## Builds")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "One row per `(host runner, target, link-mode)` matrix cell. Experimental cells contribute to the table but their failures are not workflow-fatal.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| Runner | Target | Link | Exp | Seen | ✓ | ✗ | Pass % | Stability | Flake | Streak | History |")
	fmt.Fprintln(w, "|---|---|---|:-:|---:|---:|---:|---:|:-:|:-:|:-:|---|")
	for _, r := range rows {
		expCell := "—"
		if r.exp == "true" {
			expCell = "yes"
		}
		fmt.Fprintf(w, "| %s | %s | %s | %s | %d/%d | %d | %d | %s | %s | %s | %s | %s |\n",
			or(r.runner, "?"), or(r.target, "?"), or(r.link, "default"), expCell,
			r.s.seen, runCount,
			r.s.pass, r.s.fail,
			r.s.passPercent(),
			r.s.stability(),
			r.s.flakeStr(),
			r.s.streakStr(),
			sparkline(r.history),
		)
	}
	fmt.Fprintln(w)
}

// writeRunsTable: one row per run in the window. Used to answer "which
// commit broke it?" at a glance.
func writeRunsTable(w io.Writer, asc []runWithJobs) {
	if len(asc) == 0 {
		return
	}
	fmt.Fprintln(w, "## Runs")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| # | When | Branch | SHA | Result | Jobs (✓ / ✗ / other) | Link |")
	fmt.Fprintln(w, "|---:|---|---|---|---|---|---|")
	for i := len(asc) - 1; i >= 0; i-- {
		r := asc[i]
		c := r.Run.Conclusion
		if c == "" {
			c = r.Run.Status
		}
		sha := r.Run.HeadSHA
		if len(sha) > 7 {
			sha = sha[:7]
		}
		var p, f, o int
		for _, j := range r.Jobs {
			switch j.Conclusion {
			case "success":
				p++
			case "failure":
				f++
			default:
				o++
			}
		}
		fmt.Fprintf(w, "| %d | %s | `%s` | `%s` | %s %s | %d / %d / %d | [open](%s) |\n",
			r.Run.Number,
			r.Run.CreatedAt.Format("2006-01-02 15:04 UTC"),
			r.Run.HeadBranch, sha,
			iconFor(c), c,
			p, f, o,
			r.Run.HTMLURL,
		)
	}
	fmt.Fprintln(w)
}

func writeLegend(w io.Writer) {
	fmt.Fprintln(w, "## Legend")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "### Cell glyphs (in **History**)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| Symbol | Meaning |")
	fmt.Fprintln(w, "|:-:|---|")
	fmt.Fprintf(w, "| %s | Job succeeded |\n", iconPass)
	fmt.Fprintf(w, "| %s | Job failed |\n", iconFail)
	fmt.Fprintf(w, "| %s | Job is queued or in progress |\n", iconRun)
	fmt.Fprintf(w, "| %s | Job was cancelled, skipped, or had no decisive outcome |\n", iconSkip)
	fmt.Fprintf(w, "| %s | Job did not exist in that run (matrix shape changed) |\n", iconNone)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "### Stability column")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| Symbol | Meaning |")
	fmt.Fprintln(w, "|:-:|---|")
	fmt.Fprintf(w, "| %s | Stable — every decisive run passed |\n", iconStable)
	fmt.Fprintf(w, "| %s | Flaky — mix of passes and failures in the window |\n", iconFlaky)
	fmt.Fprintf(w, "| %s | Broken — every decisive run failed |\n", iconBroken)
	fmt.Fprintf(w, "| %s | Unknown — no decisive runs yet |\n", iconUnknown)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "### Columns")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "- **Seen** — runs in which the job actually ran / total runs in the window. `n/a` columns mean no decisive outcomes yet.")
	fmt.Fprintln(w, "- **✓ / ✗** — completed-run outcomes (excludes in-progress, skipped, missing).")
	fmt.Fprintln(w, "- **Pass %** — ✓ / (✓ + ✗); `n/a` when no decisive outcomes.")
	fmt.Fprintln(w, "- **Flake** — pass↔fail transitions / (decisive_runs − 1). `0/N` means perfectly stable; higher = more flapping.")
	fmt.Fprintln(w, "- **Streak** — trailing run of identical decisive outcomes ending at the most recent run (e.g. `🟩×5` = passed the last five times).")
	fmt.Fprintln(w, "- **History** — chronological status per run, oldest leftmost.")
}

func sparkline(history []string) string {
	var b strings.Builder
	for _, c := range history {
		b.WriteString(iconFor(c))
	}
	return b.String()
}

func iconFor(conclusion string) string {
	switch conclusion {
	case "success":
		return iconPass
	case "failure":
		return iconFail
	case "in_progress", "queued", "pending":
		return iconRun
	case "":
		return iconNone
	default:
		return iconSkip
	}
}

func mdEscape(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func or(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// parseBuildAxes pulls the matrix axes out of a job name shaped like:
//
//	gdnext-ci-build (ubuntu-latest, canarybird, linux/amd64, gdextension, false)
//
// Returns (runner, example, target, link, experimental) — empty
// strings for any axis that's missing.
func parseBuildAxes(name string) (runner, example, target, link, exp string) {
	open := strings.Index(name, "(")
	close := strings.LastIndex(name, ")")
	if open < 0 || close < 0 || close <= open {
		return
	}
	axes := strings.Split(name[open+1:close], ",")
	for i := range axes {
		axes[i] = strings.TrimSpace(axes[i])
	}
	if len(axes) > 0 {
		runner = axes[0]
	}
	if len(axes) > 1 {
		example = axes[1]
	}
	if len(axes) > 2 {
		target = axes[2]
	}
	if len(axes) > 3 {
		link = axes[3]
	}
	if len(axes) > 4 {
		exp = axes[4]
	}
	return
}
