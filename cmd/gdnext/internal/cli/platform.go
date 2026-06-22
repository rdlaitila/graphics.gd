package cli

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"runtime"
	"strings"
	"text/tabwriter"

	"graphics.gd/product"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
	"gopkg.in/yaml.v3"
)

// PlatformCommand exposes `gdnext platform`: show the graphics.gd
// platform / host / target matrix.
type PlatformCommand struct {
	*cli.Command
}

// NewPlatformCommand constructs the `gdnext platform` subcommand
func NewPlatformCommand(di do.Injector) (*PlatformCommand, error) {
	t := do.MustInvokeStruct[*PlatformCommand](di)
	t.Command = &cli.Command{
		Name:      "platform",
		Usage:     "show the graphics.gd platform / host / target matrix",
		ArgsUsage: "[<name>]",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "hosts",
				Usage: "show only platforms that can run gdnext (build hosts)",
			},
			&cli.BoolFlag{
				Name:  "targets",
				Usage: "show only platforms gdnext can build for",
			},
			&cli.StringFlag{
				Name:    "format",
				Aliases: []string{"f"},
				Value:   "table",
				Usage:   "output format: table | json | yaml | xml | markdown",
			},
			&cli.BoolFlag{
				Name:    "vertical",
				Aliases: []string{"l"},
				Usage:   "render one field per line per row (useful when the table is too wide)",
			},
		},
		Action: t.platform,
	}
	return t, nil
}

func (t *PlatformCommand) platform(_ context.Context, cmd *cli.Command) error {
	format := strings.ToLower(cmd.String("format"))
	vertical := cmd.Bool("vertical")
	hostsOnly := cmd.Bool("hosts")
	targetsOnly := cmd.Bool("targets")
	if hostsOnly && targetsOnly {
		return fmt.Errorf("--hosts and --targets are mutually exclusive")
	}
	// Resolve the row set or single row.
	var (
		rows   []product.Platform
		single bool
		title  string
	)
	switch {
	case cmd.NArg() > 0:
		arg := strings.ToLower(cmd.Args().First())
		p, ok := product.FindPlatformByName(arg)
		if !ok {
			return fmt.Errorf("unknown platform %q (try `gdnext platform` for the full list)", arg)
		}
		rows = []product.Platform{p}
		single = true
		title = "platform " + p.Tuple()
	case hostsOnly:
		rows = product.Hosts()
		title = "hosts"
	case targetsOnly:
		rows = product.Targets()
		title = "targets"
	default:
		rows = product.PlatformMatrix
		title = "all platforms"
	}
	switch format {
	case "table":
		if single {
			printDetail(rows[0])
			return nil
		}
		if vertical {
			printVertical(title, rows)
			return nil
		}
		printTable(title, rows)
		return nil
	case "json":
		return printJSON(rows, single)
	case "yaml", "yml":
		return printYAML(rows, single)
	case "xml":
		return printXML(rows, single)
	case "markdown", "md":
		if vertical {
			printMarkdownVertical(title, rows)
			return nil
		}
		printMarkdown(title, rows)
		return nil
	default:
		return fmt.Errorf("unknown --format %q (want one of: table, json, yaml, xml, markdown)", format)
	}
}

// printTable renders rows in the same tabwriter style as `gdnext
// toolchain doctor` so both commands read consistently.
func printTable(title string, rows []product.Platform) {
	fmt.Fprintf(os.Stdout, "graphics.gd %s (host: %s)\n\n",
		title, product.Tuple(runtime.GOOS, runtime.GOARCH))
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer tw.Flush()
	fmt.Fprintln(tw, "PLATFORM\tKIND\tSTATUS\tLINK\tALIASES\tNOTES")
	for _, p := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			p.Tuple(), p.Kind, p.Status,
			linkModeOrDash(p.LinkModes),
			joinOrDash(p.Aliases),
			p.Notes,
		)
	}
}

// printDetail renders a single platform's full record. Used by
// `gdnext platforms <name>` in the default (table) format.
func printDetail(p product.Platform) {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer tw.Flush()
	fmt.Fprintf(tw, "title:\t%s\n", p.DisplayTitle())
	fmt.Fprintf(tw, "platform:\t%s\n", p.Tuple())
	fmt.Fprintf(tw, "aliases:\t%s\n", joinOrDash(p.Aliases))
	fmt.Fprintf(tw, "kind:\t%s\n", p.Kind)
	fmt.Fprintf(tw, "status:\t%s\n", p.Status)
	fmt.Fprintf(tw, "renderers:\t%s\n", joinOrDash(p.Renderers))
	if p.Notes != "" {
		fmt.Fprintf(tw, "notes:\t%s\n", p.Notes)
	}
}

// printJSON renders rows as indented JSON. A single platform encodes as
// an object; everything else as an array — so machine consumers know
// what shape to expect from the call they made.
func printJSON(rows []product.Platform, single bool) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if single {
		return enc.Encode(rows[0])
	}
	return enc.Encode(rows)
}

// printYAML mirrors printJSON's shape rules (single platform → mapping,
// otherwise sequence). yaml.v3 honours the same json struct tags via
// fallback for fields lacking explicit yaml tags, and Kind/Status
// flow through their MarshalText implementations, so the output
// matches printJSON's field set without per-tag bookkeeping.
func printYAML(rows []product.Platform, single bool) error {
	enc := yaml.NewEncoder(os.Stdout)
	enc.SetIndent(2)
	defer enc.Close()
	if single {
		return enc.Encode(rows[0])
	}
	return enc.Encode(rows)
}

// printXML wraps the rows in a <platforms> root element so the document
// is well-formed (XML requires exactly one root). Single-row queries
// produce the same root with one child for shape consistency.
func printXML(rows []product.Platform, _ bool) error {
	fmt.Fprintln(os.Stdout, xml.Header[:len(xml.Header)-1])
	enc := xml.NewEncoder(os.Stdout)
	enc.Indent("", "  ")
	type doc struct {
		XMLName  xml.Name           `xml:"platforms"`
		Platform []product.Platform `xml:"platform"`
	}
	if err := enc.Encode(doc{Platform: rows}); err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout)
	return nil
}

// printVertical is the --vertical form of printTable: instead of one
// row per platform with N field columns, render each row as a
// sequence of `field: value` lines separated by blank lines. Useful
// when the horizontal table would wrap (long alias / notes columns,
// narrow terminals).
func printVertical(title string, rows []product.Platform) {
	fmt.Fprintf(os.Stdout, "graphics.gd %s (host: %s)\n\n",
		title, product.Tuple(runtime.GOOS, runtime.GOARCH))
	for i, p := range rows {
		if i > 0 {
			fmt.Fprintln(os.Stdout)
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintf(tw, "title:\t%s\n", p.DisplayTitle())
		fmt.Fprintf(tw, "platform:\t%s\n", p.Tuple())
		fmt.Fprintf(tw, "kind:\t%s\n", p.Kind)
		fmt.Fprintf(tw, "status:\t%s\n", p.Status)
		fmt.Fprintf(tw, "aliases:\t%s\n", joinOrDash(p.Aliases))
		fmt.Fprintf(tw, "renderers:\t%s\n", joinOrDash(p.Renderers))
		if p.Notes != "" {
			fmt.Fprintf(tw, "notes:\t%s\n", p.Notes)
		}
		tw.Flush()
	}
}

// printMarkdown renders a GitHub-flavoured Markdown table. Pipes inside
// notes are escaped so they don't break the cell structure.
func printMarkdown(title string, rows []product.Platform) {
	fmt.Fprintf(os.Stdout, "# graphics.gd %s\n\n", title)
	fmt.Fprintln(os.Stdout, "| Platform | Kind | Status | Aliases | Renderers | Notes |")
	fmt.Fprintln(os.Stdout, "|----------|------|--------|---------|-----------|-------|")
	for _, p := range rows {
		fmt.Fprintf(os.Stdout, "| %s | %s | %s | %s | %s | %s |\n",
			p.Tuple(), p.Kind, p.Status,
			joinOrDashMD(p.Aliases),
			joinOrDashMD(p.Renderers),
			escapeMD(p.Notes),
		)
	}
}

func joinOrDash(s []string) string {
	if len(s) == 0 {
		return "-"
	}
	return strings.Join(s, ", ")
}

func linkModeOrDash(m product.LinkMode) string {
	if m == 0 {
		return "-"
	}
	return m.String()
}

// printMarkdownVertical is the --vertical form of printMarkdown:
// renders one two-column `Field | Value` table per platform instead
// of a single wide-column table. Reads well on narrow GitHub renders
// where the regular wide table would overflow.
func printMarkdownVertical(title string, rows []product.Platform) {
	fmt.Fprintf(os.Stdout, "# graphics.gd %s\n\n", title)
	for i, p := range rows {
		if i > 0 {
			fmt.Fprintln(os.Stdout)
		}
		fmt.Fprintf(os.Stdout, "## %s\n\n", p.DisplayTitle())
		fmt.Fprintln(os.Stdout, "| Field | Value |")
		fmt.Fprintln(os.Stdout, "|-------|-------|")
		fmt.Fprintf(os.Stdout, "| Platform | %s |\n", p.Tuple())
		fmt.Fprintf(os.Stdout, "| Kind | %s |\n", p.Kind)
		fmt.Fprintf(os.Stdout, "| Status | %s |\n", p.Status)
		fmt.Fprintf(os.Stdout, "| Aliases | %s |\n", joinOrDashMD(p.Aliases))
		fmt.Fprintf(os.Stdout, "| Renderers | %s |\n", joinOrDashMD(p.Renderers))
		fmt.Fprintf(os.Stdout, "| Notes | %s |\n", escapeMD(p.Notes))
	}
}

func joinOrDashMD(s []string) string {
	if len(s) == 0 {
		return "—"
	}
	return strings.Join(s, ", ")
}

// escapeMD makes a string safe for inclusion in a Markdown table cell:
// pipes terminate cells, newlines break rows.
func escapeMD(s string) string {
	if s == "" {
		return "—"
	}
	s = strings.ReplaceAll(s, "|", `\|`)
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
