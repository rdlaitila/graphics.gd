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
type shotRow struct {
	Label string
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

// parseScreenshotArtifactName recovers (target, buildRunner, playRunner)
// from a name produced by ScreenshotArtifactName. Returns ok=false when
// the name doesn't match the expected shape so collectShots can fall
// back to the raw token list rather than mislabelling the cell.
func parseScreenshotArtifactName(name string) (target, buildRunner, playRunner string, ok bool) {
	rest, found := strings.CutPrefix(name, "shot-")
	if !found {
		return "", "", "", false
	}
	head, buildRest, found := strings.Cut(rest, "-play-")
	if !found || head == "" {
		return "", "", "", false
	}
	// buildRest = `<build-runner>-<example>-<goos>-<goarch>[-<link>]`.
	// Runner labels are `<os>-latest`; split off the first two tokens.
	parts := strings.SplitN(buildRest, "-", 3)
	if len(parts) < 3 || !strings.HasSuffix(parts[1], "latest") {
		return "", "", "", false
	}
	buildRunner = parts[0] + "-" + parts[1]
	tail := parts[2]
	// tail = `<example>-<goos>-<goarch>[-<link>]`. Walk tokens right-to-left
	// looking for the `<goos>-<goarch>` pair so the example name (which
	// may itself contain dashes) is whatever sits to its left.
	tokens := strings.Split(tail, "-")
	if len(tokens) < 3 {
		return "", "", "", false
	}
	// Try tail with a link suffix first, then without.
	for _, trim := range []int{1, 0} {
		if len(tokens) < 3+trim {
			continue
		}
		archIdx := len(tokens) - 1 - trim
		goosIdx := archIdx - 1
		if goosIdx < 1 {
			continue
		}
		target = tokens[goosIdx] + "/" + tokens[archIdx]
		return target, buildRunner, head, true
	}
	return "", "", "", false
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
		label := shotLabel(name)
		out = append(out, shotRow{Label: label, PNG: body})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return out
}

// shotLabel renders a `<target> (b: <build-host> p: <play-host>)`
// caption from an artefact directory name. Falls back to the raw
// trimmed name when parsing fails so unknown shapes still show up
// rather than disappearing silently.
func shotLabel(name string) string {
	target, buildRunner, playRunner, ok := parseScreenshotArtifactName(name)
	if !ok {
		return strings.TrimPrefix(name, "shot-")
	}
	return fmt.Sprintf("%s (b: %s p: %s)", target, buildRunner, playRunner)
}

// renderShotsMarkdown appends a screenshot grid directly under the
// preceding Plays table — no section header, so it reads as part of
// the same section. Each image is inlined as a `data:` URI so the
// summary stays self-contained (no signed artefact URLs, no host
// dependencies). GitHub's step-summary markdown renderer supports
// `<img>` and accepts `data:` sources. Skipped entirely when no
// shots were collected.
func renderShotsMarkdown(w io.Writer, rows []shotRow) {
	if len(rows) == 0 {
		return
	}
	const cols = 3
	fmt.Fprintln(w, strings.Repeat("| ", cols)+"|")
	fmt.Fprintln(w, strings.Repeat("| --- ", cols)+"|")
	for i := 0; i < len(rows); i += cols {
		// caption row
		for c := 0; c < cols; c++ {
			if i+c < len(rows) {
				fmt.Fprintf(w, "| **%s** ", strings.ReplaceAll(rows[i+c].Label, "|", "\\|"))
			} else {
				fmt.Fprint(w, "|  ")
			}
		}
		fmt.Fprintln(w, "|")
		// image row
		for c := 0; c < cols; c++ {
			if i+c < len(rows) {
				fmt.Fprintf(w, "| <img alt=%q src=\"data:image/png;base64,%s\" width=\"320\"> ",
					rows[i+c].Label,
					base64.StdEncoding.EncodeToString(rows[i+c].PNG))
			} else {
				fmt.Fprint(w, "|  ")
			}
		}
		fmt.Fprintln(w, "|")
	}
	fmt.Fprintln(w)
}
