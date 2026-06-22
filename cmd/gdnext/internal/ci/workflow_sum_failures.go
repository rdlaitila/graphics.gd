package ci

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// failureRow is one failed job from the most recent run, with enough
// context (title, link, log tail) to triage from the summary alone.
type failureRow struct {
	Job     string
	Title   string
	Step    string
	URL     string
	LogTail []string
}

// collectFailures pulls failed jobs from the newest run in window and
// tails their per-step log so triage can happen from the summary
// alone. Returns an empty slice (not nil) when the run had no
// failures so renderers can distinguish "no failures" from "no run".
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
	case "gdnext-checks":
		if len(axes) >= 1 {
			return "Check on " + axes[0]
		}
	case "gdnext-build":
		if len(axes) >= 4 {
			return fmt.Sprintf("Build %s [%s] on %s", axes[2], axes[3], axes[0])
		}
	case "gdnext-play":
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
	case "gdnext-checks":
		if len(axes) >= 1 {
			return hostRank(axes[0])
		}
	case "gdnext-build":
		if len(axes) >= 4 {
			return 1_000_000 + buildRank(buildKey{host: axes[0], example: axes[1], target: axes[2], link: axes[3]})
		}
	case "gdnext-play":
		return 500_000_000
	}
	return 1_000_000_000
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
