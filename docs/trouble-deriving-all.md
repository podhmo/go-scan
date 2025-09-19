# Investigation into `deriving-all` E2E Test Failure

This document provides a detailed chronological breakdown of the attempts to fix a bug in the `deriving-all` example, and the contradictions that were discovered in the `go-scan` library's behavior.

## Part 1: The Initial Problem & Hypothesis

-   **Initial State**: `make test` passed, but `make -C examples/deriving-all e2e` failed.
-   **Error**: `json: cannot unmarshal object into Go struct field Event.data of type models.EventData`.
-   **Initial Hypothesis**: The failure was caused by `goscan.Scanner.ScanPackage` performing a partial scan of a package (by respecting a `visitedFiles` cache) and then storing this incomplete `PackageInfo` in the main `packageCache`. This "poisoned" cache would then be used by the `Implements` check, which would fail to find the necessary methods to prove that concrete types implemented the `EventData` interface.

## Part 2: Chronological Debugging Attempts & Contradictions

### Attempt 1: Force `ScanPackage` to Perform a Full Scan
-   **Action**: Modified `ScanPackage` in `goscan.go` to always scan all files in a directory, ignoring `visitedFiles`, to prevent cache poisoning.
-   **Result**: `make test` failed. A new regression appeared in the `docgen` example.
-   **Specifics**: The test `TestDocgen_fullParameters` failed with the error `Tracer did not visit the target GetPathValue call expression`.
-   **Contradiction 1**: Making `ScanPackage` more robust by ensuring it always returned complete information broke the downstream `symgo` symbolic execution engine. This implied `symgo` was fragile and depended on the partial-scan behavior, which seemed incorrect.

### Attempt 2: Investigating the `docgen` Regression
-   **Action**: Hypothesized the `docgen` failure was a separate bug. I found a call to `ScanPackage` with an import path in `type_relation.go` (which is incorrect, as `ScanPackage` expects a directory path) and fixed it to use `ScanPackageByImport`.
-   **Result**: Even with both the full-scan `ScanPackage` and the `type_relation.go` fix, `TestDocgen_fullParameters` still failed with the exact same error.
-   **Conclusion**: The bug in `type_relation.go` was real but unrelated to the `docgen` regression. The mystery of why `symgo` breaks when `ScanPackage` performs a full scan deepened.

### Attempt 3: The "No-Cache" Minimal Fix
-   **Action**: Reverted all changes. Hypothesized that the issue wasn't `ScanPackage` returning partial info, but specifically the *caching* of it. I modified `ScanPackage` to perform a partial scan as before, but to *not* write its result to the `packageCache`.
-   **Result**: Catastrophic failure. `make test` failed with dozens of errors across the test suite, primarily within `symgo` and `goscan`.
-   **Specifics**: Errors were of the type `symgo runtime error: entry point function "..." not found in package "..."` and `s.Implements(...) expected true, got false`.
-   **Conclusion**: This proved that `symgo` and other parts of the system are highly dependent on the `packageCache` being populated by `ScanPackage`. The function has a critical role in providing information to the rest of the system.

### Attempt 4: The Core Contradiction
-   **Action**: I reverted all changes to get back to the initial "clean" state where `make test` should have passed.
-   **Result**: `make test` failed on `TestScanFilesAndGetUnscanned`.
-   **Specifics**: The test failed with `ScanPackage(core) after ScanFiles(user.go): expected Files [...item.go, ...empty.go], got [...user.go, ...empty.go, ...item.go]`.
-   **The Contradiction**: This was the most significant finding. The test was written to assert that `ScanPackage` performs a **partial scan** (respecting `visitedFiles`). It failed because the actual code was performing a **full scan** (returning all files). This directly contradicts the initial hypothesis for the `deriving-all` bug, which assumed `ScanPackage` was doing a partial scan. The codebase is in a state where its behavior is inconsistent with its own unit tests.

### Attempt 5: The "Correct Call Site" Fix
-   **Action**: Following user guidance, I left the core scanner alone and fixed the call site. I identified that `deriving-all/main.go` was the true caller of `ScanPackage` for the failing e2e test. I modified it to use `ScanPackageByImport` instead.
-   **Result**: `make test` passed (as the core scanner was untouched), but `make -C examples/deriving-all e2e` still failed with the original error.
-   **Conclusion**: This invalidated the theory that simply using the "correct" function at the top-level call site would work. It implies that cache poisoning (or some other state pollution) is happening from a different part of the process that is not immediately obvious.

### Attempt 6: The "Escape Hatch" (`ForceScanPackageByImport`)
-   **Action**: To definitively rule out cache poisoning, I created a new function `ForceScanPackageByImport` that explicitly bypasses the cache and performs a fresh, full scan. I used this in `deriving-all/main.go`.
-   **Result**: `make test` passed, but `make -C examples/deriving-all e2e` *still* failed with the same error.
-   **Conclusion**: This was the most confusing result. Even when explicitly bypassing the cache and performing a fresh, full scan of the target package, the generator *still* failed to resolve the interface implementations. This strongly suggests the problem is not cache pollution, but lies deeper.

### Attempt 7: Refactor `scanner/scanner.go` (The User's Final Hypothesis)
-   **Action**: Based on the user's final hint that the issue might be a single-file AST traversal problem, I attempted a major refactoring of `scanner/scanner.go` to use a multi-pass approach (pass 1 to find all declarations, pass 2 to resolve details).
-   **Result**: This deep, architectural change proved too complex to complete successfully in the given time. It broke numerous other unit tests related to constants, generics, and package filtering in subtle ways that I was unable to fully resolve.

## Final Summary of Unresolved Issues

The codebase is in an inconsistent state regarding the behavior of `ScanPackage`, and the root cause of the `deriving-all` failure is more complex than simple cache poisoning. The final evidence from Attempt #6 (where `ForceScanPackageByImport` still failed) suggests the problem lies in how a `PackageInfo` is constructed from a single file's AST in `scanner/scanner.go`. The multi-pass refactoring (Attempt #7) is likely the correct path forward, but it is a large and delicate task that requires more time and a deeper understanding of the scanner's various consumers. This document is intended to provide a clear record of this investigation to aid that future work.
