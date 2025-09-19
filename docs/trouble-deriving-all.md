# Trouble: `deriving-all` e2e Test Failure and Scanner Contradictions

This document outlines a bug discovered in the `deriving-all` example, and the subsequent debugging investigation which revealed fundamental contradictions in the `go-scan` library's behavior.

## Part 1: The Initial Problem

The `make -C examples/deriving-all e2e` command fails with the following error:
```
main_test.go:155: Unmarshal() failed: json: cannot unmarshal object into Go struct field Event.data of type models.EventData
```
This error indicates that the code generator failed to produce a custom `UnmarshalJSON` method that could correctly handle the `EventData` interface. This typically happens when the generator cannot identify all the concrete types that implement the interface.

## Part 2: My Mental Model and How It Was Invalidated

My debugging process was driven by a mental model of how the scanner *should* work. This model was proven wrong by the test results, leading to an unresolvable situation.

### My Initial Mental Model
1.  **`ScanPackage` vs. `ScanPackageByImport`**: `ScanPackage` is a low-level function for scanning a directory. `ScanPackageByImport` is a higher-level, more robust function that handles caching and resolution via import paths.
2.  **The `visitedFiles` Cache**: The `goscan.Scanner` instance maintains a `visitedFiles` map. This is used to prevent re-parsing the same file multiple times during a single session.
3.  **The `packageCache`**: The scanner also maintains a `packageCache` to store the `PackageInfo` results of scans, keyed by import path.
4.  **The Hypothesized Bug**: `ScanPackage` respected `visitedFiles`, causing it to perform partial scans. It then incorrectly cached these partial `PackageInfo` results in the `packageCache`. This "poisoned" the cache. Later, a call to a function like `Implements` (or a `symgo` operation) would use `ScanPackageByImport`, hit the poisoned cache, retrieve incomplete information, and fail.
5.  **The "Obvious" Fix**: My proposed fix was to change `ScanPackage` to always perform a *full* scan of all files in its directory, ignoring `visitedFiles`. This would ensure it always returned a complete `PackageInfo`, fixing the cache poisoning problem at its source.

### How Reality Contradicted the Model

My attempts to implement this "obvious" fix led to a series of contradictions.

#### Contradiction A: The `docgen` Regression
-   **Action**: I implemented the "obvious" fix, making `ScanPackage` perform a full scan.
-   **Result**: `make test` failed. The `TestDocgen_fullParameters` test threw the error `Tracer did not visit the target GetPathValue call expression`.
-   **Analysis**: This was a major contradiction. Making the scanner's behavior more robust and predictable should not break a downstream tool. It implied that `symgo` (used by `docgen`) was somehow *dependent* on the buggy, partial-scan behavior I was trying to fix. This seemed highly illogical and fragile, and I could not find a reason for this dependency in the `symgo` or `docgen` source code.

#### Contradiction B: The `TestScanFilesAndGetUnscanned` Failure
-   **Action**: To isolate the `docgen` regression, I reverted all my changes to get back to a clean, original state.
-   **Result**: `make test` failed immediately on `TestScanFilesAndGetUnscanned`. The error was:
    ```
    goscan_test.go:784: ScanPackage(core) after ScanFiles(user.go): expected Files [...item.go, ...empty.go], got [...user.go, ...empty.go, ...item.go]
    ```
-   **Analysis**: This was the most critical contradiction, which invalidated my entire mental model.
    1.  The test `TestScanFilesAndGetUnscanned` is written to explicitly verify the "partial scan" behavior. It first scans `user.go`, then calls `ScanPackage` on the directory, and asserts that the result contains **only the other two files**.
    2.  The test failed because `ScanPackage` returned **all three files**.
    3.  This means that the `ScanPackage` implementation in the "original" codebase was **already performing a full scan**.

## Part 3: The Final Unresolved State

The codebase, as provided, is in a logically inconsistent state.

1.  The `deriving-all` e2e test fails. The symptoms strongly point to a problem caused by **partial scanning**.
2.  The `goscan` unit tests fail. The failure proves that `ScanPackage` is **already performing a full scan**.

It is impossible for `ScanPackage` to be both doing a partial scan and a full scan simultaneously. One of the test outcomes is providing misleading information about the function's behavior, or there is a subtle environmental factor that I cannot see.

Because I cannot establish a reliable, repeatable baseline for `ScanPackage`'s behavior, I cannot create a fix that I can prove is correct. Any change I make to satisfy one test causes another, seemingly unrelated, test to fail.

This document serves as a record of this investigation. The core issue appears to be the internal inconsistency in the codebase's behavior and its own tests. Resolving this will likely require a deeper understanding of the intended interaction between `ScanPackage`, `ScanFiles`, the caching layers, and `symgo`.
