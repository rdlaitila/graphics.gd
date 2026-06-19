package setup

import "strings"

// TestArgs converts Go-style test flags ("-bench", "-run",
// "-count", "-v", ...) into the "-test.<name>" form that compiled test
// binaries understand. Optimisations on classdb are disabled during
// ordinary test runs (faster compile, easier debugging) but kept enabled
// when -bench is present so benchmarks reflect real performance.
//
// Ported verbatim from cmd/gd/main.go:testArgs.
func TestArgs(args []string) []string {
	converted := []string{}
	var benchmark bool
	for _, arg := range args {
		switch arg {
		case "-bench":
			benchmark = true
			fallthrough
		case "-benchmem", "-benchtime", "blockprofile",
			"-blockprofilerate", "-count", "-coverprofile", "-cpu",
			"-cpuprofile", "-failfast", "-fullpath", "-fuzz", "-fuzzcachedir",
			"-fuzzminimizetime", "-fuzztime", "-fuzzworker", "-gocoverdir",
			"-list", "-memprofile", "-memprofilerate", "-mutexprofile",
			"-mutexprofilefraction", "-outputdir", "-paniconexit0",
			"-parallel", "-run", "-short", "-shuffle", "-skip", "-testlogfile",
			"-timeout", "-trace", "-v":
			converted = append(converted, "-test."+strings.TrimPrefix(arg, "-"))
		default:
			converted = append(converted, arg)
		}
	}
	if !benchmark {
		converted = append([]string{"-gcflags=graphics.gd/classdb/...=-N -l"}, converted...)
	}
	return converted
}
