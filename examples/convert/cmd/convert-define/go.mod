module github.com/podhmo/go-scan/examples/convert/cmd/convert-define

go 1.24

require golang.org/x/tools v0.35.0

require (
	golang.org/x/mod v0.26.0 // indirect
	golang.org/x/sync v0.16.0 // indirect
)

// Note: The 'go-scan' dependency is implicit via the replace directive.
// 'go mod tidy' will add it to the require block later.
replace github.com/podhmo/go-scan => ../../../../
