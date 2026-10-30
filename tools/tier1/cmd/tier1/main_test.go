package main

import (
	"bytes"
	"testing"

	"github.com/buildwarden/buildwarden/tools/tier1/report"
)

func TestUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"-nope"},
		{"stray"},
		{"rules", "-nope"},
	} {
		var out, errb bytes.Buffer
		if got := run(args, &out, &errb); got != report.ExitUsage {
			t.Errorf("%v: exit %d, want %d (stderr %q)", args, got, report.ExitUsage,
				errb.String())
		}
	}
}
