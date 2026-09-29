module github.com/podhmo/go-scan

go 1.26.0

require (
	github.com/google/go-cmp v0.7.0
	github.com/iancoleman/orderedmap v0.3.0
	github.com/podhmo/flagstruct v0.6.1
	golang.org/x/mod v0.41.0
	golang.org/x/sync v0.23.0
)

require (
	github.com/BurntSushi/toml v1.6.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	golang.org/x/exp/typeparams v0.0.0-20260611194520-c48552f49976 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/telemetry v0.0.0-20260908163034-4bcc4b2ee518 // indirect
	golang.org/x/tools v0.50.0 // indirect
	golang.org/x/tools/gopls v0.23.0 // indirect
	honnef.co/go/tools v0.8.0-rc.1 // indirect
)

tool (
	golang.org/x/tools/cmd/goimports
	golang.org/x/tools/gopls/internal/analysis/modernize/cmd/modernize
	honnef.co/go/tools/cmd/staticcheck
)
