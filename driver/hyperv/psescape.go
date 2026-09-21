package hyperv

import "strings"

// psEscapeSingle escapes a value for a single-quoted PowerShell string literal
// (a single quote is doubled). It lives in a build-tag-free file so both the
// Windows provisioner (runPS) and the cross-platform PowerShell script builders
// and their tests can use it.
func psEscapeSingle(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
