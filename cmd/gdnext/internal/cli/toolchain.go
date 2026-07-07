package cli

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"strings"

	"graphics.gd/cmd/gdnext/internal/shared"
	"graphics.gd/cmd/gdnext/internal/tooling"
	"graphics.gd/product"

	"github.com/samber/do/v2"
	"github.com/urfave/cli/v3"
	"gopkg.in/yaml.v3"
)

// ToolchainCommand wires `gdnext toolchain`.
type ToolchainCommand struct {
	*cli.Command
	Injector do.Injector `do:""`
}

type ToolchainActions struct {
	BuildEnv    product.BuildEnv `do:""`
	ToolCatalog tooling.Catalog  `do:""`
}

func NewToolchainCommand(di do.Injector) (*ToolchainCommand, error) {
	t := do.MustInvokeStruct[*ToolchainCommand](di)
	t.Command = &cli.Command{
		Name:  "toolchain",
		Usage: "manage the external programs gdnext drives",
		Commands: []*cli.Command{
			{
				Name:  "list",
				Usage: "list every toolchain gdnext can manage",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "format",
						Aliases: []string{"f"},
						Value:   "table",
						Usage:   "output format: table | json | yaml | xml",
					},
				},
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
				Usage:     "install every gdnext-managed toolchain needed by any target buildable from this host (or just one when named); user-managed tools on PATH are reported and skipped",
				ArgsUsage: "[name]",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "skip-checksum",
						Usage: "skip product.Toolchain.KnownChecksums verification after download (sets GD_SKIP_CHECKSUM=1)",
					},
					&cli.BoolFlag{
						Name:  "force",
						Usage: "reinstall already-present gd-managed tools; for a named user-managed tool, install gdnext's pinned copy into GDPath alongside the system one (gdnext will prefer its own)",
					},
				},
				Action: shared.BindAction(t.Injector, (*ToolchainActions).install),
			},
			{
				Name:      "uninstall",
				Usage:     "remove a gd-managed toolchain from GDPath; user-managed tools on PATH are refused",
				ArgsUsage: "<name>",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "all",
						Usage: "uninstall every gd-managed toolchain on this host (cannot be combined with a positional name)",
					},
					&cli.BoolFlag{
						Name:  "keep-checksum",
						Usage: "keep the <GDPath>/checksums/<slug>-<goos>-<goarch>.sha256 file so the next install pins the same hash via the checksum union",
					},
				},
				Action: shared.BindAction(t.Injector, (*ToolchainActions).uninstall),
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

func NewToolchainActions(di do.Injector) (*ToolchainActions, error) {
	return do.InvokeStruct[*ToolchainActions](di)
}

func (t *ToolchainActions) list(_ context.Context, cmd *cli.Command) error {
	format := strings.ToLower(cmd.String("format"))
	rows := collectCatalogRows(t.ToolCatalog)
	if format == "" || format == "table" {
		table := make([][]string, 0, len(rows))
		for _, r := range rows {
			v := r.Version
			if v == "" {
				v = "-"
			}
			table = append(table, []string{r.Slug, v, collapseHosts(r.Hosts)})
		}
		renderTable(os.Stdout,
			[]string{"NAME", "VERSION", "INSTALLABLE HOSTS"}, table)
		return nil
	}
	return encodeStructured(format, "toolchain-catalog", "entry", rows)
}

// collapseHosts folds a list of "goos/goarch" hosts into one entry per
// GOOS, listing its architectures: "linux/{amd64,arm64},windows/amd64".
// GOOS and arch order follow first appearance in the input. Entries
// without a "/" pass through unchanged.
func collapseHosts(hosts []string) string {
	var order []string
	arches := map[string][]string{}
	for _, h := range hosts {
		goos, arch, ok := strings.Cut(h, "/")
		if !ok {
			if _, seen := arches[h]; !seen {
				order = append(order, h)
				arches[h] = nil
			}
			continue
		}
		if _, seen := arches[goos]; !seen {
			order = append(order, goos)
		}
		arches[goos] = append(arches[goos], arch)
	}
	groups := make([]string, 0, len(order))
	for _, goos := range order {
		switch a := arches[goos]; len(a) {
		case 0:
			groups = append(groups, goos)
		case 1:
			groups = append(groups, goos+"/"+a[0])
		default:
			groups = append(groups, goos+"/{"+strings.Join(a, ",")+"}")
		}
	}
	return strings.Join(groups, ",")
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
// target the current host can build. UserManaged tools (resolvable
// on $PATH outside GDPath) are skipped with a reported line: gdnext
// owns its tools, not yours, and replacing a system-installed tool
// silently is the wrong default. --force overrides the skip:
// already-present gd-managed tools are re-downloaded, and a named
// user-managed tool gets a pinned copy installed into GDPath.
func (t *ToolchainActions) install(_ context.Context, cmd *cli.Command) error {
	if cmd.Bool("skip-checksum") {
		os.Setenv(product.EnvSkipChecksum, "1")
	}
	force := cmd.Bool("force")
	if cmd.NArg() == 1 {
		tool := t.ToolCatalog.BySlug(cmd.Args().First())
		if tool == nil {
			return fmt.Errorf("unknown toolchain %q", cmd.Args().First())
		}
		if !tool.CanInstallOn(t.BuildEnv.Host) {
			return fmt.Errorf("toolchain %q cannot be installed on host %s (AvailableHosts=%s)",
				tool.Slug, t.BuildEnv.Host.Tuple(), hostsString(tool.AvailableHosts))
		}
		if existing, err := tool.Lookup(tooling.ModeFind); err == nil && !force {
			switch tool.ManagedByPath(existing) {
			case product.UserManaged:
				fmt.Fprintf(os.Stdout, "skip: %s is already installed (user-managed at %s; pass --force to install gdnext's pinned copy into %s)\n",
					tool.Slug, existing, t.BuildEnv.Host.GDRootPath)
			default:
				fmt.Fprintf(os.Stdout, "skip: %s is already installed (gd-managed at %s; pass --force to re-download)\n", tool.Slug, existing)
			}
			return nil
		}
		mode := tooling.ModeInstall
		if force {
			mode = tooling.ModeForceInstall
			tool.Path = "" // drop the cached Path so Lookup re-runs the resolver
		}
		if src := doctorAuditSource(tool.Toolchain, t.BuildEnv.Host.GOOS, t.BuildEnv.Host.GOARCH, ""); src != "" {
			fmt.Printf("source: %s\n", src)
		}
		path, err := tool.Lookup(mode)
		if err != nil {
			return err
		}
		fmt.Printf("installed: %s -> %s\n", tool.Slug, path)
		if sum, _, err := tooling.ReadSidecar(tooling.SidecarPath(t.BuildEnv.Host, tool.Slug, t.BuildEnv.Host.GOOS, t.BuildEnv.Host.GOARCH, "")); err == nil {
			fmt.Printf("    %s\n", sum)
		}
		return nil
	}
	if cmd.NArg() > 1 {
		return fmt.Errorf("usage: gdnext toolchain install [name]")
	}
	jobs := jobsForHost(t.ToolCatalog, t.BuildEnv.Host)
	if err := validateJobs(t.BuildEnv.Host, jobs); err != nil {
		return err
	}
	failed := installJobs(t.BuildEnv.Host, jobs, force)
	if failed > 0 {
		return fmt.Errorf("%d toolchain(s) failed to install", failed)
	}
	return nil
}

// uninstall removes the on-disk binary (and by default the matching
// sidecar) of a gd-managed toolchain. UserManaged tools are refused —
// gdnext didn't install them and won't remove them. --all walks every
// gd-managed tool relevant to this host and uninstalls each, with
// per-tool report lines matching the install verb's shape.
func (t *ToolchainActions) uninstall(_ context.Context, cmd *cli.Command) error {
	all := cmd.Bool("all")
	keepSidecar := cmd.Bool("keep-checksum")
	if all && cmd.NArg() > 0 {
		return fmt.Errorf("--all is exclusive with a positional <name>")
	}
	if !all && cmd.NArg() != 1 {
		return fmt.Errorf("usage: gdnext toolchain uninstall <name>   (or --all)")
	}
	if cmd.NArg() == 1 {
		tool := t.ToolCatalog.BySlug(cmd.Args().First())
		if tool == nil {
			return fmt.Errorf("unknown toolchain %q", cmd.Args().First())
		}
		switch err := uninstallTool(t.BuildEnv.Host, tool, keepSidecar); {
		case err == nil:
			return nil
		case errors.Is(err, errUninstallUserManaged):
			fmt.Fprintf(os.Stderr, "skip: %s is user-managed (gdnext didn't install it; remove via your package manager)\n", tool.Slug)
			return nil
		case errors.Is(err, errUninstallMissing):
			fmt.Printf("skip: %s is not installed\n", tool.Slug)
			return nil
		default:
			return err
		}
	}
	// --all
	fmt.Printf("toolchain uninstall for %s\n", t.BuildEnv.Host.Tuple())
	var removed, skipped, missing, errored int
	for _, tool := range t.ToolCatalog.Tools() {
		header := tool.Slug
		if v := tool.Version; v != "" {
			header += " v" + v
		}
		fmt.Printf("\n==> %s\n", header)
		switch err := uninstallTool(t.BuildEnv.Host, tool, keepSidecar); {
		case err == nil:
			removed++
		case errors.Is(err, errUninstallUserManaged):
			fmt.Println("    skip: user-managed (gdnext didn't install this; leaving it alone)")
			skipped++
		case errors.Is(err, errUninstallMissing):
			fmt.Println("    skip: not installed")
			missing++
		default:
			fmt.Printf("    FAIL: %s\n", err)
			errored++
		}
	}
	fmt.Println()
	fmt.Printf("Summary: %d removed, %d user-managed (skipped), %d not installed, %d failed.\n",
		removed, skipped, missing, errored)
	if errored > 0 {
		return fmt.Errorf("%d toolchain(s) failed to uninstall", errored)
	}
	return nil
}

// errUninstallUserManaged signals that uninstallTool refused to touch
// a tool because the resolved binary is outside GDPath (user-managed).
// Sentinel error so the --all bulk path can render a "skip" line
// instead of bailing.
var errUninstallUserManaged = errors.New("toolchain is user-managed")

// errUninstallMissing signals that the tool isn't installed anywhere
// gdnext can see (no GDPath copy, no PATH copy). The --all path
// reports it as "not installed" instead of failing.
var errUninstallMissing = errors.New("toolchain is not installed")

// uninstallTool removes tool.Path (when GDManaged) and, unless
// keepSidecar is set, the matching central sidecar.
func uninstallTool(host product.BuildHost, tool *tooling.Tool, keepSidecar bool) error {
	path, err := tool.Lookup(tooling.ModeFind)
	if err != nil {
		return errUninstallMissing
	}
	if tool.ManagedByPath(path) == product.UserManaged {
		fmt.Printf("    found at %s\n", path)
		return errUninstallUserManaged
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	fmt.Printf("    removed binary: %s\n", path)
	tool.Path = "" // drop cache so subsequent Lookup calls re-resolve
	if !keepSidecar {
		sidecar := tooling.SidecarPath(host, tool.Slug, host.GOOS, host.GOARCH, "")
		if tool.IsLibrary {
			// IsLibrary tools may have multiple per-target sidecars;
			// walk PlatformMatrix + libc variants to clear them all.
			libcs := []string{"", product.LibCGlibc, product.LibCMusl}
			for _, p := range product.PlatformMatrix {
				if !p.Kind.Has(product.Target) {
					continue
				}
				for _, libc := range libcs {
					s := tooling.SidecarPath(host, tool.Slug, p.GOOS, p.GOARCH, libc)
					if err := os.Remove(s); err == nil {
						fmt.Printf("    removed checksum: %s\n", s)
					}
				}
			}
		} else if err := os.Remove(sidecar); err == nil {
			fmt.Printf("    removed checksum: %s\n", sidecar)
		}
	}
	return nil
}

// doctor renders the per-target install status for every tool host can
// build a target with. With --fix runs install and re-renders.
func (t *ToolchainActions) doctor(_ context.Context, cmd *cli.Command) error {
	if cmd.Bool("skip-checksum") {
		os.Setenv(product.EnvSkipChecksum, "1")
	}
	format := strings.ToLower(cmd.String("format"))
	jobs := jobsForHost(t.ToolCatalog, t.BuildEnv.Host)
	if err := validateJobs(t.BuildEnv.Host, jobs); err != nil {
		return err
	}
	// Append opt-in tools the user has already installed. Missing
	// optionals stay hidden — this is the "show me what I opted
	// into, don't nag me about the ones I didn't" behavior.
	jobs = append(jobs, optionalJobsInstalled(t.ToolCatalog, t.BuildEnv.Host)...)
	sortJobsByCatalog(t.ToolCatalog, jobs)
	if format != "" && format != "table" {
		return printDoctorAudit(t.BuildEnv.Host, jobs, format)
	}
	fail := reportJobStatus(t.BuildEnv.Host, jobs)
	if cmd.Bool("fix") && fail > 0 {
		fmt.Fprintln(os.Stdout, "\n→ installing missing toolchains...")
		installJobs(t.BuildEnv.Host, jobs, false)
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
	LibC           string
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
	return j.Tool.LookupPlatform(j.GOOS, j.GOARCH, j.LibC, mode...)
}

// jobsForHost returns, in catalog order, the unique (tool, target-tuple)
// jobs needed to prepare every target host can build. Non-library tools
// dedupe by slug; library tools dedupe by (slug, goos, goarch).
func jobsForHost(catalog tooling.Catalog, host product.BuildHost) []toolJob {
	type key struct{ slug, goos, goarch, libc string }
	idx := map[key]*toolJob{}
	var order []key
	add := func(t product.Toolchain, goos, goarch string, ctx jobContext) {
		// Optional tools opt out of the bulk install walk; they're
		// only pulled in when a specific verb asks for them
		// (e.g. `gdnext libgodot build --goos android` triggers
		// ToolchainAndroidNDK via a direct Lookup).
		if t.Optional {
			return
		}
		// IsLibrary AvailableHosts lists published target tuples; skip
		// jobs whose tuple has no published artefact.
		if t.IsLibrary && !t.CanInstallOn(product.BuildHost{GOOS: goos, GOARCH: goarch}) {
			return
		}
		libc := ""
		if t.IsLibrary && goos == product.GOOSLinux && (t.Slug == product.ToolchainLibGodot.Slug || t.Slug == product.ToolchainLibGodotEditor.Slug) {
			libc = product.LibCGlibc
		}
		k := key{slug: t.Slug}
		if t.IsLibrary {
			k = key{slug: t.Slug, goos: goos, goarch: goarch, libc: libc}
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
			LibC:           libc,
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
	out := make([]toolJob, 0, len(order))
	for _, k := range order {
		j := idx[k]
		j.Experimental = jobIsExperimentalOnly(*j)
		out = append(out, *j)
	}
	sortJobsByCatalog(catalog, out)
	return out
}

// sortJobsByCatalog stable-sorts jobs into the order catalog.Tools()
// declares them, tie-breaking by (goos, goarch) so per-target library
// jobs render deterministically. Used by jobsForHost after the initial
// PlatformMatrix walk, and by doctor after appending installed
// optionals so both flows agree on row ordering.
func sortJobsByCatalog(catalog tooling.Catalog, jobs []toolJob) {
	order := map[string]int{}
	for i, t := range catalog.Tools() {
		order[t.Slug] = i
	}
	for i := 1; i < len(jobs); i++ {
		for j := i; j > 0; j-- {
			a, b := jobs[j-1], jobs[j]
			ai, bi := order[a.Tool.Slug], order[b.Tool.Slug]
			if ai < bi || (ai == bi && (a.GOOS < b.GOOS || (a.GOOS == b.GOOS && a.GOARCH < b.GOARCH))) {
				break
			}
			jobs[j-1], jobs[j] = b, a
		}
	}
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

// optionalJobsInstalled returns synthetic doctor rows for every
// Toolchain.Optional entry the user has already installed on host.
// Uses ModeFind so a missing optional stays hidden — the whole point
// of Optional is that the user opts in per-verb, and doctor should
// not nag about opt-outs. --fix intentionally ignores these too: they
// were installed by a named `toolchain install` (or by a verb that
// does its own Lookup) and doctor is not the right place to make the
// opt-in call.
func optionalJobsInstalled(catalog tooling.Catalog, host product.BuildHost) []toolJob {
	var out []toolJob
	for _, tool := range catalog.Tools() {
		if !tool.Optional {
			continue
		}
		if !tool.CanInstallOn(host) {
			continue
		}
		if _, err := tool.LookupPlatform(host.GOOS, host.GOARCH, "", tooling.ModeFind); err != nil {
			continue
		}
		out = append(out, toolJob{
			Tool:      tool,
			GOOS:      host.GOOS,
			GOARCH:    host.GOARCH,
			IsLibrary: tool.IsLibrary,
		})
	}
	return out
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
func installJobs(host product.BuildHost, jobs []toolJob, force bool) (failed int) {
	fmt.Printf("toolchain install for %s", host.Tuple())
	if force {
		fmt.Print(" (--force)")
	}
	fmt.Println()
	var (
		installedN  int
		userOwnedN  int
		gdAlreadyN  int
		expSkippedN int
	)
	type miss struct {
		label        string
		err          error
		experimental bool
	}
	var misses []miss
	// IsLibrary tools fan out to one job per target tuple, but several
	// of them (notably android.jar) resolve to the same on-disk file
	// regardless of target. Dedupe by resolved path so we only print
	// + download each unique artefact once.
	seenPath := map[string]bool{}
	for _, j := range jobs {
		header := jobLabel(j)
		if v := j.Tool.Version; v != "" {
			header += " v" + v
		}
		fmt.Printf("\n==> %s\n", header)
		if src := doctorAuditSource(j.Tool.Toolchain, j.GOOS, j.GOARCH, j.LibC); src != "" {
			fmt.Printf("    source: %s\n", src)
		}
		mode := tooling.ModeInstall
		if path, err := j.Lookup(tooling.ModeFind); err == nil {
			if seenPath[path] {
				fmt.Printf("    skip: shares artefact with a previous job (%s)\n", path)
				continue
			}
			seenPath[path] = true
			if j.Tool.ManagedByPath(path) == product.UserManaged {
				// --force never touches user-managed tools in bulk
				// mode; that'd surprise users who installed go/adb
				// via their package manager. Use the named-install
				// path to opt in per-slug.
				fmt.Printf("    skip: already installed (user-managed at %s)\n", path)
				userOwnedN++
				continue
			}
			if !force {
				fmt.Printf("    skip: already installed (gd-managed at %s)\n%s", path, sidecarSHALine(host, j))
				gdAlreadyN++
				continue
			}
			fmt.Println("    --force: re-downloading")
			mode = tooling.ModeForceInstall
			j.Tool.Path = "" // drop the cached Path so Lookup re-runs
		}
		path, err := j.Lookup(mode)
		if err != nil {
			if j.Experimental {
				fmt.Printf("    skip: install failed (experimental, ignored): %s\n", err)
				expSkippedN++
			} else {
				fmt.Printf("    FAIL: %s\n", err)
				failed++
			}
			misses = append(misses, miss{label: jobLabel(j), err: err, experimental: j.Experimental})
			continue
		}
		seenPath[path] = true
		fmt.Printf("    installed: %s\n%s", path, sidecarSHALine(host, j))
		installedN++
	}
	fmt.Println()
	fmt.Printf("Summary: %d installed, %d already gd-managed, %d already user-managed, %d skipped (experimental), %d failed.\n",
		installedN, gdAlreadyN, userOwnedN, expSkippedN, failed)
	if failed > 0 {
		fmt.Println("\nErrors:")
		for _, m := range misses {
			if m.experimental {
				continue
			}
			fmt.Printf("  %s: %s\n", m.label, m.err)
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
// IsLibrary jobs that resolve to the same on-disk file are collapsed
// into one row (e.g. android.jar fans out three times in the job
// stream but lands at a single path on linux/amd64).
func reportJobStatus(host product.BuildHost, jobs []toolJob) (missing int) {
	fmt.Fprintf(os.Stdout, "host:    %s\ntargets: %s\n\n", host.Tuple(), targetsString(targetsForHost(host)))
	type miss struct {
		label        string
		err          error
		experimental bool
	}
	var misses []miss
	var table [][]string
	seenOK := map[string]struct{}{}
	for _, j := range jobs {
		path, err := j.Lookup(tooling.ModeFind)
		ver := j.Tool.Version
		if ver == "" {
			ver = "-"
		}
		if err == nil {
			if _, dup := seenOK[path]; dup {
				continue
			}
			seenOK[path] = struct{}{}
			managed := j.Tool.ManagedByPath(path)
			sha := "-"
			if managed == product.GDManaged {
				sidecar := tooling.SidecarPath(host, j.Tool.Slug, j.GOOS, j.GOARCH, j.LibC)
				if sum, _, err := tooling.ReadSidecar(sidecar); err == nil {
					sha = shortSHA(sum)
				}
			}
			table = append(table, []string{jobLabel(j), ver, managed.String(), "OK", sha, path})
			continue
		}
		status := "MISSING"
		if j.Experimental {
			status = "MISSING (experimental)"
		} else {
			missing++
		}
		table = append(table, []string{jobLabel(j), ver, "-", status, "-", "-"})
		misses = append(misses, miss{label: jobLabel(j), err: err, experimental: j.Experimental})
	}
	renderTable(os.Stdout,
		[]string{"NAME", "VERSION", "MANAGED", "STATUS", "SHA256", "DETAIL"}, table)
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

// shortSHA truncates a "sha256:<hex>" string to its first 12 hex
// chars for table display, matching the workflow summary's truncation.
func shortSHA(sum string) string {
	hex := strings.TrimPrefix(sum, "sha256:")
	if len(hex) > 12 {
		return hex[:12]
	}
	return hex
}

// sidecarSHALine returns "    sha256: <full hex>\n" when the
// install-time sidecar for this job exists, else "". Printed after
// the install / already-installed line so the hash is grep-able on
// its own row without bloating the primary line.
func sidecarSHALine(host product.BuildHost, j toolJob) string {
	sum, _, err := tooling.ReadSidecar(tooling.SidecarPath(host, j.Tool.Slug, j.GOOS, j.GOARCH, j.LibC))
	if err != nil {
		return ""
	}
	return "    " + sum + "\n"
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

// CatalogRow is the per-tool catalog shape emitted by `gdnext
// toolchain list --format=json|yaml|xml`. Pure catalog metadata —
// no on-host state. Use DoctorAuditRow for "what's actually
// installed on this host" snapshots.
type CatalogRow struct {
	XMLName  xml.Name `json:"-"                      xml:"entry"                  yaml:"-"`
	Slug     string   `json:"slug"                   xml:"slug"                   yaml:"slug"`
	Name     string   `json:"name"                   xml:"name"                   yaml:"name"`
	Version  string   `json:"version,omitempty"      xml:"version,omitempty"      yaml:"version,omitempty"`
	Required string   `json:"required_for,omitempty" xml:"required_for,omitempty" yaml:"required_for,omitempty"`
	Library  bool     `json:"library,omitempty"      xml:"library,attr,omitempty" yaml:"library,omitempty"`
	Optional bool     `json:"optional,omitempty"     xml:"optional,attr,omitempty" yaml:"optional,omitempty"`
	Hosts    []string `json:"installable_hosts"      xml:"installable_hosts>host" yaml:"installable_hosts"`
	Source   string   `json:"source,omitempty"       xml:"source,omitempty"       yaml:"source,omitempty"`
}

func collectCatalogRows(cat tooling.Catalog) []CatalogRow {
	tools := cat.Tools()
	out := make([]CatalogRow, 0, len(tools))
	for _, tool := range tools {
		hosts := make([]string, 0, len(tool.AvailableHosts))
		for _, h := range tool.AvailableHosts {
			hosts = append(hosts, h.Tuple())
		}
		out = append(out, CatalogRow{
			Slug:     tool.Slug,
			Name:     tool.Name,
			Version:  tool.Version,
			Required: tool.RequiredFor,
			Library:  tool.IsLibrary,
			Optional: tool.Optional,
			Hosts:    hosts,
			Source:   tool.DownloadURL,
		})
	}
	return out
}

// DoctorAuditRow is the per-job audit shape emitted by `gdnext
// toolchain doctor --format=json|yaml|xml`. Each row is one
// (tool, target) entry the host needs, with the resolved install
// path, the management type (gdnext-owned vs user-owned), the
// downloaded archive's byte size and SHA256 (for supply-chain
// audit), plus the catalog Source URL the artefact was fetched
// from. Source is informational provenance — KnownChecksums
// identifies artefacts by hash, so a mirror change does not
// invalidate the supply-chain check.
type DoctorAuditRow struct {
	XMLName    xml.Name           `json:"-"                      xml:"entry"                  yaml:"-"`
	Slug       string             `json:"slug"                   xml:"slug"                   yaml:"slug"`
	Name       string             `json:"name"                   xml:"name"                   yaml:"name"`
	Version    string             `json:"version,omitempty"      xml:"version,omitempty"      yaml:"version,omitempty"`
	GOOS       string             `json:"goos"                   xml:"goos"                   yaml:"goos"`
	GOARCH     string             `json:"goarch"                 xml:"goarch"                 yaml:"goarch"`
	Host       string             `json:"host"                   xml:"host"                   yaml:"host"`
	Library    bool               `json:"library,omitempty"      xml:"library,attr,omitempty" yaml:"library,omitempty"`
	ManageType product.ManageType `json:"manage_type"            xml:"manage_type,attr"       yaml:"manage_type"`
	Required   string             `json:"required_for,omitempty" xml:"required_for,omitempty" yaml:"required_for,omitempty"`
	Status     string             `json:"status"                 xml:"status,attr"            yaml:"status"`
	Path       string             `json:"path,omitempty"         xml:"path,omitempty"         yaml:"path,omitempty"`
	Size       int64              `json:"size,omitempty"         xml:"size,omitempty"         yaml:"size,omitempty"`
	SHA256     string             `json:"sha256,omitempty"       xml:"sha256,omitempty"       yaml:"sha256,omitempty"`
	Source     string             `json:"source,omitempty"       xml:"source,omitempty"       yaml:"source,omitempty"`
	Error      string             `json:"error,omitempty"        xml:"error,omitempty"        yaml:"error,omitempty"`
}

func printDoctorAudit(host product.BuildHost, jobs []toolJob, format string) error {
	return encodeStructured(format, "toolchain-audit", "entry", collectDoctorAudit(host, jobs))
}

// encodeStructured renders rows as json / yaml / xml. xmlRoot is the
// outer element name; xmlChild is the per-row element name. Both verbs
// (`toolchain list` and `toolchain doctor --format=...`) share this
// helper so the format flag behaves identically.
func encodeStructured[T any](format, xmlRoot, xmlChild string, rows []T) error {
	switch strings.ToLower(format) {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rows)
	case "yaml", "yml":
		return yaml.NewEncoder(os.Stdout).Encode(rows)
	case "xml":
		// Wrap rows in a labelled root via reflect-free composition:
		// xml.Marshaler on an inline struct keyed by xmlRoot.
		var buf strings.Builder
		buf.WriteString("<" + xmlRoot + ">\n")
		for _, r := range rows {
			inner, err := xml.MarshalIndent(r, "  ", "  ")
			if err != nil {
				return err
			}
			buf.Write(inner)
			buf.WriteByte('\n')
		}
		buf.WriteString("</" + xmlRoot + ">")
		fmt.Println(buf.String())
		_ = xmlChild // xmlChild participates via the row type's XMLName tag
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
			Source:   doctorAuditSource(j.Tool.Toolchain, j.GOOS, j.GOARCH, j.LibC),
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
		row.ManageType = j.Tool.ManagedByPath(path)
		// SHA256 + Size come from the install-time sidecar under
		// <GDChecksumsPath>. UserManaged tools (system PATH or
		// pre-existing local installs) have no sidecar; that's not an
		// error, just no provenance to surface.
		if row.ManageType == product.GDManaged {
			sidecar := tooling.SidecarPath(host, j.Tool.Slug, j.GOOS, j.GOARCH, j.LibC)
			if sum, size, err := tooling.ReadSidecar(sidecar); err == nil {
				row.SHA256 = sum
				row.Size = size
			} else if !os.IsNotExist(err) {
				row.Error = err.Error()
			}
		}
		out = append(out, row)
	}
	return out
}

// doctorAuditSource resolves the catalog's DownloadURL with the same
// substitutions LookupPlatform applies, so the audit row carries the
// exact upstream the artefact was fetched from. libc feeds
// $(LIBC)/$(LIBC_DOT); pass "" for tools that don't fan out on libc.
func doctorAuditSource(t product.Toolchain, goos, goarch, libc string) string {
	if u, ok := t.Downloads[goos][goarch]; ok {
		return u
	}
	if t.DownloadURL == "" {
		return ""
	}
	arch := t.DownloadARCH[goarch]
	osTok := strings.ReplaceAll(t.DownloadOS[goos], "$(ARCH)", arch)
	ext := t.DownloadEXT[goos]
	libcDot := ""
	if libc != "" {
		libcDot = "." + libc
	}
	r := strings.NewReplacer(
		"$(VERSION)", t.Version,
		"$(ARCH)", arch,
		"$(OS)", osTok,
		"$(GOARCH)", goarch,
		"$(GOOS)", goos,
		"$(EXT)", ext,
		"$(LIBC)", libc,
		"$(LIBC_DOT)", libcDot,
	)
	return r.Replace(t.DownloadURL)
}
