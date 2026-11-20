// Command warden-driver-container is the container driver
// (specs/driver-contract/v1.md): warden runs it to build in a container
// runtime (finch, docker or podman).
package main

import (
	"github.com/buildwarden/buildwarden/driver/container"
	"github.com/buildwarden/buildwarden/driver/drivercmd"
)

func main() { drivercmd.Main(container.Driver()) }
