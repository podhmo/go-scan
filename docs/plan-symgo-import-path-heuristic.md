# `symgo`: Enhance Import Path Heuristic

## 1. Problem

The `symgo` symbolic execution engine relies on a heuristic to guess the package name from an import path when the package's source code is not scanned (e.g., it is outside the primary analysis scope and has no explicit alias). The current heuristic is too simplistic: it assumes the package name is the last segment of the import path.

This leads to incorrect name resolution for versioned import paths, causing "identifier not found" errors.

-   `"github.com/go-chi/chi/v5"` is incorrectly resolved to package name `v5` instead of `chi`.
-   `"github.com/alecthomas/kingpin/v2"` is incorrectly resolved to `v2` instead of `kingpin`.

However, the heuristic must still correctly handle paths where the last segment is the intended package name, such as:

-   `"github.com/go-chi/chi/v5/middleware"` should resolve to `middleware`.

## 2. Proposed Solution

A more robust heuristic will be implemented to correctly handle these common versioning patterns.

### 2.1. New Heuristic Function

A new private helper function, `guessPackageNameFromImportPath(path string) string`, will be added to `symgo/evaluator/evaluator.go`. Its logic will be as follows:

1.  Get the last segment of the import path.
2.  Use a regular expression (`^v[0-9]+$`) to check if this segment is a version string (e.g., `v2`, `v5`, `v10`).
3.  If it is a version string and the path has at least two segments, return the second-to-last segment as the package name.
4.  Otherwise, return the last segment as the package name.

This logic correctly handles all the cases identified above.

### 2.2. Integration

The `evalIdent` function in `symgo/evaluator/evaluator.go` will be modified. The existing code that splits the path by `/` will be replaced with a call to the new `guessPackageNameFromImportPath` function. This change will only affect the logic for packages that are not scanned from source and do not have an explicit import alias.

## 3. Testing

A new test file, `symgo/symgo_versioned_import_test.go`, will be created to validate the new heuristic. The test will use the `scantest` library to create an in-memory package that imports and uses symbols from versioned packages like `"github.com/go-chi/chi/v5"`. The test will fail until the new heuristic is correctly implemented and integrated.

This ensures the fix is robust and prevents future regressions.