package ci

import "strings"

// matrixFilter is the parsed form of --filter-host, --filter-target,
// and --filter-link on `ci matrix` and `ci play-matrix`. Each flag
// takes a comma-separated list of canonical values (host/target are
// "<goos>/<goarch>" tuples; link is a LinkMode string like
// "gdextension" or "libgodot"). A row passes when every axis
// matches at least one allowed entry; an empty list on an axis
// allows every value on that axis.
type matrixFilter struct {
	hosts   []string
	targets []string
	links   []string
}

func parseMatrixFilter(hosts, targets, links string) matrixFilter {
	return matrixFilter{
		hosts:   splitTuples(hosts),
		targets: splitTuples(targets),
		links:   splitTuples(links),
	}
}

func splitTuples(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func (f matrixFilter) allows(host, target string) bool {
	return matchesAny(host, f.hosts) && matchesAny(target, f.targets)
}

// allowsLink reports whether link survives the --filter-link axis.
// link == "" (no link mode applies to this row) is always allowed.
func (f matrixFilter) allowsLink(link string) bool {
	if link == "" {
		return true
	}
	return matchesAny(link, f.links)
}

func matchesAny(value string, allow []string) bool {
	if len(allow) == 0 {
		return true
	}
	for _, v := range allow {
		if v == value {
			return true
		}
	}
	return false
}
