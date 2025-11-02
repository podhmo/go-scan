# Plan: goinspect from anywhere

This document outlines the plan to allow `goinspect` to run from outside a Go module, for example, from the root directory (`/`), and to inspect packages from the standard library or the module cache.

## Background

Currently, `goinspect` (and the underlying `go-scan` library) assumes it is running within a Go module. It relies on `go.mod` to determine the module root and resolve dependencies. This prevents users from inspecting standard library packages or other cached modules without creating a dummy `go.mod` file.

The goal is to make `goinspect` more versatile, behaving more like the official `go` tool, which can be invoked from anywhere.

## Plan

### 1. Detect Execution Context

In `goinspect`'s `main` function, we will first determine the execution context:

- **Module Context**: A `go.mod` file is found in the current directory or any parent directory. This is the current behavior.
- **No-Module Context**: No `go.mod` file is found.

We can use `go list -m -f {{.Dir}}` or a similar mechanism to find the module root. If it fails, we are in a no-module context.

### 2. Implement No-Module Execution Logic

When in a no-module context, `goinspect` will adopt a different package resolution strategy.

#### 2.1. Standard Library Packages

If the input package path (e.g., `fmt`, `net/http`) belongs to the standard library, `goinspect` should locate it in `GOROOT`.

- We can use `go list -f '{{.Dir}}' <pkg>` to get the directory of a standard library package.
- This path will be passed to the `go-scan` scanner.

#### 2.2. Third-Party Packages (from Module Cache)

If the input package path is not in the standard library (e.g., `github.com/google/go-cmp/cmp`), `goinspect` will need to resolve it from the Go module cache.

As suggested in the prompt, we can implement the following logic:
1.  Create a temporary directory (e.g., in `/tmp/goinspect-XXXX`).
2.  Inside the temporary directory, run `go mod init temp`.
3.  Run `go get <package>@latest` to ensure the package is downloaded to the module cache.
4.  Run `go list -f '{{.Dir}}' <package>` to get the package's source code location in the module cache (`GOPATH/pkg/mod/...`).
5.  Pass this directory to the `go-scan` scanner.

This approach avoids polluting the user's current directory with a `go.mod` file and correctly resolves versioned dependencies.

### 3. Refactor `go-scan` for Context-Awareness

The core `go-scan` library may need adjustments to support scanning directories that are not part of a single, unified module.

- The `goscan.NewScanner()` function and its configuration (`goscan.Config`) will be reviewed.
- We might need a new constructor or option to pass a list of package directories directly, bypassing the module-based discovery mechanism.
- The `locator.Locator` will need to be configured to resolve import paths based on these explicitly provided directories (`GOROOT`, module cache paths) instead of just the main module's vendor/dependency graph.

### 4. Update `goinspect` CLI

- The `goinspect` `main.go` will be the primary place for these changes.
- We will orchestrate the context detection and package location logic there.
- The flags and arguments will be parsed first, and then the appropriate scanner will be configured and run.

### 5. Add Tests

A new integration test file for `goinspect` will be created to validate the new functionality.

- **Test Case 1: Standard Library**:
    - Run `goinspect` from a temporary directory without a `go.mod`.
    - Target a standard library package like `fmt`.
    - Assert that the call graph for `fmt.Println` is correctly generated.
- **Test Case 2: Third-Party Package**:
    - Run `goinspect` from a temporary directory without a `go.mod`.
    - Target a third-party package like `github.com/google/go-cmp/cmp`.
    - Assert that the call graph for a function within that package is correctly generated.

## TODO in `TODO.md`

A new section will be added to `TODO.md` to track this feature.

```markdown
### `goinspect`: Standalone Execution
- [ ] **Run without go.mod**: Allow `goinspect` to run from outside a Go module to inspect standard library and third-party packages.
- [ ] **Context Detection**: Implement logic to detect whether `goinspect` is running in a module or no-module context.
- [ ] **Standard Library Resolution**: Add support for finding and scanning standard library packages from `GOROOT`.
- [ ] **Module Cache Resolution**: Add support for resolving and scanning third-party packages from the module cache by creating a temporary module.
- [ ] **Integration Tests**: Add tests for running `goinspect` in a no-module context.
```