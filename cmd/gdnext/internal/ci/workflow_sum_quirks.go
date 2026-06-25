package ci

import (
	"fmt"
	"io"

	"graphics.gd/product"
)

func renderQuirksMarkdown(w io.Writer) {
	entries := product.KnownQuirks()
	fmt.Fprintln(w, `<h2 id="known-quirks">Known quirks</h2>`)
	fmt.Fprintln(w)
	if len(entries) == 0 {
		fmt.Fprintln(w, "_None declared._")
		fmt.Fprintln(w)
		return
	}
	fmt.Fprintf(w, "**%d** quirk(s) recorded in `product.PlatformMatrix`. Expand each for the symptom, what we tried, and how the matrix routes around it.\n\n", len(entries))
	for _, e := range entries {
		fmt.Fprintln(w, "<details>")
		title := fmt.Sprintf("[<code>%s</code>] %s", htmlEscape(e.Quirk.Scope.String()), htmlEscape(e.Quirk.Title))
		fmt.Fprintf(w, "<summary><b>%s</b></summary>\n\n", title)
		fmt.Fprintf(w, "- **platforms:** %s\n", joinCodeList(e.Platforms))
		if len(e.Quirk.Hosts) > 0 {
			fmt.Fprintf(w, "- **hosts:** %s\n", joinCodeList(e.Quirk.Hosts))
		} else {
			fmt.Fprintln(w, "- **hosts:** (every build host)")
		}
		fmt.Fprintf(w, "- **reason:** %s\n", htmlEscape(e.Quirk.Reason))
		if len(e.Quirk.Result) > 0 {
			fmt.Fprintln(w, "- **result:**")
			for _, r := range e.Quirk.Result {
				fmt.Fprintf(w, "  - %s\n", htmlEscape(r))
			}
		}
		if len(e.Quirk.Refs) > 0 {
			fmt.Fprintln(w, "- **refs:**")
			for _, r := range e.Quirk.Refs {
				fmt.Fprintf(w, "  - <%s>\n", r)
			}
		}
		fmt.Fprintln(w)
		fmt.Fprintln(w, "</details>")
		fmt.Fprintln(w)
	}
}

func joinCodeList(items []string) string {
	out := ""
	for i, s := range items {
		if i > 0 {
			out += ", "
		}
		out += "`" + s + "`"
	}
	return out
}
