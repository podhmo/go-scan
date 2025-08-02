module github.com/podhmo/go-scan/examples/deriving-all

go 1.24

toolchain go1.24.3

require (
	github.com/podhmo/go-scan v0.0.0
	github.com/podhmo/go-scan/examples/derivingbind v0.0.0
	github.com/podhmo/go-scan/examples/derivingjson v0.0.0
)

replace github.com/podhmo/go-scan => ../../
replace github.com/podhmo/go-scan/examples/derivingjson => ../derivingjson
replace github.com/podhmo/go-scan/examples/derivingbind => ../derivingbind
