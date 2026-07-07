package cli

import (
	"bytes"
	"strings"
	"testing"
)

func sum(xs []int) int {
	t := 0
	for _, x := range xs {
		t += x
	}
	return t
}

func TestFitColumnsFitsUnchanged(t *testing.T) {
	max := []int{5, 10, 4}
	min := []int{3, 4, 2}
	got := fitColumns(max, min, 100)
	for i := range max {
		if got[i] != max[i] {
			t.Fatalf("col %d: within budget should keep natural width %d, got %d", i, max[i], got[i])
		}
	}
}

func TestFitColumnsShrinksWidestKeepsNarrow(t *testing.T) {
	// PLATFORM-like narrow col plus a wide prose col; budget forces the
	// wide one to give up width while the narrow one stays intact.
	max := []int{13, 60} // natural 73
	min := []int{13, 8}
	got := fitColumns(max, min, 40)
	if got[0] != 13 {
		t.Errorf("narrow column should stay at its natural/min width 13, got %d", got[0])
	}
	if got[1] != 27 { // 40 - 13
		t.Errorf("wide column should absorb the shrink to 27, got %d", got[1])
	}
	if sum(got) != 40 {
		t.Errorf("total should equal budget 40, got %d", sum(got))
	}
}

func TestFitColumnsRespectsMinThenHardBreaks(t *testing.T) {
	// Minimums (10+10) exceed the budget (12): first pass floors both at
	// min, second pass hard-breaks the widest down toward the floor.
	max := []int{20, 20}
	min := []int{10, 10}
	got := fitColumns(max, min, 12)
	if sum(got) != 12 {
		t.Errorf("second pass should drive total to budget 12, got %d (%v)", sum(got), got)
	}
	for i, w := range got {
		if w < 3 {
			t.Errorf("col %d fell below floor 3: %d", i, w)
		}
	}
}

func TestRenderTableNoTrailingWhitespace(t *testing.T) {
	// Off a terminal (test has no tty) columns render at natural width;
	// every line must be free of trailing spaces regardless.
	var buf bytes.Buffer
	renderTable(&buf,
		[]string{"A", "B"},
		[][]string{{"x", "long value here"}, {"yy", "z"}})
	for _, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		if line != strings.TrimRight(line, " ") {
			t.Errorf("line has trailing whitespace: %q", line)
		}
	}
}

func TestRenderKeyValueAligns(t *testing.T) {
	var buf bytes.Buffer
	renderKeyValue(&buf, [][2]string{{"short:", "a"}, {"longlabel:", "b"}})
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d", len(lines))
	}
	// Both labels pad to len("longlabel:")+gutter, so each single-char
	// value lands in the final column of an equal-length line.
	if len(lines[0]) != len(lines[1]) {
		t.Errorf("rows not aligned to equal width:\n%q\n%q", lines[0], lines[1])
	}
	if !strings.HasSuffix(lines[0], "a") || !strings.HasSuffix(lines[1], "b") {
		t.Errorf("values missing/misplaced:\n%q\n%q", lines[0], lines[1])
	}
}
