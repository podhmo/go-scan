# Trouble Report: `find-orphans` Spec and Implementation

This document outlines the current state, remaining issues, and attempted solutions for the `find-orphans` task.

## Current State & Work Completed

The following has been completed:

1.  **`symgo` Engine Enhancement**: The symbolic execution engine (`symgo`) was enhanced to correctly handle analysis starting from a method entry point. Previously, it failed to create a symbolic receiver (`this`/`self`), causing an "identifier not found" runtime error that terminated analysis. This fix makes the engine more robust.

2.  **`TestFindOrphans_intraPackageMethodCall` Fixed**: As a direct result of the `symgo` enhancement, this test now passes. The engine correctly traces the call from an exported method to an unexported method within the same package, and the unexported method is no longer incorrectly reported as an orphan.

3.  **Bug in Test Setup Identified and Fixed**: Initial debugging suggested a bug in the `go-scan` library's file filtering. However, further investigation revealed the issue was a misunderstanding in the test case itself.

## Resolution of `TestFindOrphans_WithIncludeTests`

The key failing test was `TestFindOrphans_WithIncludeTests`.

-   **Initial Symptom**: A function named `TestShouldBeOrphan` located in a file named `not_a_test.go` was not being reported as an orphan when running with `--include-tests=false`.
-   **Initial (Incorrect) Analysis**: It was believed that `go-scan` was incorrectly filtering this function from a non-test file.
-   **Correct Analysis & Resolution**: A crucial oversight was that the filename `not_a_test.go` **does** end with the `_test.go` suffix. According to Go's convention, this makes it a test file. The `go-scan` library was correctly identifying it as a test file and filtering it out when `--include-tests=false`. The bug was not in the application code but in the test's premise: it expected a file that is, by convention, a test file to be treated as a non-test file.
-   **The Fix**: The fix was to rename the file in the test setup from `not_a_test.go` to `app.go`. This makes it a standard, non-test file. With this change, the `go-scan` library no longer filters it when `--include-tests=false`, the `TestShouldBeOrphan` function is correctly parsed and analyzed, and the `find-orphans` tool correctly reports it as an orphan. The test now passes and accurately reflects the desired behavior.

## Next Steps

All outstanding issues related to the `find-orphans` specification and logic have been resolved.
