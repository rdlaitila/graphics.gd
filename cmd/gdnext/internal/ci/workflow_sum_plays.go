package ci

import (
	"fmt"
	"io"
	"sort"
)

// playRow is one (play-host, build-host, target, link, compat) play
// cell across the window. Populated from `gdnext-play` jobs only.
type playRow struct {
	PlayHost  string
	BuildHost string
	Example   string
	Target    string
	Link      string
	Compat    string
	AllowFail bool
	History   history
}

type playKey struct{ playHost, buildHost, example, target, link, compat string }

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
			compat := ""
			if len(axes) >= 6 {
				if c := axes[5]; c != "true" && c != "false" && c != "allow-fail" {
					compat = c
				}
			}
			k := playKey{playHost: axes[0], buildHost: axes[1], example: axes[2], target: axes[3], link: link, compat: compat}
			row, exists := rows[k]
			if !exists {
				row = &playRow{
					PlayHost:  k.playHost,
					BuildHost: k.buildHost,
					Example:   k.example,
					Target:    k.target,
					Link:      k.link,
					Compat:    k.compat,
					History:   make(history, len(asc)),
				}
				rows[k] = row
				order = append(order, k)
			}
			row.AllowFail = exp
			row.History[i] = entryFromJob(j)
		}
	}
	sort.Slice(order, func(a, b int) bool {
		if order[a].example != order[b].example {
			return order[a].example < order[b].example
		}
		if order[a].target != order[b].target {
			return targetRank(order[a].target) < targetRank(order[b].target)
		}
		if order[a].link != order[b].link {
			return linkSubrank(order[a].link) < linkSubrank(order[b].link)
		}
		if order[a].compat != order[b].compat {
			return order[a].compat < order[b].compat
		}
		if order[a].buildHost != order[b].buildHost {
			return hostRank(order[a].buildHost) < hostRank(order[b].buildHost)
		}
		return hostRank(order[a].playHost) < hostRank(order[b].playHost)
	})
	out := make([]playRow, 0, len(order))
	for _, k := range order {
		out = append(out, *rows[k])
	}
	return out
}

// playRank keeps the original (target, link, build host, play host)
// precedence for any other code that still consumes a single int
// (none today, but the helper stays cheap and symmetric with
// buildRank).
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
	writeMarkdownHeader(w, "Example", "Target Host", "Link Mode", "Compat Layer", "Build Host", "Play Host")
	for _, r := range rows {
		link := r.Link
		if link == "" {
			link = "—"
		}
		if r.AllowFail {
			link += " (allow-fail)"
		}
		compat := r.Compat
		if compat == "" {
			compat = "native"
		}
		writeMarkdownRow(w, []string{r.Example, r.Target, link, compat, r.BuildHost, r.PlayHost}, r.History)
	}
	fmt.Fprintln(w)
}
