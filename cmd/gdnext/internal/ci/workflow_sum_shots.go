package ci

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// shotRow is one cell's screenshot picked up from the downloaded
// `shot-*/play-screenshot.png` artefact tree. The PNG bytes are
// loaded eagerly so the renderer can inline them as `data:` URIs in
// the markdown step summary — GitHub doesn't host arbitrary artefact
// images for inline display.
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
// Shape: `shot-<play-runner>-play-<build-runner>-<example>-<goos>-<goarch>[-<link>]`
// (the embedded `play-` delimiter mirrors ArtifactName's build-cell
// prefix, which lets parseScreenshotArtifactName split the two halves
// cleanly without a second token-counting heuristic).
func ScreenshotArtifactName(playRunner, buildRunner, example, target, link string) string {
	return "shot-" + playRunner + "-" + ArtifactName(buildRunner, example, target, link)
}

// parseScreenshotArtifactName recovers (target, link, buildRunner, playRunner)
// from a name produced by ScreenshotArtifactName. Returns ok=false when
// the name doesn't match the expected shape so collectShots can fall
// back to the raw token list rather than mislabelling the cell.
func parseScreenshotArtifactName(name string) (target, link, buildRunner, playRunner string, ok bool) {
	rest, found := strings.CutPrefix(name, "shot-")
	if !found {
		return "", "", "", "", false
	}
	head, buildRest, found := strings.Cut(rest, "-play-")
	if !found || head == "" {
		return "", "", "", "", false
	}
	// buildRest = `<build-runner>-<example>-<goos>-<goarch>[-<link>]`.
	// Runner labels are `<os>-latest`; split off the first two tokens.
	parts := strings.SplitN(buildRest, "-", 3)
	if len(parts) < 3 || !strings.HasSuffix(parts[1], "latest") {
		return "", "", "", "", false
	}
	buildRunner = parts[0] + "-" + parts[1]
	tail := parts[2]
	// tail = `<example>-<goos>-<goarch>[-<link>]`. The link token, when
	// present, is one of the well-known LinkMode names; sniff that
	// first so the goos/goarch pair always sits at a fixed offset.
	tokens := strings.Split(tail, "-")
	if len(tokens) < 3 {
		return "", "", "", "", false
	}
	last := tokens[len(tokens)-1]
	if last == "gdextension" || last == "libgodot" {
		link = last
		tokens = tokens[:len(tokens)-1]
	}
	if len(tokens) < 3 {
		return "", "", "", "", false
	}
	archIdx := len(tokens) - 1
	goosIdx := archIdx - 1
	target = tokens[goosIdx] + "/" + tokens[archIdx]
	return target, link, buildRunner, head, true
}

// collectShots walks dir for artefact subdirectories named
// `shot-<play-runner>-play-<build-runner>-<example>-<goos>-<goarch>[-<link>]`
// and returns one row per cell whose play-screenshot.png is present
// and non-empty. Returns nil when dir is unset or empty so the
// renderer can short-circuit.
//
// Each cell uploads under a unique `shot-...` artefact name; the
// summary job's `actions/download-artifact` with `pattern: shot-*`
// materialises each as a sibling directory under the shots/ path
// passed via --shots.
func collectShots(dir string) []shotRow {
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []shotRow
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "shot-") {
			continue
		}
		shot := filepath.Join(dir, name, "play-screenshot.png")
		body, err := os.ReadFile(shot)
		if err != nil || len(body) == 0 {
			continue
		}
		label, tail := shotLabel(name)
		out = append(out, shotRow{Label: label, Tail: tail, PNG: body})
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
	target, link, buildRunner, playRunner, ok := parseScreenshotArtifactName(name)
	if !ok {
		return strings.TrimPrefix(name, "shot-"), ""
	}
	head = target
	if link != "" {
		head += "+" + link
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
// the same section. Each cell embeds an `<img src="<URL>">` when the
// upload succeeded; rows whose Error is set print the message
// instead so push failures stay visible. Skipped entirely when no
// shots were collected.
func renderShotsMarkdown(w io.Writer, rows []pushedShot) {
	if len(rows) == 0 {
		return
	}
	const cols = 3
	fmt.Fprintln(w, strings.Repeat("| ", cols)+"|")
	fmt.Fprintln(w, strings.Repeat("| --- ", cols)+"|")
	for i := 0; i < len(rows); i += cols {
		// caption row — head on its own line, tail on the next via
		// inline <br> so the column stays narrow.
		for c := 0; c < cols; c++ {
			if i+c < len(rows) {
				row := rows[i+c]
				caption := escapeMDCell(row.Label)
				if row.Tail != "" {
					caption += "<br>" + escapeMDCell(row.Tail)
				}
				fmt.Fprintf(w, "| **%s** ", caption)
			} else {
				fmt.Fprint(w, "|  ")
			}
		}
		fmt.Fprintln(w, "|")
		// image row
		for c := 0; c < cols; c++ {
			if i+c >= len(rows) {
				fmt.Fprint(w, "|  ")
				continue
			}
			row := rows[i+c]
			switch {
			case row.Error != "":
				fmt.Fprintf(w, "| ⚠️ %s ", escapeMDCell(row.Error))
			case row.URL == "":
				fmt.Fprint(w, "| _no image_ ")
			default:
				fmt.Fprintf(w, "| <img alt=%q src=%q width=\"320\"> ",
					shotAltText(row.Label, row.Tail), row.URL)
			}
		}
		fmt.Fprintln(w, "|")
	}
	fmt.Fprintln(w)
}

// escapeMDCell escapes characters that would break out of a GFM
// table cell. Pipes are the only structural one; newlines are
// stripped (a `<br>` placed by the caller is what wraps within a
// cell).
func escapeMDCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
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
