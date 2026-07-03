package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"graphics.gd/cmd/gdnext/internal/shared"
	"graphics.gd/cmd/gdnext/internal/tooling"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
)

// DocCommand wires `gdnext doc`. Runtime state lives on *DocActions.
type DocCommand struct {
	*cli.Command
	Injector do.Injector `do:""`
}

// DocActions carries the runtime state for doc lookups.
type DocActions struct {
	ToolCatalog tooling.Catalog `do:""`
}

type gdDocMatch struct {
	gdTag     string
	goDocPath string
}

// NewDocCommand constructs the `gdnext doc` subcommand
func NewDocCommand(di do.Injector) (*DocCommand, error) {
	t := do.MustInvokeStruct[*DocCommand](di)
	t.Command = &cli.Command{
		Name:            "doc",
		Usage:           "go doc with //gd: tag lookup against classdb",
		ArgsUsage:       "[symbol] [args...]",
		SkipFlagParsing: true,
		Action:          shared.BindAction(t.Injector, (*DocActions).doc),
	}
	return t, nil
}

// NewDocActions resolves the runtime state for doc.
func NewDocActions(di do.Injector) (*DocActions, error) {
	return do.InvokeStruct[*DocActions](di)
}

func (t *DocActions) doc(_ context.Context, cmd *cli.Command) error {
	if helpRequested(cmd) {
		return cli.ShowSubcommandHelp(cmd)
	}
	args := cmd.Args().Slice()
	if len(args) == 0 {
		return t.ToolCatalog.Go.Exec("doc")
	}
	query := args[0]
	remaining := args[1:]
	matches, err := t.findGdDocMatches(query)
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		return t.ToolCatalog.Go.Exec(append([]string{"doc"}, args...)...)
	}
	var lastErr error
	var failures int
	for i, match := range matches {
		if i > 0 {
			fmt.Println()
			fmt.Println("---")
			fmt.Println()
		}
		docArgs := append([]string{"doc", match.goDocPath}, remaining...)
		if err := t.ToolCatalog.Go.Exec(docArgs...); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not get doc for %s: %v\n", match.goDocPath, err)
			lastErr = err
			failures++
		}
	}
	if failures == len(matches) {
		return lastErr
	}
	return nil
}

func (t *DocActions) findGdDocMatches(query string) ([]gdDocMatch, error) {
	goPath, err := t.ToolCatalog.Go.Lookup()
	if err != nil {
		return nil, err
	}
	modRoot, err := shared.OutputBytes(goPath, "list", "-m", "-f", "{{.Dir}}", "graphics.gd")
	if err != nil {
		return nil, fmt.Errorf("could not find graphics.gd module: %w", err)
	}
	classdbDir := filepath.Join(strings.TrimSpace(string(modRoot)), "classdb")
	var matches []gdDocMatch
	entries, err := os.ReadDir(classdbDir)
	if err != nil {
		return nil, fmt.Errorf("could not read classdb directory: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		classFile := filepath.Join(classdbDir, entry.Name(), "class.go")
		file, err := os.Open(classFile)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "(self class)") {
				continue
			}
			_, gdTag, ok := strings.Cut(line, "//gd:")
			if !ok {
				continue
			}
			parts := strings.SplitN(gdTag, ".", 2)
			if len(parts) != 2 {
				continue
			}
			className := parts[0]
			methodName := parts[1]
			if methodName != query && gdTag != query {
				continue
			}
			goMethodName := snakeToPascal(methodName)
			goDocPath := fmt.Sprintf("graphics.gd/classdb/%s.%s", className, goMethodName)
			matches = append(matches, gdDocMatch{gdTag: gdTag, goDocPath: goDocPath})
		}
		file.Close()
	}
	return matches, nil
}

func snakeToPascal(s string) string {
	parts := strings.Split(s, "_")
	for i := range parts {
		if len(parts[i]) > 0 {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}
