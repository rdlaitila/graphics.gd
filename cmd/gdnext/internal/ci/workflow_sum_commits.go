package ci

import (
	"fmt"
	"io"
)

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
