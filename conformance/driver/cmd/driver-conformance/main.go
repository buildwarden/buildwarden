// Command driver-conformance runs the driver contract's conformance suite
// (specs/driver-contract/v1.md) against a driver binary:
//
//	driver-conformance --driver <path> [--tier static|lifecycle|all]
//	    [--driver-arg <flag>]... [--config-file <file>]
//	    [--base-image <ref>] [--image <image>] [--stub-relay]
//	    [--skip <case>]... [--exit-script <file>] [--case-deadline <duration>]
//
// The static tier needs nothing of the host; the lifecycle tier runs real
// builds, so it needs a host the driver runs on, and takes the driver's
// flags and a config file for that host. It prints one line per check, with
// the requirements the check holds the driver to, and exits 1 when a check
// failed.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	driverconformance "github.com/buildwarden/buildwarden/conformance/driver"
)

// list is a repeatable flag.
type list []string

func (l *list) String() string     { return strings.Join(*l, ",") }
func (l *list) Set(v string) error { *l = append(*l, v); return nil }

func main() {
	path := flag.String("driver", "", "the driver binary, warden-driver-<name>")
	tier := flag.String("tier", "all", "the tiers to run: static, lifecycle or all")
	var o driverconformance.LifecycleOptions
	var args, skip list
	flag.Var(&args, "driver-arg", "a flag of the driver's, passed on in args (repeated)")
	flag.StringVar(&o.ConfigFile, "config-file", "", "a config file for this host, "+
		"passed on as config_file")
	flag.StringVar(&o.BaseImage, "base-image", "", "the build's base image (start.base_image)")
	flag.StringVar(&o.Image, "image", "", "the build's image (start.image)")
	flag.BoolVar(&o.StubRelay, "stub-relay", false,
		"the driver runs stubrelay, which writes no ledger: skip the ledger checks")
	flag.Var(&skip, "skip", "a lifecycle case not to run on this host (repeated)")
	exitScript := flag.String("exit-script", "", "a build script that ends the "+
		"environment without completing, for the exited case")
	flag.DurationVar(&o.CaseDeadline, "case-deadline", 0, "how long a lifecycle "+
		"case may run before it fails as hung and the tier ends (default: the "+
		"slowest case's worst path, "+
		driverconformance.DefaultCaseDeadline(60*time.Second).String()+")")
	flag.Parse()
	if *path == "" {
		fmt.Fprintln(os.Stderr, "driver-conformance: --driver is required")
		os.Exit(2)
	}
	o.DriverArgs, o.Skip = args, skip
	if *exitScript != "" {
		data, err := os.ReadFile(*exitScript)
		if err != nil {
			fmt.Fprintln(os.Stderr, "driver-conformance:", err)
			os.Exit(2)
		}
		o.ExitScript = string(data)
	}
	s, err := driverconformance.New(*path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "driver-conformance:", err)
		os.Exit(2)
	}
	failed := false
	if *tier == "static" || *tier == "all" {
		failed = report("static", s.Static()) || failed
	}
	if *tier == "lifecycle" || *tier == "all" {
		failed = report("lifecycle", s.Lifecycle(o)) || failed
	}
	_ = s.Close()
	if failed {
		os.Exit(1)
	}
}

// report prints one line per result of tier, and returns whether one failed.
func report(tier string, results []driverconformance.Result) bool {
	failed := false
	for _, r := range results {
		status := "ok  "
		switch {
		case r.Failure != "":
			status, failed = "FAIL", true
		case r.Skipped != "":
			status = "skip"
		}
		fmt.Printf("%s %s: %s (%s)", status, tier, r.Check, strings.Join(r.Requirements,
			", "))
		if msg := r.Failure + r.Skipped; msg != "" {
			fmt.Printf(": %s", msg)
		}
		fmt.Println()
	}
	return failed
}
