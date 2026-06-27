package ci

import (
	"fmt"
	"io"
	"sort"
)

// testRow is one (host, link) test cell across the window.
type testRow struct {
	Host    string
	Link    string
	History history
}

type testKey struct{ host, link string }

func collectTests(asc []runWithJobs) []testRow {
	rows := map[testKey]*testRow{}
	var order []testKey
	for i, r := range asc {
		for _, j := range r.Jobs {
			head, axes, ok := splitJobName(j.Name)
			if !ok || head != "gdnext-test" || len(axes) == 0 {
				continue
			}
			host := axes[0]
			var link string
			if len(axes) > 1 {
				link = axes[1]
			}
			k := testKey{host: host, link: link}
			row, exists := rows[k]
			if !exists {
				row = &testRow{Host: host, Link: link, History: make(history, len(asc))}
				rows[k] = row
				order = append(order, k)
			}
			row.History[i] = entryFromJob(j)
		}
	}
	sort.Slice(order, func(a, b int) bool {
		if order[a].host != order[b].host {
			return hostRank(order[a].host) < hostRank(order[b].host)
		}
		return linkSubrank(order[a].link) < linkSubrank(order[b].link)
	})
	out := make([]testRow, 0, len(order))
	for _, k := range order {
		out = append(out, *rows[k])
	}
	return out
}

func renderTestsMarkdown(w io.Writer, rows []testRow) {
	var histories []history
	for _, r := range rows {
		histories = append(histories, r.History)
	}
	writeSectionHeader(w, "tests", "Tests", histories)
	if len(rows) == 0 {
		fmt.Fprintln(w, "_No `gdnext-test` jobs in this window._")
		fmt.Fprintln(w)
		return
	}
	writeMarkdownHeader(w, "Build Host", "Link Mode")
	for _, r := range rows {
		link := r.Link
		if link == "" {
			link = "—"
		}
		writeMarkdownRow(w, []string{r.Host, link}, r.History)
	}
	fmt.Fprintln(w)
}
