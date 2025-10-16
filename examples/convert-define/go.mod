module github.com/podhmo/go-scan/examples/convert-define

go 1.24.2

toolchain go1.24.3

require (
	github.com/google/go-cmp v0.7.0
	github.com/podhmo/go-scan v0.0.0
	github.com/podhmo/go-scan/examples/convert v0.0.0
	golang.org/x/tools v0.35.1-0.20250728180453-01a3475a31bc
)

require (
	golang.org/x/mod v0.27.0 // indirect
	golang.org/x/sync v0.16.0 // indirect
)

replace github.com/podhmo/go-scan => ../../

replace github.com/podhmo/go-scan/examples/convert => ../convert
