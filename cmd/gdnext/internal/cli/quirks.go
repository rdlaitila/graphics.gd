package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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

type QuirksActions struct{}

func NewQuirksCommand(di do.Injector) (*QuirksCommand, error) {
	t := do.MustInvokeStruct[*QuirksCommand](di)
	t.Command = &cli.Command{
		Name:  "quirks",
		Usage: "list every known platform quirk (CI gates, runtime caveats)",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "format",
				Aliases: []string{"f"},
				Value:   "table",
				Usage:   "output format: table | json",
			},
			&cli.IntFlag{
				Name:  "width",
				Value: 0,
				Usage: "wrap reason text at N columns (0 = auto-detect terminal, fall back to 80); table format only",
			},
		},
		Action: shared.BindAction(t.Injector, (*QuirksActions).action),
	}
	return t, nil
}

func NewQuirksActions(di do.Injector) (*QuirksActions, error) {
	return do.InvokeStruct[*QuirksActions](di)
}

func (t *QuirksActions) action(_ context.Context, cmd *cli.Command) error {
	entries := product.KnownQuirks()
	switch strings.ToLower(cmd.String("format")) {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(entries)
	case "", "table":
	default:
		return fmt.Errorf("unknown --format %q (want: table | json)", cmd.String("format"))
	}
	width := cmd.Int("width")
	if width <= 0 {
		width = detectWidth()
	}
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

// printQuirk renders one entry to stdout, wrapping reason / result / refs at width.
func printQuirk(e product.QuirkEntry, width int) {
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
