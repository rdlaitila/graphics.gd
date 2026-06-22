package cli

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"graphics.gd/cmd/gdnext/internal/tooling"
	"graphics.gd/product"

	"github.com/samber/do/v2"
	"graphics.gd/cmd/gdnext/internal/shared"
	"github.com/urfave/cli/v3"
	"gopkg.in/yaml.v3"
)

// ToolchainCommand wires `gdnext toolchain`. Runtime state lives on
// *ToolchainActions.
type ToolchainCommand struct {
	*cli.Command
	Injector do.Injector `do:""`
}

// ToolchainActions carries the runtime state.
type ToolchainActions struct {
	BuildEnv    product.BuildEnv `do:""`
	ToolCatalog tooling.Catalog  `do:""`
}

// NewToolchainCommand constructs the `gdnext toolchain` subcommand
func NewToolchainCommand(di do.Injector) (*ToolchainCommand, error) {
	t := do.MustInvokeStruct[*ToolchainCommand](di)
	t.Command = &cli.Command{
		Name:  "toolchain",
		Usage: "manage the external programs gdnext drives",
		Commands: []*cli.Command{
			{
				Name:   "list",
				Usage:  "list every toolchain gdnext can manage",
				Action: shared.BindAction(t.Injector, (*ToolchainActions).list),
			},
			{
				Name:      "path",
				Usage:     "print the absolute install path of a toolchain (lookup only, no download)",
				ArgsUsage: "<name>",
				Action:    shared.BindAction(t.Injector, (*ToolchainActions).path),
			},
			{
				Name:      "install",
				Usage:     "install one named toolchain, or every tool needed by any target buildable from this host",
				ArgsUsage: "[name]",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "skip-checksum",
						Usage: "skip product.Toolchain.KnownChecksums verification after download (sets GDNEXT_SKIP_CHECKSUM=1)",
					},
				},
				Action: shared.BindAction(t.Injector, (*ToolchainActions).install),
			},
			{
				Name:  "doctor",
				Usage: "report toolchain status for every target buildable from this host; --fix runs install",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "fix",
						Usage: "after reporting, run `toolchain install` and re-report",
					},
					&cli.BoolFlag{
						Name:  "skip-checksum",
						Usage: "skip checksum verification when --fix runs install",
					},
					&cli.StringFlag{
						Name:    "format",
						Aliases: []string{"f"},
						Value:   "table",
						Usage:   "output format: table | json | yaml | xml",
					},
				},
				Action: shared.BindAction(t.Injector, (*ToolchainActions).doctor),
			},
		},
	}
	return t, nil
}

// NewToolchainActions resolves the runtime state for toolchain.
func NewToolchainActions(di do.Injector) (*ToolchainActions, error) {
	return do.InvokeStruct[*ToolchainActions](di)
}

func (t *ToolchainActions) list(_ context.Context, _ *cli.Command) error {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer tw.Flush()
	fmt.Fprintln(tw, "NAME\tVERSION\tPURPOSE\tINSTALLABLE HOSTS")
	for _, tool := range t.ToolCatalog.Tools() {
		v := tool.Version
		if v == "" {
			v = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", tool.Slug, v, tool.RequiredFor, hostsString(tool.AvailableHosts))
	}
	return nil
}

func (t *ToolchainActions) path(_ context.Context, cmd *cli.Command) error {
	if cmd.NArg() != 1 {
		return fmt.Errorf("usage: gdnext toolchain path <name>")
	}
	tool := t.ToolCatalog.BySlug(cmd.Args().First())
	if tool == nil {
		return fmt.Errorf("unknown toolchain %q (try: gdnext toolchain list)", cmd.Args().First())
	}
	path, err := tool.Lookup(tooling.ModeFind)
	if err != nil {
		return err
	}
	fmt.Println(path)
	return nil
}

// install installs one named toolchain, or every tool needed by any
// target the current host can build.
func (t *ToolchainActions) install(_ context.Context, cmd *cli.Command) error {
	if cmd.Bool("skip-checksum") {
		os.Setenv("GDNEXT_SKIP_CHECKSUM", "1")
	}
	if cmd.NArg() == 1 {
		tool := t.ToolCatalog.BySlug(cmd.Args().First())
		if tool == nil {
			return fmt.Errorf("unknown toolchain %q", cmd.Args().First())
		}
		if !tool.CanInstallOn(t.BuildEnv.Host) {
			return fmt.Errorf("toolchain %q cannot be installed on host %s (AvailableHosts=%s)",
				tool.Slug, t.BuildEnv.Host.Tuple(), hostsString(tool.AvailableHosts))
		}
		path, err := tool.Lookup(tooling.ModeInstall)
		if err != nil {
			return err
		}
		fmt.Println(path)
		return nil
	}
	if cmd.NArg() > 1 {
		return fmt.Errorf("usage: gdnext toolchain install [name]")
	}
	jobs := jobsForHost(t.ToolCatalog, t.BuildEnv.Host)
	if err := validateJobs(t.BuildEnv.Host, jobs); err != nil {
		return err
	}
	failed := installJobs(t.BuildEnv.Host, jobs)
	if failed > 0 {
		return fmt.Errorf("%d toolchain(s) failed to install", failed)
	}
	return nil
}

// doctor renders the per-target install status for every tool host can
// build a target with. With --fix runs install and re-renders.
func (t *ToolchainActions) doctor(_ context.Context, cmd *cli.Command) error {
	if cmd.Bool("skip-checksum") {
		os.Setenv("GDNEXT_SKIP_CHECKSUM", "1")
	}
	format := strings.ToLower(cmd.String("format"))
	jobs := jobsForHost(t.ToolCatalog, t.BuildEnv.Host)
	if err := validateJobs(t.BuildEnv.Host, jobs); err != nil {
		return err
	}
	if format != "" && format != "table" {
		return printDoctorAudit(t.BuildEnv.Host, jobs, format)
	}
	fail := reportJobStatus(t.BuildEnv.Host, jobs)
	if cmd.Bool("fix") && fail > 0 {
		fmt.Fprintln(os.Stdout, "\n→ installing missing toolchains...")
		installJobs(t.BuildEnv.Host, jobs)
		after := countMissingJobs(jobs)
		if after < fail {
			fmt.Fprintln(os.Stdout)
			fail = reportJobStatus(t.BuildEnv.Host, jobs)
		} else {
			fail = after
		}
	}
	if fail > 0 {
		return fmt.Errorf("%d toolchain(s) missing on host %s (rerun with --fix to auto-download)",
			fail, t.BuildEnv.Host.Tuple())
	}
	fmt.Fprintf(os.Stdout, "all toolchains present for every target buildable from %s\n", t.BuildEnv.Host.Tuple())
	return nil
}

func countMissingJobs(jobs []toolJob) (n int) {
	for _, j := range jobs {
		if _, err := j.Lookup(tooling.ModeFind); err != nil {
			n++
		}
	}
	return n
}

// toolJob is one (tool, target-tuple) unit of lookup/install work.
// Host-scoped tools carry the host tuple; IsLibrary tools carry the
// target tuple so per-target archives fetch correctly. ContextTargets
// records the Platforms (and LinkModes) that caused the queue.
type toolJob struct {
	Tool           *tooling.Tool
	GOOS           string
	GOARCH         string
	IsLibrary      bool
	ContextTargets []jobContext
	// Experimental: every consumer Platform is Experimental, so a
	// failure here doesn't count toward the install failure tally.
	Experimental bool
}

type jobContext struct {
	Target   product.Platform
	LinkMode product.LinkMode
}

func (j toolJob) Lookup(mode ...tooling.Mode) (string, error) {
	return j.Tool.LookupPlatform(j.GOOS, j.GOARCH, mode...)
}

// jobsForHost returns, in catalog order, the unique (tool, target-tuple)
// jobs needed to prepare every target host can build. Non-library tools
// dedupe by slug; library tools dedupe by (slug, goos, goarch).
func jobsForHost(catalog tooling.Catalog, host product.BuildHost) []toolJob {
	type key struct{ slug, goos, goarch string }
	idx := map[key]*toolJob{}
	var order []key
	add := func(t product.Toolchain, goos, goarch string, ctx jobContext) {
		// IsLibrary AvailableHosts lists published target tuples; skip
		// jobs whose tuple has no published artefact.
		if t.IsLibrary && !t.CanInstallOn(product.BuildHost{GOOS: goos, GOARCH: goarch}) {
			return
		}
		k := key{slug: t.Slug}
		if t.IsLibrary {
			k = key{slug: t.Slug, goos: goos, goarch: goarch}
		}
		if existing, ok := idx[k]; ok {
			existing.ContextTargets = append(existing.ContextTargets, ctx)
			return
		}
		runtime := catalog.BySlug(t.Slug)
		if runtime == nil {
			return
		}
		jg, jc := host.GOOS, host.GOARCH
		if t.IsLibrary {
			jg, jc = goos, goarch
		}
		job := &toolJob{
			Tool:           runtime,
			GOOS:           jg,
			GOARCH:         jc,
			IsLibrary:      t.IsLibrary,
			ContextTargets: []jobContext{ctx},
		}
		idx[k] = job
		order = append(order, k)
	}
	for _, p := range product.PlatformMatrix {
		if !p.Kind.Has(product.Target) {
			continue
		}
		if !p.CanBuildOn(host.GOOS, host.GOARCH) {
			continue
		}
		if p.LinkModes.Has(product.GDExtension) {
			ctx := jobContext{Target: p, LinkMode: product.GDExtension}
			for _, t := range p.BuildTools {
				add(t, p.GOOS, p.GOARCH, ctx)
			}
		}
		if p.LinkModes.Has(product.LibGodot) {
			ctx := jobContext{Target: p, LinkMode: product.LibGodot}
			for _, t := range p.BuildTools {
				add(t, p.GOOS, p.GOARCH, ctx)
			}
			for _, t := range product.LibGodotToolchains {
				add(t, p.GOOS, p.GOARCH, ctx)
			}
		}
	}
	catalogOrder := map[string]int{}
	for i, t := range catalog.Tools() {
		catalogOrder[t.Slug] = i
	}
	out := make([]toolJob, 0, len(order))
	for _, k := range order {
		j := idx[k]
		j.Experimental = jobIsExperimentalOnly(*j)
		out = append(out, *j)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0; j-- {
			a, b := out[j-1], out[j]
			ai, bi := catalogOrder[a.Tool.Slug], catalogOrder[b.Tool.Slug]
			if ai < bi || (ai == bi && (a.GOOS < b.GOOS || (a.GOOS == b.GOOS && a.GOARCH < b.GOARCH))) {
				break
			}
			out[j-1], out[j] = b, a
		}
	}
	return out
}

func jobIsExperimentalOnly(j toolJob) bool {
	if len(j.ContextTargets) == 0 {
		return false
	}
	for _, ctx := range j.ContextTargets {
		if !ctx.Target.Status.Has(product.Experimental) {
			return false
		}
	}
	return true
}

// validateJobs surfaces product-catalog bugs (a non-library tool a
// target needs whose installer doesn't cover host) before any download.
// IsLibrary tools are gated per-target in jobsForHost, not here.
func validateJobs(host product.BuildHost, jobs []toolJob) error {
	seen := map[string]bool{}
	var bad []string
	for _, j := range jobs {
		if j.IsLibrary {
			continue
		}
		if seen[j.Tool.Slug] {
			continue
		}
		seen[j.Tool.Slug] = true
		if !j.Tool.CanInstallOn(host) {
			bad = append(bad, fmt.Sprintf("%s (AvailableHosts=%s)", j.Tool.Slug, hostsString(j.Tool.AvailableHosts)))
		}
	}
	if len(bad) == 0 {
		return nil
	}
	return fmt.Errorf("product catalog: tool(s) needed on host %s but not declared installable there:\n  %s",
		host.Tuple(), strings.Join(bad, "\n  "))
}

// installJobs runs ModeInstall on every missing job. One failure does
// not abort the rest. Returns the count of non-experimental failures.
func installJobs(host product.BuildHost, jobs []toolJob) (failed int) {
	var (
		todo     []toolJob
		alreadyN int
	)
	for _, j := range jobs {
		if _, err := j.Lookup(tooling.ModeFind); err == nil {
			alreadyN++
			continue
		}
		todo = append(todo, j)
	}
	if len(todo) == 0 {
		fmt.Printf("All %d toolchain(s) already installed.\n", alreadyN)
		return 0
	}
	fmt.Printf("Installing %d toolchain(s) for %s (%d already present)\n\n", len(todo), host.Tuple(), alreadyN)
	var width int
	for _, j := range todo {
		if n := len(jobLabel(j)); n > width {
			width = n
		}
	}
	type miss struct {
		label        string
		err          error
		experimental bool
	}
	var misses []miss
	var installedN, skippedN int
	for _, j := range todo {
		fmt.Printf("  %-*s  ", width, jobLabel(j))
		path, err := j.Lookup(tooling.ModeInstall)
		if err != nil {
			if j.Experimental {
				fmt.Println("SKIP  (experimental)")
				skippedN++
			} else {
				fmt.Println("FAIL")
				failed++
			}
			misses = append(misses, miss{label: jobLabel(j), err: err, experimental: j.Experimental})
			continue
		}
		fmt.Println("OK   ", path)
		installedN++
	}
	fmt.Println()
	switch {
	case failed > 0:
		fmt.Printf("Summary: %d installed, %d skipped, %d failed.\n", installedN, skippedN, failed)
	case skippedN > 0:
		fmt.Printf("Summary: %d installed, %d skipped (experimental).\n", installedN, skippedN)
	default:
		fmt.Printf("Summary: %d installed.\n", installedN)
	}
	if len(misses) > 0 {
		fmt.Println("\nErrors:")
		for _, m := range misses {
			tag := ""
			if m.experimental {
				tag = " (experimental, ignored)"
			}
			fmt.Printf("  %s%s: %s\n", m.label, tag, m.err)
		}
	}
	return failed
}

// jobLabel renders a job's display name. Library jobs include the
// (goos/goarch) tuple so per-target rows stay distinct.
func jobLabel(j toolJob) string {
	if j.IsLibrary {
		return j.Tool.Slug + " (" + product.Tuple(j.GOOS, j.GOARCH) + ")"
	}
	return j.Tool.Slug
}

// reportJobStatus prints the per-job status table (ModeFind, no
// downloads) and returns the count of non-experimental missing jobs.
func reportJobStatus(host product.BuildHost, jobs []toolJob) (missing int) {
	fmt.Fprintf(os.Stdout, "host:    %s\ntargets: %s\n\n", host.Tuple(), targetsString(targetsForHost(host)))
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tPURPOSE\tLINK\tSTATUS\tDETAIL")
	type miss struct {
		label        string
		err          error
		experimental bool
	}
	var misses []miss
	for _, j := range jobs {
		path, err := j.Lookup(tooling.ModeFind)
		link := jobLinkSummary(j)
		if err == nil {
			fmt.Fprintf(tw, "%s\t%s\t%s\tOK\t%s\n", jobLabel(j), j.Tool.RequiredFor, link, path)
			continue
		}
		status := "MISSING"
		if j.Experimental {
			status = "MISSING (experimental)"
		} else {
			missing++
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t-\n", jobLabel(j), j.Tool.RequiredFor, link, status)
		misses = append(misses, miss{label: jobLabel(j), err: err, experimental: j.Experimental})
	}
	tw.Flush()
	if len(misses) > 0 {
		fmt.Fprintln(os.Stdout, "\nErrors:")
		for _, m := range misses {
			tag := ""
			if m.experimental {
				tag = " (experimental, ignored)"
			}
			fmt.Fprintf(os.Stdout, "  %s%s: %s\n", m.label, tag, m.err)
		}
	}
	return missing
}

func jobLinkSummary(j toolJob) string {
	var modes product.LinkMode
	for _, ctx := range j.ContextTargets {
		modes |= ctx.LinkMode
	}
	if modes == 0 {
		return "-"
	}
	return modes.String()
}

func targetsForHost(host product.BuildHost) []product.Platform {
	out := make([]product.Platform, 0, len(product.PlatformMatrix))
	for _, p := range product.PlatformMatrix {
		if !p.Kind.Has(product.Target) {
			continue
		}
		if !p.CanBuildOn(host.GOOS, host.GOARCH) {
			continue
		}
		out = append(out, p)
	}
	return out
}

func targetsString(targets []product.Platform) string {
	if len(targets) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(targets))
	for _, t := range targets {
		parts = append(parts, t.Tuple())
	}
	return strings.Join(parts, ",")
}

func hostsString(hosts []product.BuildHost) string {
	if len(hosts) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(hosts))
	for _, h := range hosts {
		parts = append(parts, h.Tuple())
	}
	return strings.Join(parts, ",")
}

// DoctorAuditRow is the per-job audit shape emitted by `gdnext
// toolchain doctor --format=json|yaml|xml`. Each row is one
// (tool, target) entry the host needs, with the resolved install
// path, the downloaded archive's byte size and SHA256 (for
// supply-chain audit), plus the catalog Source URL the artefact
// was fetched from. Source is informational provenance —
// KnownChecksums identifies artefacts by hash, so a mirror change
// does not invalidate the supply-chain check.
type DoctorAuditRow struct {
	XMLName  xml.Name `json:"-"                  xml:"entry"               yaml:"-"`
	Slug     string   `json:"slug"               xml:"slug"                yaml:"slug"`
	Name     string   `json:"name"               xml:"name"                yaml:"name"`
	Version  string   `json:"version,omitempty"  xml:"version,omitempty"   yaml:"version,omitempty"`
	GOOS     string   `json:"goos"               xml:"goos"                yaml:"goos"`
	GOARCH   string   `json:"goarch"             xml:"goarch"              yaml:"goarch"`
	Host     string   `json:"host"               xml:"host"                yaml:"host"`
	Library  bool     `json:"library,omitempty"  xml:"library,attr,omitempty" yaml:"library,omitempty"`
	Required string   `json:"required_for,omitempty" xml:"required_for,omitempty" yaml:"required_for,omitempty"`
	Status   string   `json:"status"             xml:"status,attr"         yaml:"status"`
	Path     string   `json:"path,omitempty"     xml:"path,omitempty"      yaml:"path,omitempty"`
	Size     int64    `json:"size,omitempty"     xml:"size,omitempty"      yaml:"size,omitempty"`
	SHA256   string   `json:"sha256,omitempty"   xml:"sha256,omitempty"    yaml:"sha256,omitempty"`
	Source   string   `json:"source,omitempty"   xml:"source,omitempty"    yaml:"source,omitempty"`
	Error    string   `json:"error,omitempty"    xml:"error,omitempty"     yaml:"error,omitempty"`
}

func printDoctorAudit(host product.BuildHost, jobs []toolJob, format string) error {
	rows := collectDoctorAudit(host, jobs)
	switch format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rows)
	case "yaml", "yml":
		return yaml.NewEncoder(os.Stdout).Encode(rows)
	case "xml":
		out, err := xml.MarshalIndent(struct {
			XMLName xml.Name         `xml:"toolchain-audit"`
			Entries []DoctorAuditRow `xml:"entry"`
		}{Entries: rows}, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(out))
		return nil
	default:
		return fmt.Errorf("unknown --format %q (want table | json | yaml | xml)", format)
	}
}

func collectDoctorAudit(host product.BuildHost, jobs []toolJob) []DoctorAuditRow {
	out := make([]DoctorAuditRow, 0, len(jobs))
	for _, j := range jobs {
		row := DoctorAuditRow{
			Slug:     j.Tool.Slug,
			Name:     j.Tool.Name,
			Version:  j.Tool.Version,
			GOOS:     j.GOOS,
			GOARCH:   j.GOARCH,
			Host:     host.Tuple(),
			Library:  j.IsLibrary,
			Required: j.Tool.RequiredFor,
			Source:   doctorAuditSource(j.Tool.Toolchain, j.GOOS, j.GOARCH),
		}
		path, err := j.Lookup(tooling.ModeFind)
		if err != nil {
			row.Status = "missing"
			row.Error = err.Error()
			out = append(out, row)
			continue
		}
		row.Status = "ok"
		row.Path = path
		// SHA256 + Size come from the install-time sidecar (download
		// archive, not installed file). Tools satisfied from $PATH or
		// a pre-existing local install have no sidecar, so they audit
		// without a hash or size.
		if sum, size, err := readDownloadSidecar(j.Tool.Path); err == nil {
			row.SHA256 = sum
			row.Size = size
		} else if !os.IsNotExist(err) {
			row.Error = err.Error()
		}
		out = append(out, row)
	}
	return out
}

// doctorAuditSource resolves the catalog's DownloadURL with the same
// substitutions LookupPlatform applies, so the audit row carries the
// exact upstream the artefact was fetched from.
func doctorAuditSource(t product.Toolchain, goos, goarch string) string {
	if u, ok := t.Downloads[goos][goarch]; ok {
		return u
	}
	if t.DownloadURL == "" {
		return ""
	}
	arch := t.DownloadARCH[goarch]
	osTok := strings.ReplaceAll(t.DownloadOS[goos], "$(ARCH)", arch)
	ext := t.DownloadEXT[goos]
	r := strings.NewReplacer(
		"$(VERSION)", t.Version,
		"$(ARCH)", arch,
		"$(OS)", osTok,
		"$(GOARCH)", goarch,
		"$(GOOS)", goos,
		"$(EXT)", ext,
	)
	return r.Replace(t.DownloadURL)
}

// readDownloadSidecar parses the <install_path>.sha256 file written
// by the installer. Format: two newline-terminated lines,
// "sha256:hex" then "size:bytes". Returns os.ErrNotExist when the
// sidecar is absent (system-PATH tools, pre-existing local installs).
func readDownloadSidecar(installPath string) (string, int64, error) {
	if installPath == "" {
		return "", 0, os.ErrNotExist
	}
	raw, err := os.ReadFile(installPath + ".sha256")
	if err != nil {
		return "", 0, err
	}
	var sum string
	var size int64
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		switch {
		case strings.HasPrefix(line, "sha256:"):
			sum = line
		case strings.HasPrefix(line, "size:"):
			fmt.Sscanf(strings.TrimPrefix(line, "size:"), "%d", &size)
		}
	}
	if sum == "" {
		return "", 0, fmt.Errorf("sidecar %s.sha256 has no sha256 line", installPath)
	}
	return sum, size, nil
}
