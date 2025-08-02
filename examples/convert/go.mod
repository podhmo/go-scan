module github.com/podhmo/go-scan/examples/convert

go 1.24

require (
	github.com/google/go-cmp v0.7.0
	github.com/podhmo/go-scan v0.0.0-20250801212757-b46a643f644b
	golang.org/x/tools v0.35.0
)

require (
	golang.org/x/mod v0.26.0 // indirect
	golang.org/x/sync v0.16.0 // indirect
)

replace github.com/podhmo/go-scan => ../../

replace github.com/podhmo/go-scan/examples/convert/sampledata/generated => ./sampledata/generated
