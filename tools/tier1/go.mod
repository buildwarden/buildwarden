module github.com/buildwarden/buildwarden/tools/tier1

go 1.26.3

replace github.com/buildwarden/buildwarden/tools/workflow => ../workflow

require go.yaml.in/yaml/v3 v3.0.5
