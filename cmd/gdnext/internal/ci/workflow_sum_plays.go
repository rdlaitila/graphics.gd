package ci

import (
	"fmt"
	"io"
	"sort"
)

// playRow is one (play-host, build-host, target, link) play cell
// across the window. Populated from `gdnext-play` jobs only.
type playRow struct {
	PlayHost     string
	BuildHost    string
	Example      string
	Target       string
	Link         string
	Experimental bool
	History      history
}

type playKey struct{ playHost, buildHost, example, target, link string }

func collectPlays(asc []runWithJobs) []playRow {
	rows := map[playKey]*playRow{}
	var order []playKey
	for i, r := range asc {
		for _, j := range r.Jobs {
			head, axes, ok := splitJobName(j.Name)
			if !ok || head != "gdnext-play" || len(axes) < 5 {
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

func renderPlaysMarkdown(w io.Writer, rows []playRow) {
	var histories []history
	for _, r := range rows {
		histories = append(histories, r.History)
	}
	writeSectionHeader(w, "plays", "Plays", histories)
	if len(rows) == 0 {
		fmt.Fprintln(w, "_No `gdnext-play` jobs in this window._")
		fmt.Fprintln(w)
		return
	}
	writeMarkdownHeader(w, "Example", "Target Host", "Link", "Build Host", "Play Host")
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
