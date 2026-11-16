module github.com/buildwarden/buildwarden/tools/conformance/bootstrap-config

go 1.26.3

tool github.com/buildwarden/buildwarden-specs/bootstrap-config/cmd/bw-conformance-bootstrap-config

require (
	github.com/BurntSushi/toml v1.6.0 // indirect
	github.com/buildwarden/buildwarden-specs/bootstrap-config v0.0.0-20261116000050-bc03f6fdc96e // indirect
	github.com/buildwarden/buildwarden-specs/conformance v0.1.0 // indirect
)

replace github.com/buildwarden/buildwarden-specs/conformance v0.1.0 => github.com/buildwarden/buildwarden-specs/conformance v0.0.0-20261116000050-bc03f6fdc96e
