module github.com/podhmo/go-scan/examples/deriving-all

go 1.24

toolchain go1.24.3

require (
	github.com/google/go-cmp v0.7.0
	github.com/podhmo/go-scan v0.0.0
	github.com/podhmo/go-scan/examples/derivingbind v0.0.0
	github.com/podhmo/go-scan/examples/derivingjson v0.0.0
	golang.org/x/tools v0.35.0
)

require (
	golang.org/x/mod v0.27.0 // indirect
	golang.org/x/sync v0.16.0 // indirect
)

replace github.com/podhmo/go-scan => ../../

replace github.com/podhmo/go-scan/examples/derivingjson => ../derivingjson

replace github.com/podhmo/go-scan/examples/derivingbind => ../derivingbind
