module github.com/podhmo/go-scan/examples/docgen/testdata/fn-patterns

go 1.24

replace github.com/podhmo/go-scan/examples/docgen => ../../../

replace github.com/podhmo/go-scan => ../../../../../

require github.com/podhmo/go-scan/examples/docgen v0.0.0-00010101000000-000000000000

require (
	github.com/podhmo/go-scan v0.0.0-00010101000000-000000000000 // indirect
	golang.org/x/mod v0.27.0 // indirect
	golang.org/x/sync v0.16.0 // indirect
)
