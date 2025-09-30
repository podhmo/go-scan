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

## 4. Initial Implementation Plan

1.  **Locate the Heuristic Logic**: Identify the code in the `symgo` package (likely within the resolver or package loading components) that is responsible for converting an import path to a package name.
2.  **Enhance the Heuristic**: Modify the logic to incorporate new rules.
3.  **Create a New Test File**: Add a new test file, `symgo/evaluator/guess_package_name_test.go`, to specifically test these new heuristics.
4.  **Add Test Cases** for various import path styles.
5.  **Iterate and Refine**: Run the tests and refine the implementation until all tests pass.

## 5. Final Implementation Details

The initial plan to produce a single "perfect" package name proved too rigid for real-world cases like `go-isatty` (package `isatty`) vs. `go-scan` (package `goscan`). Based on user feedback, the approach was revised to generate multiple likely candidates.

1.  **Generate Multiple Candidates**:
    -   The `guessPackageNameFromImportPath` function in `symgo/evaluator/evaluator.go` was modified to return a slice of strings (`[]string`).
    -   For an import path like `github.com/mattn/go-isatty`, it now generates `["goisatty", "isatty"]`.
    -   For `github.com/podhmo/go-scan`, it generates `["goscan", "scan"]`.

2.  **Iterative Checking**:
    -   The `evalIdent` function, which resolves identifiers, was updated to iterate through the slice of candidates returned by `guessPackageNameFromImportPath`, checking each one against the identifier used in the code.

3.  **Testing**:
    -   A dedicated unit test file, `symgo/evaluator/guess_package_name_test.go`, was created to validate the multi-candidate generation logic.
    -   The tests were updated to assert against the expected slice of candidates for various import path styles.

## 6. Status

-   [x] **Completed**