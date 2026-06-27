package ci

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"graphics.gd/product"
)

// shotRow is one cell's screenshot picked up from a flat
// `shots/<artefact-name>.png` tree (download-artifact with
// `merge-multiple: true`). The PNG bytes are loaded eagerly so the
// renderer can inline them as `data:` URIs in the markdown step
// summary — GitHub doesn't host arbitrary artefact images for inline
// display.
//
// Label is the head of the caption (`<target>+<link>`); Tail is the
// context line (`b: <build-host> p: <play-host>`). Splitting them lets
// the markdown renderer break between the two with `<br>` so a narrow
// grid column doesn't overflow.
//
// Label is the head of the caption (`<target>+<link>`); Tail is the
// context line (`b: <build-host> p: <play-host>`). Splitting them lets
// the markdown renderer break between the two with `<br>` so a narrow
// grid column doesn't overflow.
type shotRow struct {
	Label string
	Tail  string
	PNG   []byte
}

// ScreenshotArtifactName is the canonical upload/download name for
// one play cell's screenshot. Both the workflow YAML and the
// renderer derive it from the same fields so a rename only happens
// in one place.
//
// Shape: `shot-<play-runner>-play-<build-runner>-<example>-<goos>-<goarch>[-<link>][-<compat>]`.
// Compat is appended last so cells driving the same artefact through
// different compatibility layers (wine, proton, proton-9, ...) upload
// to distinct slots. Native cells omit the compat suffix.
func ScreenshotArtifactName(playRunner, buildRunner, example, target, link, compat string) string {
	name := "shot-" + playRunner + "-" + ArtifactName(buildRunner, example, target, link)
	if compat != "" {
		name += "-" + compat
	}
	return name
}

// parseScreenshotArtifactName recovers (target, link, compat, buildRunner, playRunner)
// from a name produced by ScreenshotArtifactName. Returns ok=false when
// the name doesn't match the expected shape so collectShots can fall
// back to the raw token list rather than mislabelling the cell.
func parseScreenshotArtifactName(name string) (target, link, compat, buildRunner, playRunner string, ok bool) {
	rest, found := strings.CutPrefix(name, "shot-")
	if !found {
		return "", "", "", "", "", false
	}
	head, buildRest, found := strings.Cut(rest, "-play-")
	if !found || head == "" {
		return "", "", "", "", "", false
	}
	// buildRest = `<build-runner>-<example>-<goos>-<goarch>[-<link>][-<compat>]`.
	// Runner labels are `<os>-latest`; split off the first two tokens.
	parts := strings.SplitN(buildRest, "-", 3)
	if len(parts) < 3 || !strings.HasSuffix(parts[1], "latest") {
		return "", "", "", "", "", false
	}
	buildRunner = parts[0] + "-" + parts[1]
	tail := parts[2]
	// Peel a compat suffix off if one of the known compat tokens
	// from product.PlayMatrix matches. Longest-first to handle
	// multi-token compats (android-emu, proton-10) before shorter
	// ones that would otherwise prefix-match (proton).
	for _, c := range knownCompatSuffixes() {
		if t, ok := strings.CutSuffix(tail, "-"+c); ok {
			compat = c
			tail = t
			break
		}
	}
	tokens := strings.Split(tail, "-")
	if len(tokens) < 3 {
		return "", "", "", "", "", false
	}
	knownLink := map[string]bool{"gdextension": true, "libgodot": true}
	if last := tokens[len(tokens)-1]; knownLink[last] {
		link = last
		tokens = tokens[:len(tokens)-1]
	}
	if len(tokens) < 3 {
		return "", "", "", "", "", false
	}
	archIdx := len(tokens) - 1
	goosIdx := archIdx - 1
	target = tokens[goosIdx] + "/" + tokens[archIdx]
	return target, link, compat, buildRunner, head, true
}

// knownCompatSuffixes returns the distinct CompatLayer values from
// product.PlayMatrix sorted by length descending, so longest match
// wins when one compat name is a prefix of another (e.g. "proton"
// vs "proton-10"). Cached on first call; PlayMatrix is package data
// and doesn't change at runtime.
func knownCompatSuffixes() []string {
	if cachedCompatSuffixes != nil {
		return cachedCompatSuffixes
	}
	seen := map[string]bool{}
	var out []string
	for _, p := range product.PlayMatrix {
		if p.CompatLayer == "" || seen[p.CompatLayer] {
			continue
		}
		seen[p.CompatLayer] = true
		out = append(out, p.CompatLayer)
	}
	sort.Slice(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	cachedCompatSuffixes = out
	return out
}

var cachedCompatSuffixes []string

// collectShots reads <shot-*>.png files in dir and returns one row per
// cell, sorted by caption.
func collectShots(dir string) []shotRow {
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "shots: cannot read --shots dir %q: %v\n", dir, err)
		return nil
	}
	var out []shotRow
	var skipped []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		filename := e.Name()
		name, ok := strings.CutSuffix(filename, ".png")
		if !ok || !strings.HasPrefix(name, "shot-") {
			continue
		}
		shot := filepath.Join(dir, filename)
		body, err := os.ReadFile(shot)
		if err != nil || len(body) == 0 {
			skipped = append(skipped, fmt.Sprintf("%s (%v)", shot, err))
			continue
		}
		label, tail := shotLabel(name)
		out = append(out, shotRow{Label: label, Tail: tail, PNG: body})
	}
	if len(out) == 0 && len(entries) > 0 {
		fmt.Fprintf(os.Stderr, "shots: --shots dir %q had %d entries but none yielded a screenshot:\n", dir, len(entries))
		for _, e := range entries {
			fmt.Fprintf(os.Stderr, "  - %s (dir=%v)\n", e.Name(), e.IsDir())
		}
		for _, s := range skipped {
			fmt.Fprintf(os.Stderr, "  skipped: %s\n", s)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Label != out[j].Label {
			return out[i].Label < out[j].Label
		}
		return out[i].Tail < out[j].Tail
	})
	return out
}

// shotLabel renders the caption for one screenshot cell. The first
// return value is the headline (`<target>+<link>`); the second is
// the build/play context line (`b: <build-host> p: <play-host>`).
// Splitting them lets the markdown renderer put a `<br>` between
// the two so the caption wraps cleanly inside a narrow grid column
// without overflowing the title, while the alt text + slug paths
// can still concatenate them into one line. Falls back to the raw
// trimmed name when parsing fails so unknown shapes still show up
// rather than disappearing silently.
func shotLabel(name string) (head, tail string) {
	target, link, compat, buildRunner, playRunner, ok := parseScreenshotArtifactName(name)
	if !ok {
		return strings.TrimPrefix(name, "shot-"), ""
	}
	head = target
	if link != "" {
		head += "+" + link
	}
	if compat != "" {
		head += " (" + compat + ")"
	}
	tail = fmt.Sprintf("b: %s p: %s", buildRunner, playRunner)
	return head, tail
}

// shotAltText joins the head + tail with " " so the alt attribute
// keeps the full context on one line; missing-image tooltips and
// screen readers don't render `<br>` usefully.
func shotAltText(head, tail string) string {
	if tail == "" {
		return head
	}
	return head + " " + tail
}

// renderShotsMarkdown appends a screenshot grid directly under the
// preceding Plays table — no section header, so it reads as part of
// the same section. Emits raw HTML rather than a Markdown table so
// the grid doesn't carry an empty header row (GFM tables require one)
// and the column count adapts to however many shots we have, capped
// at maxCols for layout.
func renderShotsMarkdown(w io.Writer, rows []pushedShot) {
	if len(rows) == 0 {
		return
	}
	const maxCols = 3
	cols := len(rows)
	if cols > maxCols {
		cols = maxCols
	}
	cellPct := 100 / cols
	fmt.Fprintln(w, `<table>`)
	for i := 0; i < len(rows); i += cols {
		// Caption row.
		fmt.Fprint(w, "<tr>")
		for c := 0; c < cols; c++ {
			if i+c >= len(rows) {
				fmt.Fprintf(w, `<td width="%d%%"></td>`, cellPct)
				continue
			}
			row := rows[i+c]
			caption := htmlEscapeCell(row.Label)
			if row.Tail != "" {
				caption += "<br>" + htmlEscapeCell(row.Tail)
			}
			fmt.Fprintf(w, `<td width="%d%%" align="center"><strong>%s</strong></td>`, cellPct, caption)
		}
		fmt.Fprintln(w, "</tr>")
		// Image row.
		fmt.Fprint(w, "<tr>")
		for c := 0; c < cols; c++ {
			if i+c >= len(rows) {
				fmt.Fprintf(w, `<td width="%d%%"></td>`, cellPct)
				continue
			}
			row := rows[i+c]
			fmt.Fprintf(w, `<td width="%d%%" align="center">`, cellPct)
			switch {
			case row.Error != "":
				fmt.Fprintf(w, "⚠️ %s", htmlEscapeCell(row.Error))
			case row.URL == "":
				fmt.Fprint(w, "<em>no image</em>")
			default:
				fmt.Fprintf(w, `<img alt=%q src=%q width="320">`,
					shotAltText(row.Label, row.Tail), row.URL)
			}
			fmt.Fprint(w, "</td>")
		}
		fmt.Fprintln(w, "</tr>")
	}
	fmt.Fprintln(w, `</table>`)
	fmt.Fprintln(w)
}

// htmlEscapeCell escapes the four characters that would break out of
// an HTML cell. Keeps tags the caller intentionally embedded (e.g.
// <br>) intact by only escaping bare `<` / `>` when not already part
// of an escape sequence; the only callers feed plain strings, so a
// naive pass is enough.
func htmlEscapeCell(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// errorShots stamps every row with the given message so a push
// failure surfaces inline rather than degrading to an unrenderable
// data URI. Keeps the captions and the row count, so the grid still
// reflects the cells the playbot produced screenshots for.
func errorShots(rows []shotRow, msg string) []pushedShot {
	out := make([]pushedShot, 0, len(rows))
	for _, r := range rows {
		out = append(out, pushedShot{
			Label: r.Label,
			Tail:  r.Tail,
			Error: msg,
		})
	}
	return out
}

// dataURIShots is the offline-mode fallback for renderShotsMarkdown:
// turn each raw PNG into a `data:image/png;base64,...` URL so the
// verb still produces a self-contained markdown document without
// hitting the GitHub API. The summary verb uses this when
// --shots-branch is unset (typical for local debugging).
func dataURIShots(rows []shotRow) []pushedShot {
	out := make([]pushedShot, 0, len(rows))
	for _, r := range rows {
		out = append(out, pushedShot{
			Label: r.Label,
			Tail:  r.Tail,
			URL:   "data:image/png;base64," + base64.StdEncoding.EncodeToString(r.PNG),
		})
	}
	return out
}
