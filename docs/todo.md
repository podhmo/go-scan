# TODO List

## Improve Package Location Logic in `locator.go`

Currently, `locator.Locator.FindPackageDir` contains fallback mechanisms to resolve import paths that are outside the primary module context from which `go-scan` is initiated. This was specifically added to support `go-scan`'s own examples and testdata (e.g., `examples/derivingjson`), where an example might have its own `go.mod` file but needs to reference packages within the main `go-scan` repository structure (e.g., shared test utility packages or other examples).

**Background / Problem:**

- `go-scan` is designed to be lightweight and avoid dependencies like `go/packages`.
- The default behavior of the `Locator` is to resolve packages relative to the module root (`go.mod`) it discovers from the initial starting path.
- When scanning an example project within the `go-scan` repo that has its *own* `go.mod` (making it a separate module from `go-scan`'s main module), the locator, by default, cannot find packages belonging to the main `go-scan` module (e.g., `github.com/podhmo/go-scan/testdata/multipkg`) because they are not part of the example's module.
- The fallback logic (marked with `// TODO:` comments in `locator.go`) attempts to heuristically find the main `go-scan` repository root and resolve paths against it if the primary module resolution fails and the import path looks like one of `go-scan`'s internal paths.

**Why this is a TODO:**

- The current fallback is somewhat hardcoded (e.g., assumes `github.com/podhmo/go-scan` as a known module path) and not a general solution for arbitrary multi-module setups.
- It makes the package location logic less predictable and pure from an API perspective if `go-scan` were to be used as a library in complex projects.

**Potential Future Solutions:**

1.  **Stricter Module Boundaries:** Remove the fallback entirely. `go-scan` would only resolve packages within the module it was initialized for. This would make its behavior simpler and more predictable but would require examples/tests to be structured as part of the main module or use standard Go module replacement/vendoring techniques if they need to cross-reference.
2.  **Configurable Module Roots / Workspace Concept:**
    *   Allow the user of `go-scan` (either via CLI flags or library API) to specify multiple module roots or a "workspace" root. The locator could then attempt to resolve packages against these known roots.
    *   This could involve limited parsing of `go.work` files to understand a multi-module workspace structure, though full `go.work` support might be too complex given the "lightweight" goal.
3.  **Improved Heuristics (if fallback is kept):** If some form of fallback for internal testing/examples is deemed necessary, make it more robust or clearly documented as a development-only feature.
4.  **Re-evaluate Scope:** Clarify whether resolving packages outside the primary scanned module is a desired feature for `go-scan` as a general tool, or if it's purely for internal convenience. The design choices would follow from this clarification.

The core challenge is balancing `go-scan`'s lightweight nature (no `go/packages` dependency) with the flexibility needed for convenient development/testing and potential use in more complex multi-module Go projects.
