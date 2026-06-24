package ci

import "strings"

// matrixFilter is the parsed form of --filter-host and --filter-target
// on `ci matrix` and `ci play-matrix`. Each flag takes a comma-separated
// list of canonical "<goos>/<goarch>" tuples. A row passes when the host
// axis matches any --filter-host entry AND the target axis matches any
// --filter-target entry. An empty list on a given axis allows every
// value on that axis.
type matrixFilter struct {
	hosts   []string
	targets []string
}

func parseMatrixFilter(hosts, targets string) matrixFilter {
	return matrixFilter{
		hosts:   splitTuples(hosts),
		targets: splitTuples(targets),
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
