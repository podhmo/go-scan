# Trouble: `deriving-all` e2e Test Failure due to Incomplete Package Scan

This document outlines a bug discovered while running the e2e tests for the `deriving-all` example, its root cause in the `go-scan` library, and the subsequent investigation.

## Problem

The `make -C examples/deriving-all e2e` command fails with the following error:

```
--- FAIL: TestUnmarshalEvent (0.00s)
    --- FAIL: TestUnmarshalEvent/user_created_event (0.00s)
    main_test.go:155: Unmarshal() failed: json: cannot unmarshal object into Go struct field Event.data of type models.EventData
```

This error indicates that the custom `UnmarshalJSON` method for the `Event` struct is not being generated correctly. The `Event` struct contains a field `Data EventData`, where `EventData` is an interface. The code generator is failing to identify the concrete types (`UserCreated`, `MessagePosted`) that implement this interface.

## Initial Hypothesis: Partial Scans Poisoning the Cache

The root cause was hypothesized to be a bug in `goscan.Scanner.ScanPackage`. This function is responsible for scanning a directory and returning its `PackageInfo`. The theory was that `ScanPackage` respected the `visitedFiles` cache, leading to it returning a partial `PackageInfo` if some files in a package had already been scanned. This partial info would then be placed in the main `packageCache`, "poisoning" it and causing later operations like `Implements()` to fail.

---

## Debugging Log & Contradictions

The effort to fix the bug based on the initial hypothesis revealed a series of deep contradictions in the scanner's behavior and its interaction with other parts of the system.

### Attempt 1: Force `ScanPackage` to Perform a Full Scan

-   **Action**: Modified `ScanPackage` in `goscan.go` to always scan all files in a directory, ignoring the `visitedFiles` cache. The goal was to prevent cache poisoning by ensuring `ScanPackage` always returned and cached a complete `PackageInfo`.
-   **Result**: This caused a major regression. `make test` failed with the following error:
    ```
    --- FAIL: TestDocgen_fullParameters (0.08s)
        main_test.go:338: Tracer did not visit the target GetPathValue call expression
    FAIL
    FAIL	github.com/podhmo/go-scan/examples/docgen	0.723s
    ```
-   **Contradiction 1**: Why does making `ScanPackage` more robust and predictable (always returning complete information) break a complex downstream tool like `symgo`/`docgen`? It implies that `symgo` depends on the fragile, partial-scanning behavior of `ScanPackage`, which seems like a flawed design.

### Attempt 2: Investigating the `docgen` Regression

-   **Action**: I hypothesized the `docgen` failure was due to a separate bug. I found a call to `ScanPackage` with an import path in `type_relation.go` and fixed it to use the correct `ScanPackageByImport` function.
-   **Result**: Even with both the `ScanPackage` full-scan fix and the `type_relation.go` fix applied, the `docgen` test *still* failed with the exact same error (`Tracer did not visit...`).
-   **Contradiction 2**: The bug in `type_relation.go`, while a valid issue, was not the cause of the `docgen` regression. This deepened the mystery of why `symgo` breaks when `ScanPackage` performs a full scan.

### Attempt 3: The "No-Cache" Minimal Fix

-   **Action**: Reverted all changes. Hypothesized that the issue wasn't `ScanPackage` returning partial info, but specifically the *caching* of it. I modified `ScanPackage` to perform a partial scan as before, but to *not* write its result to the `packageCache`.
-   **Result**: This was catastrophic. `make test` failed with dozens of errors across the test suite, primarily within `symgo`, all with messages like:
    ```
    --- FAIL: TestEval_LocalTypeDefinition (0.01s)
        evaluator_local_type_test.go:31: symgotest: test failed unexpectedly: symgo runtime error: entry point function "Do" not found in package "example.com/m"
    --- FAIL: TestImplements (0.00s)
        --- FAIL: TestImplements/SimpleStruct_SimpleInterface (0.00s)
            goscan_test.go:952: s.Implements(SimpleStruct, SimpleInterface): expected true, got false
    ```
-   **Contradiction 3**: This proves that `symgo` and other parts of the system are highly dependent on the `packageCache` being populated. By preventing `ScanPackage` from caching, I starved the system. This means `ScanPackage` is indeed a key part of the information-gathering process that other tools rely on, making its buggy, partial-caching behavior even more problematic.

### The Core Contradiction

-   **Action**: I reverted all changes to get back to the initial "clean" state where `make test` should have passed.
-   **Result**: `make test` failed immediately on `TestScanFilesAndGetUnscanned`.
    ```
    --- FAIL: TestScanFilesAndGetUnscanned/ScanPackage_RespectsVisitedFiles (0.00s)
        goscan_test.go:784: ScanPackage(core) after ScanFiles(user.go): expected Files [...item.go, ...empty.go], got [...user.go, ...empty.go, ...item.go]
    ```
-   **The Contradiction**: This is the most significant contradiction. The test failure shows that `ScanPackage` is **actually performing a full scan** (returning all 3 files), not a partial one. The test was expecting a partial result (2 files) and failed because it received a full one. This directly contradicts the initial hypothesis for the `deriving-all` bug, which assumed `ScanPackage` was doing a partial scan. The codebase is in a state where its behavior is inconsistent with its own unit tests.

### Conclusion of Debugging

The codebase appears to be in an internally inconsistent state. The `ScanPackage` function's behavior seems to change depending on the context, or its main unit test (`TestScanFilesAndGetUnscanned`) is asserting a behavior that the function does not actually have. I cannot resolve this fundamental contradiction with the available information, and therefore cannot produce a fix that reliably passes all tests. This detailed log is provided for future investigation.
