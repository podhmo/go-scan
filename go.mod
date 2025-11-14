module github.com/podhmo/go-scan

go 1.24.2

toolchain go1.24.3

require (
	github.com/google/go-cmp v0.7.0
	golang.org/x/mod v0.29.0
	golang.org/x/sync v0.17.0
)

require (
	github.com/BurntSushi/toml v1.5.0 // indirect
	github.com/iancoleman/orderedmap v0.3.0 // indirect
	github.com/podhmo/flagstruct v0.6.1 // indirect
	github.com/spf13/pflag v1.0.7 // indirect
	golang.org/x/exp/typeparams v0.0.0-20250620022241-b7579e27df2b // indirect
	golang.org/x/sys v0.36.0 // indirect
	golang.org/x/telemetry v0.0.0-20250908211612-aef8a434d053 // indirect
	golang.org/x/tools v0.37.0 // indirect
	golang.org/x/tools/gopls v0.20.0 // indirect
	honnef.co/go/tools v0.7.0-0.dev.0.20250523013057-bbc2f4dd71ea // indirect
)

tool (
	golang.org/x/tools/cmd/goimports
	golang.org/x/tools/gopls/internal/analysis/modernize/cmd/modernize
	honnef.co/go/tools/cmd/staticcheck
)
