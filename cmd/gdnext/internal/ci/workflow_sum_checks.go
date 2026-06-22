package ci

import (
	"fmt"
	"io"
	"sort"
)

// checkRow is one host's smoke-check job across the window.
type checkRow struct {
	Host    string
	History history
}

func collectChecks(asc []runWithJobs) []checkRow {
	rows := map[string]*checkRow{}
	var keys []string
	for i, r := range asc {
		for _, j := range r.Jobs {
			head, axes, ok := splitJobName(j.Name)
			if !ok || head != "gdnext-checks" || len(axes) < 1 {
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

func renderChecksMarkdown(w io.Writer, rows []checkRow) {
	var histories []history
	for _, r := range rows {
		histories = append(histories, r.History)
	}
	writeSectionHeader(w, "checks", "Checks", histories)
	if len(rows) == 0 {
		fmt.Fprintln(w, "_No `gdnext-checks` jobs in this window._")
		fmt.Fprintln(w)
		return
	}
	writeMarkdownHeader(w, "Build Host")
	for _, r := range rows {
		writeMarkdownRow(w, []string{r.Host}, r.History)
	}
	fmt.Fprintln(w)
}
