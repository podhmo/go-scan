# Plan: symgo Import Path Heuristic Improvement

## 1. Objective

Improve the heuristic in `symgo` for guessing package names from import paths to handle more complex cases, such as versioned paths and paths with prefixes or hyphens. This ensures that the symbolic execution engine can correctly resolve identifiers from unscanned packages.

## 2. Background

The `symgo` engine sometimes fails to resolve the correct package name from an import path, leading to `identifier not found` errors. This was observed with packages like `github.com/mattn/go-isatty`.

The initial approach of creating a single "perfect" heuristic proved too rigid, as different conventions exist (e.g., `go-isatty` becomes `isatty`, but `go-scan` becomes `goscan`). The final, more robust approach is to generate multiple likely candidates for the package name and try each one.

## 3. Final Implementation

1.  **Generate Multiple Candidates**:
    -   The `guessPackageNameFromImportPath` function in `symgo/evaluator/evaluator.go` was modified to return a slice of strings (`[]string`) instead of a single string.
    -   For an import path like `github.com/mattn/go-isatty`, it now generates a list of potential candidates, such as `["goisatty", "isatty"]`.
    -   The heuristic handles various patterns, including version suffixes (`/v5`), `gopkg.in` style paths, `.git` suffixes, and hyphens.

2.  **Iterative Checking**:
    -   The `evalIdent` function, which is responsible for resolving identifiers, was updated to handle the slice of candidates.
    -   It now iterates through the list of guessed names and checks if any of them match the identifier being used in the code. This makes the resolution process more flexible and resilient to different package naming conventions.

3.  **Comprehensive Testing**:
    -   A dedicated unit test file, `symgo/evaluator/guess_package_name_test.go`, was created to validate the multi-candidate generation logic.
    -   Test cases were added to cover a wide range of import path styles and ensure the correct set of candidates is generated for each.

## 4. Status

-   [x] **Completed**