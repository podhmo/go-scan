module github.com/podhmo/go-scan/examples/convert-define

go 1.26.0

require (
	github.com/google/go-cmp v0.7.0
	github.com/podhmo/go-scan v0.0.3
	github.com/podhmo/go-scan/examples/convert v0.0.0
	golang.org/x/tools v0.50.0
)

require (
	golang.org/x/mod v0.41.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
)

replace github.com/podhmo/go-scan => ../../

replace github.com/podhmo/go-scan/examples/convert => ../convert
