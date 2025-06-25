# TODO List

## Package Location Logic in `locator.go` (Resolved)

The `locator.Locator.FindPackageDir` previously contained fallback mechanisms to resolve import paths outside the primary module context. This was primarily to support examples like `examples/derivingjson` which, having their own `go.mod`, needed to reference other packages within the main `go-scan` repository (e.g., `github.com/podhmo/go-scan/examples/derivingjson/testdata/separated/shapes`).

This fallback logic has been **removed** from `locator.go` to promote stricter module boundaries and more predictable behavior, aligning with standard Go module practices.

For cases like `examples/derivingjson` that need to reference other local packages within the same repository but across module boundaries (as defined by their own `go.mod` files), the solution is to use `replace` directives in their respective `go.mod` files.

For example, `examples/derivingjson/go.mod` now contains:
```go
module github.com/podhmo/go-scan/examples/derivingjson

// ... other directives ...

replace github.com/podhmo/go-scan => ../../
replace github.com/podhmo/go-scan/examples/derivingjson/testdata/separated/shapes => ./testdata/separated/shapes
```
This approach makes package resolution explicit and relies on standard Go tooling, removing the need for custom, heuristic fallbacks in `locator.go`. The previous "Potential Future Solutions" regarding this specific fallback are now largely addressed by this change.
