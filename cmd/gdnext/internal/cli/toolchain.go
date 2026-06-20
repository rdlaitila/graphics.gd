package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"graphics.gd/cmd/gdnext/internal/tooling"
	"graphics.gd/product"

	"github.com/urfave/cli/v3"
)

func toolchainCmd() *cli.Command {
	return &cli.Command{
		Name:  "toolchain",
		Usage: "manage the external programs gdnext drives",
		Commands: []*cli.Command{
			{
				Name:   "list",
				Usage:  "list every toolchain gdnext can manage",
				Action: toolchainList,
			},
			{
				Name:      "path",
				Usage:     "print the absolute install path of a toolchain (lookup only, no download)",
				ArgsUsage: "<name>",
				Action:    toolchainPath,
			},
			{
				Name:      "install",
				Usage:     "install one named toolchain, or every tool needed by any target buildable from this host",
				ArgsUsage: "[name]",
				Action:    toolchainInstall,
			},
			{
				Name:  "doctor",
				Usage: "report toolchain status for every target buildable from this host; --fix runs install",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "fix",
						Usage: "after reporting, run `toolchain install` and re-report",
					},
				},
				Action: toolchainDoctor,
			},
		},
	}
}

func toolchainList(_ context.Context, _ *cli.Command) error {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer tw.Flush()
	fmt.Fprintln(tw, "NAME\tVERSION\tPURPOSE\tINSTALLABLE HOSTS")
	for _, t := range tooling.Catalog {
		v := t.Version
		if v == "" {
			v = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", t.Slug, v, t.RequiredFor, hostsString(t.AvailableHosts))
	}
	return nil
}

func toolchainPath(_ context.Context, cmd *cli.Command) error {
	if cmd.NArg() != 1 {
		return fmt.Errorf("usage: gdnext toolchain path <name>")
	}
	t := tooling.BySlug(cmd.Args().First())
	if t == nil {
		return fmt.Errorf("unknown toolchain %q (try: gdnext toolchain list)", cmd.Args().First())
	}
	path, err := t.Lookup(tooling.ModeFind)
	if err != nil {
		return err
	}
	fmt.Println(path)
	return nil
}

// toolchainInstall installs one named toolchain, or every tool needed
// by any target the current host can build.
func toolchainInstall(_ context.Context, cmd *cli.Command) error {
	if cmd.NArg() == 1 {
		t := tooling.BySlug(cmd.Args().First())
		if t == nil {
			return fmt.Errorf("unknown toolchain %q", cmd.Args().First())
		}
		if !t.CanInstallOn(buildEnv.Host) {
			return fmt.Errorf("toolchain %q cannot be installed on host %s (AvailableHosts=%s)",
				t.Slug, buildEnv.Host.Tuple(), hostsString(t.AvailableHosts))
		}
		path, err := t.Lookup(tooling.ModeInstall)
		if err != nil {
			return err
		}
		fmt.Println(path)
		return nil
	}
	if cmd.NArg() > 1 {
		return fmt.Errorf("usage: gdnext toolchain install [name]")
	}
	jobs := jobsForHost(buildEnv.Host)
	if err := validateJobs(buildEnv.Host, jobs); err != nil {
		return err
	}
	failed := installJobs(buildEnv.Host, jobs)
	if failed > 0 {
		return fmt.Errorf("%d toolchain(s) failed to install", failed)
	}
	return nil
}

// toolchainDoctor renders the per-target install status for every tool
// host can build a target with. With --fix runs install and re-renders.
func toolchainDoctor(_ context.Context, cmd *cli.Command) error {
	jobs := jobsForHost(buildEnv.Host)
	if err := validateJobs(buildEnv.Host, jobs); err != nil {
		return err
	}
	fail := reportJobStatus(buildEnv.Host, jobs)
	if cmd.Bool("fix") && fail > 0 {
		fmt.Fprintln(os.Stdout, "\n→ installing missing toolchains...")
		installJobs(buildEnv.Host, jobs)
		after := countMissingJobs(jobs)
		if after < fail {
			fmt.Fprintln(os.Stdout)
			fail = reportJobStatus(buildEnv.Host, jobs)
		} else {
			fail = after
		}
	}
	if fail > 0 {
		return fmt.Errorf("%d toolchain(s) missing on host %s (rerun with --fix to auto-download)",
			fail, buildEnv.Host.Tuple())
	}
	fmt.Fprintf(os.Stdout, "all toolchains present for every target buildable from %s\n", buildEnv.Host.Tuple())
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
func jobsForHost(host product.BuildHost) []toolJob {
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
		runtime := tooling.BySlug(t.Slug)
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
	for i, t := range tooling.Catalog {
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
