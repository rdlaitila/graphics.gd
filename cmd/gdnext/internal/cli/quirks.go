package cli

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"graphics.gd/cmd/gdnext/internal/shared"
	"graphics.gd/product"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
	"golang.org/x/term"
)

// QuirksCommand wires `gdnext quirks`.
type QuirksCommand struct {
	*cli.Command
	Injector do.Injector `do:""`
}

// QuirksActions carries the runtime state.
type QuirksActions struct{}

// NewQuirksCommand constructs the `gdnext quirks` subcommand.
func NewQuirksCommand(di do.Injector) (*QuirksCommand, error) {
	t := do.MustInvokeStruct[*QuirksCommand](di)
	t.Command = &cli.Command{
		Name:  "quirks",
		Usage: "list every known platform quirk (CI gates, runtime caveats)",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:  "width",
				Value: 0,
				Usage: "wrap reason text at N columns (0 = auto-detect terminal, fall back to 80)",
			},
		},
		Action: shared.BindAction(t.Injector, (*QuirksActions).action),
	}
	return t, nil
}

// NewQuirksActions resolves the runtime state.
func NewQuirksActions(di do.Injector) (*QuirksActions, error) {
	return do.InvokeStruct[*QuirksActions](di)
}

func (t *QuirksActions) action(_ context.Context, cmd *cli.Command) error {
	width := cmd.Int("width")
	if width <= 0 {
		width = detectWidth()
	}
	entries := collectQuirks()
	if len(entries) == 0 {
		fmt.Println("no quirks declared")
		return nil
	}
	for i, e := range entries {
		if i > 0 {
			fmt.Println()
		}
		printQuirk(e, width)
	}
	return nil
}

// quirkEntry pairs a Quirk with the platforms it is attached to.
// Quirks are de-duplicated by Title so a single QuirkBuild* var shared
// across multiple Platform rows renders once.
type quirkEntry struct {
	Quirk     product.Quirk
	Platforms []string
}

func collectQuirks() []quirkEntry {
	by := map[string]*quirkEntry{}
	var order []string
	for _, p := range product.PlatformMatrix {
		for _, q := range p.Quirks {
			e, ok := by[q.Title]
			if !ok {
				e = &quirkEntry{Quirk: q}
				by[q.Title] = e
				order = append(order, q.Title)
			}
			e.Platforms = append(e.Platforms, p.Tuple())
		}
	}
	out := make([]quirkEntry, 0, len(order))
	for _, title := range order {
		e := by[title]
		sort.Strings(e.Platforms)
		out = append(out, *e)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Quirk.Scope != out[j].Quirk.Scope {
			return out[i].Quirk.Scope > out[j].Quirk.Scope
		}
		return out[i].Quirk.Title < out[j].Quirk.Title
	})
	return out
}

// printQuirk renders one entry to stdout, wrapping reason / result / refs at width.
func printQuirk(e quirkEntry, width int) {
	fmt.Printf("[%s] %s\n", e.Quirk.Scope, e.Quirk.Title)
	indent := "  "
	printField(indent, "platforms", strings.Join(e.Platforms, ", "), width)
	if len(e.Quirk.Hosts) > 0 {
		printField(indent, "hosts", strings.Join(e.Quirk.Hosts, ", "), width)
	} else {
		printField(indent, "hosts", "(every build host)", width)
	}
	printField(indent, "reason", e.Quirk.Reason, width)
	for i, r := range e.Quirk.Result {
		label := "result"
		if i > 0 {
			label = ""
		}
		printField(indent, label, "- "+r, width)
	}
	for i, ref := range e.Quirk.Refs {
		label := "refs"
		if i > 0 {
			label = ""
		}
		printField(indent, label, ref, width)
	}
}

// printField writes "<indent><label>: <value>" with subsequent wrapped
// lines hanging under the value column. label may be empty for
// continuation entries (e.g. repeated refs).
func printField(indent, label, value string, width int) {
	const labelCol = 11 // longest field name ("platforms") + ": "
	var prefix string
	if label == "" {
		prefix = indent + strings.Repeat(" ", labelCol)
	} else {
		prefix = fmt.Sprintf("%s%-*s", indent, labelCol, label+":")
	}
	hang := indent + strings.Repeat(" ", labelCol)
	avail := width - len(prefix)
	if avail < 20 {
		avail = 20
	}
	lines := wrap(value, avail)
	for i, l := range lines {
		if i == 0 {
			fmt.Println(prefix + l)
		} else {
			fmt.Println(hang + l)
		}
	}
}

func wrap(s string, width int) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	cur := words[0]
	for _, w := range words[1:] {
		if len(cur)+1+len(w) > width {
			lines = append(lines, cur)
			cur = w
			continue
		}
		cur += " " + w
	}
	lines = append(lines, cur)
	return lines
}

func detectWidth() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 40 {
		return w
	}
	return 80
}
