package cli

// Shared terminal-output helpers. Every CLI table (platform matrix,
// toolchain list, toolchain doctor, ...) renders through here so width
// handling and column style stay consistent across verbs.
//
// Tables are borderless to match the historical tabwriter look, but
// width-aware: a table that fits the terminal renders at its natural
// compact width; one that would overflow is capped to the terminal and
// long cells wrap in place rather than garbling the layout.

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"golang.org/x/term"
)

// gutter is the inter-column spacing, matching the old
// tabwriter(_, 0, 0, 2, ' ', 0) padding of 2.
const gutter = 2

// ttyWidth returns the terminal width in columns, or 0 when stdout is
// not a terminal (piped, redirected, CI) — 0 signals "no width
// constraint", so tables render at natural width off a terminal, as the
// old tabwriter output did. The >40 floor ignores pathologically narrow
// sizes that would make wrapping useless.
func ttyWidth() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 40 {
		return w
	}
	return 0
}

// termWidth is ttyWidth with an 80-column fallback, for prose wrapping
// that always wants a concrete width even when piped.
func termWidth() int {
	if w := ttyWidth(); w > 0 {
		return w
	}
	return 80
}

// renderTable writes a borderless, width-aware table. Columns size to
// their content; if the natural width exceeds the terminal, the wide
// (prose) columns shrink and their cells wrap in place while narrow
// columns stay intact. Each row should carry one cell per header.
func renderTable(w io.Writer, headers []string, rows [][]string) {
	ncols := len(headers)
	if ncols == 0 {
		return
	}
	// Per column: colMax is the natural (longest cell) width; colMin is
	// the longest whitespace-delimited token — the narrowest the column
	// can get without breaking a word mid-token.
	colMax := make([]int, ncols)
	colMin := make([]int, ncols)
	consider := func(c int, s string) {
		if n := lipgloss.Width(s); n > colMax[c] {
			colMax[c] = n
		}
		for _, tok := range strings.Fields(s) {
			if n := lipgloss.Width(tok); n > colMin[c] {
				colMin[c] = n
			}
		}
	}
	for c, h := range headers {
		consider(c, h)
	}
	for _, r := range rows {
		for c := 0; c < ncols && c < len(r); c++ {
			consider(c, r[c])
		}
	}
	for c := range colMin {
		if colMin[c] < 1 {
			colMin[c] = 1
		}
		if colMin[c] > colMax[c] {
			colMin[c] = colMax[c]
		}
	}

	// Content widths. Off a terminal, or when the natural layout fits,
	// use full content widths; otherwise shrink to the terminal budget.
	widths := colMax
	if tw := ttyWidth(); tw > 0 {
		widths = fitColumns(colMax, colMin, tw-gutter*(ncols-1))
	}

	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderTop(false).BorderBottom(false).BorderLeft(false).
		BorderRight(false).BorderColumn(false).BorderRow(false).
		BorderHeader(false).
		StyleFunc(func(_, col int) lipgloss.Style {
			// Fixed per-column content width + a trailing gutter on all
			// but the last column. Width in lipgloss includes padding, so
			// add the gutter into the frame width.
			if col < ncols-1 {
				return lipgloss.NewStyle().Width(widths[col] + gutter).PaddingRight(gutter)
			}
			return lipgloss.NewStyle().Width(widths[col])
		}).
		Headers(headers...)
	for _, r := range rows {
		t.Row(r...)
	}
	writeTrimmed(w, t.Render())
}

// fitColumns allocates content widths within budget columns. It starts
// from the natural widths and, while over budget, trims one cell off
// whichever column is currently widest but still above its per-column
// minimum (longest word) — so narrow columns stay intact and the wide
// prose columns absorb the loss. If the minimums alone exceed the
// budget, it keeps trimming the widest regardless (accepting mid-word
// breaks) down to a floor of 3, which is the unavoidable case of a
// terminal too narrow for the data.
func fitColumns(colMax, colMin []int, budget int) []int {
	w := make([]int, len(colMax))
	copy(w, colMax)
	total := 0
	for _, x := range w {
		total += x
	}
	trimWidest := func(floor func(i int) int) bool {
		idx := -1
		for i := range w {
			if w[i] > floor(i) && (idx == -1 || w[i] > w[idx]) {
				idx = i
			}
		}
		if idx == -1 {
			return false
		}
		w[idx]--
		total--
		return true
	}
	for total > budget && trimWidest(func(i int) int { return colMin[i] }) {
	}
	for total > budget && trimWidest(func(int) int { return 3 }) {
	}
	return w
}

// writeTrimmed writes s a line at a time, stripping trailing spaces —
// lipgloss pads every cell (including the last column) to its width, so
// without this each row would carry ragged trailing whitespace and
// break golden comparisons / grep.
func writeTrimmed(w io.Writer, s string) {
	for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		fmt.Fprintln(w, strings.TrimRight(line, " "))
	}
}

// renderKeyValue writes an aligned "label  value" block: labels padded
// to a common width, values wrapped to the remaining terminal width so
// long fields (notes, paths) stay readable on narrow terminals. Used
// for single-record detail and the --vertical table form. Each label
// should already carry its trailing colon.
func renderKeyValue(w io.Writer, pairs [][2]string) {
	keyMax := 0
	for _, p := range pairs {
		if n := lipgloss.Width(p[0]); n > keyMax {
			keyMax = n
		}
	}
	keyStyle := lipgloss.NewStyle().Width(keyMax + gutter)
	// Off a terminal, don't wrap values — keep single-line records so
	// piped/CI output stays grep-friendly.
	tw := ttyWidth()
	for _, p := range pairs {
		if tw == 0 {
			writeTrimmed(w, keyStyle.Render(p[0])+p[1])
			continue
		}
		valWidth := tw - keyMax - gutter
		if valWidth < 20 {
			valWidth = 20
		}
		writeTrimmed(w, lipgloss.JoinHorizontal(lipgloss.Top,
			keyStyle.Render(p[0]), lipgloss.NewStyle().Width(valWidth).Render(p[1])))
	}
}
