//go:build !windows

package hyperv

// App-package redirection is Windows-only; elsewhere nothing is redirected.

func finalPath(string) (string, bool) { return "", false }

func probeNewFile(string) (string, bool) { return "", false }
