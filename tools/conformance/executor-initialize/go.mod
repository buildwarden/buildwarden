module github.com/buildwarden/buildwarden/tools/conformance/executor-initialize

go 1.26.3

tool github.com/buildwarden/buildwarden-specs/executor-initialize/cmd/bw-conformance-executor-initialize

require (
	github.com/BurntSushi/toml v1.6.0 // indirect
	github.com/buildwarden/buildwarden-specs/conformance v0.1.0 // indirect
	github.com/buildwarden/buildwarden-specs/executor-initialize v0.0.0-20261116000050-bc03f6fdc96e // indirect
)

replace github.com/buildwarden/buildwarden-specs/conformance v0.1.0 => github.com/buildwarden/buildwarden-specs/conformance v0.0.0-20261116000050-bc03f6fdc96e
