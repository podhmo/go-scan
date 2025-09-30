# Plan: symgo Import Path Heuristic Improvement

## 1. Objective

Improve the heuristic in `symgo` for guessing package names from import paths to handle more complex cases, such as versioned paths and paths with prefixes or hyphens.

## 2. Background

The current symbolic execution engine (`symgo`) sometimes fails to resolve the correct package name from an import path. This leads to `identifier not found` errors when analyzing code that uses certain libraries. For example, the user reported an error `level=ERROR msg="identifier not found: isatty"` when analyzing a dependency on `github.com/mattn/go-isatty`.

The current heuristic needs to be enhanced to correctly infer the package name in such scenarios.

## 3. Requirements

The improved heuristic should correctly handle the following cases:

-   **Versioned Paths**:
    -   `"github.com/go-chi/chi/v5"` -> `chi`
-   **Prefixed and Hyphenated Paths**:
    -   `"github.com/mattn/go-isatty"` -> `isatty` (or `goisatty`)
-   **Subpackages (No Change)**: The existing behavior for subpackages should be preserved.
    -   `"github.com/go-chi/chi/v5/middleware"` -> `middleware`

## 4. Implementation Plan

1.  **Locate the Heuristic Logic**: Identify the code in the `symgo` package (likely within the resolver or package loading components) that is responsible for converting an import path to a package name.
2.  **Enhance the Heuristic**: Modify the logic to incorporate the new rules:
    -   If the last element of the path is a version string (e.g., `vN`), use the second-to-last element as the package name.
    -   If a package name starts with `go-`, create a candidate name by stripping the prefix.
    -   If a package name contains hyphens (`-`), create a candidate name by removing them.
    -   The final package name should be a valid Go identifier. The logic should sanitize the derived name (e.g., by removing hyphens).
3.  **Create a New Test File**: Add a new test file, `symgo/symgo_versioned_import_test.go`, to specifically test these new heuristics.
4.  **Add Test Cases**:
    -   Create a test case for `"github.com/go-chi/chi/v5"`.
    -   Create a test case for `"github.com/mattn/go-isatty"`.
    -   Create a test case for `"github.com/go-chi/chi/v5/middleware"` to ensure no regressions.
5.  **Iterate and Refine**: Run the tests and refine the implementation until all tests pass and the desired behavior is achieved.

## 5. Status

-   [ ] **In Progress**