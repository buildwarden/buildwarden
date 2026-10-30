module github.com/buildwarden/buildwarden/tools/tier1

go 1.26.3

replace github.com/buildwarden/buildwarden/tools/workflow => ../workflow

require (
	github.com/buildwarden/buildwarden/tools/workflow v0.0.0-00010101000000-000000000000
	go.yaml.in/yaml/v3 v3.0.5
)
