# Trouble Report: `minigo` Package Resolution Failure in Nested Test Modules

This document provides a detailed analysis of a persistent package resolution failure encountered when using the `minigo` interpreter within a `go test` environment that involves multiple, nested Go modules.

## Goal

The high-level goal was to add a feature to `docgen` that used `minigo` to load a configuration file. This configuration file needed to import a package from within its own test-specific Go module.

## The Problematic Structure

The core of the issue lies in the interaction between three different Go modules during a test run.

**1. Directory Layout:**

```
/ (root of go-scan project)
├── go.mod (Module: github.com/podhmo/go-scan)
│
└── examples/
    └── docgen/
        ├── go.mod (Module: github.com/podhmo/go-scan/examples/docgen)
        │
        ├── main_test.go (The test being executed)
        │
        └── testdata/
            └── fn-patterns/
                ├── go.mod (Module: github.com/podhmo/go-scan/examples/docgen/testdata/fn-patterns)
                │
                ├── patterns.go (The minigo script being evaluated)
                │
                └── api/
                    └── api.go (The package that patterns.go tries to import)
```

**2. Module Definitions:**

- **Root `go.mod`**: `module github.com/podhmo/go-scan`
- **`docgen` `go.mod`**: `module github.com/podhmo/go-scan/examples/docgen`
  - Contains `replace github.com/podhmo/go-scan => ../../`
- **`fn-patterns` `go.mod`**: `module github.com/podhmo/go-scan/examples/docgen/testdata/fn-patterns`
  - Contains `replace github.com/podhmo/go-scan => ../../../../`

**3. Code Interaction:**

- **`main_test.go`** (in `docgen` module) runs a test, `TestDocgen_withFnPatterns`.
- This test calls `docgen`'s `LoadPatternsFromConfig` function.
- `LoadPatternsFromConfig` creates a `minigo` interpreter. Crucially, it's configured to use the `fn-patterns` directory as its working directory:
  ```go
  // in main_test.go
  moduleDir := "testdata/fn-patterns"
  minigoOpts := minigo.WithScannerOptions(goscan.WithWorkDir(moduleDir))
  customPatterns, err := LoadPatternsFromConfig(patternsFile, logger, minigoOpts)
  ```
- The `minigo` interpreter then evaluates **`fn-patterns/patterns.go`**.
- **`fn-patterns/patterns.go`** tries to import the `api` package:
  ```go
  // in patterns.go
  import "github.com/podhmo/go-scan/examples/docgen/testdata/fn-patterns/api"
  ```
- The `api` package is defined in **`fn-patterns/api/api.go`** as `package api`.

## The Failure

When `go test` is run from the `examples/docgen` directory, the `minigo` interpreter fails to evaluate `patterns.go`.

**Exact Error Message:**
```
failed to load custom patterns: failed to evaluate patterns config source: runtime error: undefined: api.API (package scan failed: could not get unscanned files for package github.com/podhmo/go-scan/examples/docgen/testdata/fn-patterns/api: could not find package directory for "github.com/podhmo/go-scan/examples/docgen/testdata/fn-patterns/api" (tried as path and import path): import path "github.com/podhmo/go-scan/examples/docgen/testdata/fn-patterns/api" could not be resolved. Current module is "github.com/podhmo/go-scan/examples/docgen/testdata/fn-patterns" (root: /app/examples/docgen/testdata/fn-patterns))
```

### Analysis of Failure

The error message is contradictory and reveals the core of the problem.
- It correctly identifies the `Current module` as `.../fn-patterns` with the correct root directory. This indicates that the `WithWorkDir` option is working as intended.
- However, it then claims it `could not find package directory` for an import path that should be resolvable *within that exact module*.

This strongly suggests that the `go-scan/locator` component, when invoked by a `minigo` interpreter that itself is running inside a `go test` process from a *different* parent module, is unable to correctly layer the filesystem and module contexts. The `go test` process's primary module context (`examples/docgen`) seems to be interfering with the `go-scan` locator's ability to resolve packages within the nested test module (`fn-patterns`), even when explicitly told that the nested module is its workspace.

## Chronicle of Failed Attempts

1.  **Changing Test Execution Directory**: Running `go test` from `/` vs. `/examples/docgen` made no difference. The pathing errors changed, but the fundamental resolution failure remained.
2.  **Simplifying Module Structure**: I removed the `fn-patterns/go.mod` file and treated `fn-patterns` as a standard Go package within the `docgen` module. This failed because the `patterns.go` script needs to import `github.com/podhmo/go-scan/examples/docgen/patterns`, which now creates a circular dependency within the `docgen` module. The submodule approach is necessary to break this cycle.
3.  **Isolating the Test**: Moving the test logic to a separate `*_test.go` file had no effect.
4.  **Verifying `minigo` Standalone**: The `minigo` interpreter's own test suite passes flawlessly, including tests for `replace` directives. This confirms the issue is not with `minigo` in isolation, but with its use in this specific nested test context.

## Conclusion & Hypothesis

The issue is not a simple misconfiguration but likely a deeper limitation or bug in the `go-scan` package locator when used in a complex, multi-module `go test` environment. The locator seems unable to fully detach from the primary test process's module context.

**Recommended Next Steps:**

The most direct way to solve this is to fix the underlying issue in the `go-scan` locator. However, a pragmatic workaround for testing the `docgen` feature is to **avoid file-based module resolution entirely**.

A better test strategy would be:
1.  In `main_test.go`, read the contents of `api.go` and `patterns.go` into strings.
2.  Create a `minigo` interpreter.
3.  Use `minigo.LoadGoSourceAsPackage("api", apiSource)` to manually load the `api` package into the interpreter's memory. This bypasses the filesystem locator.
4.  Use `minigo.EvalString(patternsSource)` to evaluate the patterns script. Since the `api` package is now in memory, the import should resolve.

This creates a hermetic test for the `minigo` evaluation and `docgen` loading logic without depending on the fragile nested module resolution via the filesystem.

---

## Post-Mortem and Final Solution (2025-08-22)

The initial analysis was correct that the `minigo` interpreter was not using the correct module context. However, the initial proposed workaround (using in-memory files) was a temporary fix. The true, robust solution involved a deeper refactoring.

### Root Cause Re-evaluation

The fundamental problem was that the `minigo` interpreter created its own `go-scan.Scanner` instance, which was completely disconnected from the scanner instance used by the host tool (e.g., `docgen` or a test harness like `scantest`). This meant that even if the host's scanner was correctly configured for a nested module, the interpreter's scanner was not.

### The Fix

The fix was implemented in two main parts:

1.  **Scanner Injection in `minigo`**: The `minigo.NewInterpreter` function was enhanced with a new option, `minigo.WithScanner(*goscan.Scanner)`. This allows the host application to create and configure a single `goscan.Scanner` and share it with the interpreter. When this option is used, the interpreter adopts the host's scanner context, including its working directory, module root, and `go.mod` overlay.

2.  **Refactoring Host Tools**: Tools that use `minigo` were updated to use this new pattern.
    -   The `docgen` loader (`loader.go`) was changed to accept a `*goscan.Scanner` and pass it to the interpreter.
    -   The `docgen` command-line tool (`main.go`) was updated to create a scanner with a `WorkDir` set to the directory of the patterns file, ensuring correct resolution for user-provided scripts.
    -   Tests, especially the new integration test, were written to use this scanner-sharing pattern, proving its effectiveness.

This approach is more robust because it ensures a single source of truth for module and file resolution throughout the application and its embedded scripting components. The final working solution is detailed in `docs/summary-minigo-import-test.md`.
