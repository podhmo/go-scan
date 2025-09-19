# Trouble: `deriving-all` e2e Test Failure and Scanner Inconsistency

This document provides a detailed chronological breakdown of the attempts to fix a bug in the `deriving-all` example, and the contradictions that were discovered in the `go-scan` library's behavior.

## Part 1: The Initial Problem & Hypothesis

-   **Initial State**: At the start of the task, `make test` passed, but `make -C examples/deriving-all e2e` failed.
-   **Error**: `json: cannot unmarshal object into Go struct field Event.data of type models.EventData`.
-   **Analysis**: This error indicated that the code generator failed to produce a custom `UnmarshalJSON` method that could correctly handle the `EventData` interface. This typically happens when the generator cannot identify all the concrete types that implement the interface.
-   **Initial Hypothesis**: The failure was caused by `goscan.Scanner.ScanPackage` performing a partial scan (by respecting a `visitedFiles` cache) and then storing this incomplete `PackageInfo` in the main `packageCache`. This "poisoned" cache would then be used by subsequent operations (like the `Implements` check), which would fail due to the incomplete information.

## Part 2: Chronological Debugging Attempts & Contradictions

My attempts to fix the bug based on the initial hypothesis led to a series of contradictions.

### Attempt 1: Force `ScanPackage` to Perform a Full Scan
-   **My Mental Model**: The `ScanPackage` function's partial-scan behavior is a bug. It should always return a complete view of a package to be robust.
-   **Action**: I modified `ScanPackage` in `goscan.go` to always scan all files in a directory, ignoring `visitedFiles`.
-   **Result**: This caused a major regression. `make test` failed.
-   **Specifics**: The test `TestDocgen_fullParameters` in `examples/docgen/main_test.go` failed with the error `Tracer did not visit the target GetPathValue call expression`.
-   **Contradiction**: Making `ScanPackage` more robust and predictable broke the downstream `symgo` symbolic execution engine. This implied `symgo` was somehow dependent on the fragile, partial-scan behavior, which seemed illogical.

### Attempt 2: Investigating the `docgen` Regression
-   **My Mental Model**: The `docgen` failure might be a separate, pre-existing bug exposed by my change.
-   **Action**: I found a bug in `type_relation.go` where `ScanPackage` was being called with an import path instead of a directory path. I fixed it to use the correct `ScanPackageByImport` function.
-   **Result**: Even with both the full-scan `ScanPackage` and the `type_relation.go` fix applied, the `docgen` test *still* failed with the exact same error.
-   **Conclusion**: The bug in `type_relation.go`, while a valid issue, was not the cause of the `docgen` regression.

### Attempt 3: The "No-Cache" Minimal Fix
-   **My Mental Model**: If the full-scan is problematic, perhaps the issue is not the partial *return* from `ScanPackage`, but specifically the *caching* of that partial result.
-   **Action**: I modified `ScanPackage` to perform a partial scan as before, but to *not* write its result to the `packageCache`, to avoid poisoning it.
-   **Result**: Catastrophic failure of `make test`.
-   **Specifics**: Dozens of tests failed, primarily within `symgo`, with errors like `symgo runtime error: entry point function "..." not found in package "..."`.
-   **Conclusion**: This proved that `symgo` and other parts of the system are highly dependent on the `packageCache` being populated by `ScanPackage`.

### Attempt 4: The "Correct Call Site" Fix (User's Suggestion)
-   **My Mental Model**: My initial assumption was wrong. As the user suggested, `ScanPackage` is *intended* to be partial, and callers needing a full scan should use `ScanPackageByImport`. The bug must be a misuse of `ScanPackage` at the call site.
-   **Action**: I identified that `deriving-all/main.go` was the true caller of `ScanPackage` for the failing e2e test. I modified it to resolve the directory to an import path and use `ScanPackageByImport`.
-   **Result**: `make test` passed, but `make -C examples/deriving-all e2e` *still failed* with the original error.
-   **Conclusion**: This invalidated the theory that simply using the "correct" function at the top-level call site would work, likely because the cache was still being polluted by some other part of the process.

### Attempt 5: The "Escape Hatch" (`ForceScanPackageByImport`)
-   **My Mental Model**: The cache must be getting polluted somehow, somewhere. The only way to be sure is to explicitly bypass it.
-   **Action**: I created a new function `ForceScanPackageByImport` that explicitly bypasses the cache and performs a fresh, full scan. I used this in `deriving-all/main.go`.
-   **Result**: `make test` passed, but `make -C examples/deriving-all e2e` *still* failed with the same error.
-   **Conclusion**: This was the most confusing result. Even when explicitly bypassing the cache and performing a fresh, full scan, the generator *still* failed. This points away from cache poisoning and towards a deeper bug.

### Attempt 6: The Single-File Traversal Hypothesis (User's Final Hint)
-   **My Mental Model**: The user provided a new hypothesis: the problem is not about multiple files, but about the AST traversal being order-dependent *within a single file*.
-   **Action**: I attempted a major refactoring of the low-level `scanner/scanner.go` to use a multi-pass approach (pass 1 to find all declarations, pass 2 to resolve details).
-   **Result**: This deep, architectural change proved too complex. It broke numerous other unit tests related to constants, generics, and package filtering in subtle ways that I was unable to fully resolve.

## Part 3: Final Unresolved State

The investigation concluded without a successful fix. The final, and most likely correct, hypothesis is that there is an order-of-declaration dependency bug in the single-pass AST traversal logic within `scanner/scanner.go`. However, fixing this requires a significant and delicate refactoring of the scanner's core, which proved to have too many unintended side effects on other parts of the system that I was unable to resolve.

The key takeaway is that the problem is not a simple caching issue, but a fundamental architectural one in the low-level scanner. This document serves as a record of this investigation to aid future work on this complex issue.
