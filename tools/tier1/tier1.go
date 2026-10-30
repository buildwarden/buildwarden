// Package tier1 carries the files tier 1 is configured by, embedded so the
// binary checks any checkout, including branches that predate tools/tier1.
package tier1

import _ "embed"

// RulesYAML is rules.yaml: the rule-to-check map and the coupling path data.
//
//go:embed rules.yaml
var RulesYAML []byte

// GolangciYAML is golangci.yml: the diff-scoped tier-1 golangci-lint config.
//
//go:embed golangci.yml
var GolangciYAML []byte
