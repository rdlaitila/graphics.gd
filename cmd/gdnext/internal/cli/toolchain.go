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

// toolchainInstall installs one named toolchain, or every tool needed by
// any target the current host can build. Both paths run the same pre-check
// (validateJobs) so a stale product catalog entry surfaces before any
// download begins.
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

// toolchainDoctor renders the host/target report for every target this host
// can build, and the install status of every tool those targets need. With
// --fix the same install pipeline runs and the report is re-emitted only
// when the install actually changed the missing set — repeating an
// identical table after a failed install is noise.
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

// countMissingJobs returns the number of jobs that fail ModeFind. Used by
// --fix to decide whether to re-render the status table.
func countMissingJobs(jobs []toolJob) (n int) {
	for _, j := range jobs {
		if _, err := j.Lookup(tooling.ModeFind); err != nil {
			n++
		}
	}
	return n
}

// toolJob is one unit of (lookup/install) work — a tool resolved at a
// specific (GOOS, GOARCH). Host-scoped tools (godot, go, zig) carry the
// host tuple; target-scoped library artefacts (libgodot, libgodot-editor,
// android.jar) carry the target's tuple so the right per-target archive
// gets fetched. ContextTargets records which Platforms (with which
// LinkModes) caused this job to be queued, so the diagnostic table can
// show why each entry exists.
type toolJob struct {
	Tool           *tooling.Tool
	GOOS           string
	GOARCH         string
	IsLibrary      bool
	ContextTargets []jobContext
	// Experimental is true when every consumer Platform is Experimental
	// (or every consumer uses an experimental LinkMode). Failures of
	// experimental-only jobs are reported but do not count toward the
	// caller's failure tally.
	Experimental bool
}

// jobContext records why a particular job got queued — used to render
// the per-job "needed for" cell in the doctor table.
type jobContext struct {
	Target   product.Platform
	LinkMode product.LinkMode
}

// Lookup runs at the job's resolved (GOOS, GOARCH) so per-target archives
// fetch the right artefact.
func (j toolJob) Lookup(mode ...tooling.Mode) (string, error) {
	return j.Tool.LookupPlatform(j.GOOS, j.GOARCH, mode...)
}

// jobsForHost returns, in catalog order, the unique set of
// (tool, target-tuple) pairs needed to fully prepare every target
// platform host can build. Non-library tools are deduped by slug across
// targets; library tools are deduped by (slug, GOOS, GOARCH) so each
// per-target archive is its own job.
func jobsForHost(host product.BuildHost) []toolJob {
	type key struct{ slug, goos, goarch string }
	idx := map[key]*toolJob{}
	var order []key
	add := func(t product.Toolchain, goos, goarch string, ctx jobContext) {
		// Library toolchains advertise the target tuples they have
		// published artefacts for via AvailableHosts. Skip jobs whose
		// target isn't covered yet so we don't 404 trying to fetch
		// something the upstream hasn't shipped.
		if t.IsLibrary && !t.CanInstallOn(product.BuildHost{GOOS: goos, GOARCH: goarch}) {
			return
		}
		// Host-scoped: ignore target tuple in the dedup key so we
		// only download/check the tool once per host.
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
			return // unknown to the cli tooling layer; skip
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
		// One pass per LinkMode this platform supports. Tools in
		// p.BuildTools (the GDExtension baseline) are added for the
		// GDExtension mode; LibGodotToolchains are added for the
		// LibGodot mode. SharedToolchains (already inside BuildTools)
		// dedupe across iterations via the slug-only key.
		if p.LinkModes.Has(product.GDExtension) {
			ctx := jobContext{Target: p, LinkMode: product.GDExtension}
			for _, t := range p.BuildTools {
				add(t, p.GOOS, p.GOARCH, ctx)
			}
		}
		if p.LinkModes.Has(product.LibGodot) {
			ctx := jobContext{Target: p, LinkMode: product.LibGodot}
			// Shared host tools come from BuildTools regardless of
			// link mode; LibGodotToolchains layers on the per-target
			// archives.
			for _, t := range p.BuildTools {
				add(t, p.GOOS, p.GOARCH, ctx)
			}
			for _, t := range product.LibGodotToolchains {
				add(t, p.GOOS, p.GOARCH, ctx)
			}
		}
	}
	// Catalog order for stable output: host-scoped first (in catalog
	// order), then library entries grouped by tool slug.
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
	// Stable sort: by catalog order of the underlying tool, then by
	// (goos, goarch) for libraries.
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

// jobIsExperimentalOnly reports whether every Platform that triggered
// this job is Experimental. Failures of such jobs are reported but
// don't count toward the install failure tally.
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

// validateJobs confirms every host-installed tool in jobs declares host
// in its AvailableHosts. A failure is a product-catalog bug — a target
// references a tool whose installer doesn't cover this host — surfaced
// once up front rather than partway through a download. Library
// toolchains are skipped: they're fetched per-target (the (goos, goarch)
// gate lives in jobsForHost), so AvailableHosts there describes target
// tuples, not the host axis.
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

// installJobs runs ModeInstall on every job that isn't already present
// at its resolved tuple. One failing job does not abort the rest.
// Returns the number of *required* failures (jobs whose every consumer
// Platform is non-experimental). Jobs that already resolve via ModeFind
// are skipped silently so `--fix` only narrates real work.
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

// jobLabel renders a job's display name. Host-scoped tools use just the
// slug; per-target libraries include the (goos/goarch) tuple so two
// rows for the same library at different targets stay distinguishable.
func jobLabel(j toolJob) string {
	if j.IsLibrary {
		return j.Tool.Slug + " (" + product.Tuple(j.GOOS, j.GOARCH) + ")"
	}
	return j.Tool.Slug
}

// reportJobStatus renders the per-job status table to stdout (ModeFind,
// no downloads) and returns the count of jobs that aren't installed
// excluding experimental-only ones. Missing jobs get a one-word DETAIL
// cell so the table stays narrow; full per-job error texts go into an
// "Errors:" block below the table.
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

// jobLinkSummary collapses a job's contexts into a `+`-joined LinkMode
// string so the doctor table shows which mode(s) need the entry.
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

// targetsForHost returns every Target platform that host can build, in
// PlatformMatrix order.
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

// targetsString renders a []Platform as "goos/goarch,goos/goarch".
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

// hostsString renders a []BuildHost as "goos/goarch,goos/goarch" for
// error and diagnostic messages.
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
