package ci

import (
	"fmt"
	"io"
	"sort"
)

// buildRow is one (host, example, target, link) build cell across the
// window.
type buildRow struct {
	Host      string
	Example   string
	Target    string
	Link      string
	AllowFail bool
	History   history
}

type buildKey struct{ host, example, target, link string }

func collectBuilds(asc []runWithJobs) []buildRow {
	rows := map[buildKey]*buildRow{}
	var order []buildKey
	for i, r := range asc {
		for _, j := range r.Jobs {
			head, axes, ok := splitJobName(j.Name)
			if !ok || head != "gdnext-build" || len(axes) < 3 {
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
			// asc is oldest-first, so the newest run's allow-fail flag
			// is what's left in place — changes to a Quirk's scope or to
			// Status's Experimental bit reflect immediately, not on a
			// window-rotation delay.
			row.AllowFail = exp
			row.History[i] = entryFromJob(j)
		}
	}
	sort.Slice(order, func(a, b int) bool {
		if order[a].example != order[b].example {
			return order[a].example < order[b].example
		}
		if order[a].target != order[b].target {
			return order[a].target < order[b].target
		}
		if order[a].link != order[b].link {
			return linkSubrank(order[a].link) < linkSubrank(order[b].link)
		}
		return hostRank(order[a].host) < hostRank(order[b].host)
	})
	out := make([]buildRow, 0, len(order))
	for _, k := range order {
		out = append(out, *rows[k])
	}
	return out
}

// buildRank keeps the original (host, target, link) precedence so the
// failure-list sort in workflow_sum_failures.go stays aligned with
// the historical layout. The build-table sort above uses its own
// multi-key comparator.
func buildRank(k buildKey) int {
	return hostRank(k.host)*1_000_000 + targetRank(k.target)*1000 + linkSubrank(k.link)
}

func renderBuildsMarkdown(w io.Writer, rows []buildRow) {
	var histories []history
	for _, r := range rows {
		histories = append(histories, r.History)
	}
	writeSectionHeader(w, "builds", "Builds", histories)
	if len(rows) == 0 {
		fmt.Fprintln(w, "_No `gdnext-build` jobs in this window._")
		fmt.Fprintln(w)
		return
	}
	writeMarkdownHeader(w, "Example", "Target Host", "Link Mode", "Build Host")
	for _, r := range rows {
		link := r.Link
		if link == "" {
			link = "—"
		}
		if r.AllowFail {
			link += " (allow-fail)"
		}
		writeMarkdownRow(w, []string{r.Example, r.Target, link, r.Host}, r.History)
	}
	fmt.Fprintln(w)
}
