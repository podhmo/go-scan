module github.com/podhmo/go-scan

go 1.24.2

toolchain go1.24.3

require (
	github.com/google/go-cmp v0.7.0
	golang.org/x/mod v0.27.0
	golang.org/x/sync v0.16.0
)

require (
	github.com/BurntSushi/toml v1.5.0 // indirect
	golang.org/x/exp/typeparams v0.0.0-20250620022241-b7579e27df2b // indirect
	golang.org/x/sys v0.34.0 // indirect
	golang.org/x/telemetry v0.0.0-20250710130107-8d8967aff50b // indirect
	golang.org/x/tools v0.35.1-0.20250728180453-01a3475a31bc // indirect
	golang.org/x/tools/gopls v0.20.0 // indirect
	honnef.co/go/tools v0.7.0-0.dev.0.20250523013057-bbc2f4dd71ea // indirect
)

tool (
	golang.org/x/tools/cmd/goimports
	golang.org/x/tools/gopls/internal/analysis/modernize/cmd/modernize
	honnef.co/go/tools/cmd/staticcheck
)
