// Command warden-pause is the container driver's namespace holder: it owns the
// build's network namespace, which the iptables sidecar isolates before the
// build container joins it (docs/development/bootstrap.md, BOOT-01). It runs
// before isolation, so it does nothing: no network, no files, no children. It
// blocks until SIGTERM or SIGINT and exits 0.
package main

import (
	"os"
	"os/signal"
	"syscall"
)

func main() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	<-sig
}
