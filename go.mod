module github.com/podhmo/go-scan

go 1.24

toolchain go1.24.3

require (
	github.com/google/go-cmp v0.7.0
	golang.org/x/mod v0.27.0
	golang.org/x/sync v0.16.0
)

require (
	github.com/BurntSushi/toml v1.4.1-0.20240526193622-a339e1f7089c // indirect
	golang.org/x/exp/typeparams v0.0.0-20231108232855-2478ac86f678 // indirect
	golang.org/x/tools v0.35.0 // indirect
	golang.org/x/tools/go/expect v0.1.1-deprecated // indirect
	honnef.co/go/tools v0.6.1 // indirect
)

tool (
	golang.org/x/tools/cmd/goimports
	honnef.co/go/tools/cmd/staticcheck
)
