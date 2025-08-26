# Trouble Report: `find-orphans` Spec and Implementation

This document outlines the current state, remaining issues, and attempted solutions for the `find-orphans` task.

## Current State & Work Completed

The following has been completed:

1.  **`symgo` Engine Enhancement**: The symbolic execution engine (`symgo`) was enhanced to correctly handle analysis starting from a method entry point. Previously, it failed to create a symbolic receiver (`this`/`self`), causing an "identifier not found" runtime error that terminated analysis. This fix makes the engine more robust.

2.  **`TestFindOrphans_intraPackageMethodCall` Fixed**: As a direct result of the `symgo` enhancement, this test now passes. The engine correctly traces the call from an exported method to an unexported method within the same package, and the unexported method is no longer incorrectly reported as an orphan.

3.  **Bug in `go-scan` Identified**: Extensive debugging revealed that the `TestFindOrphans_WithIncludeTests` failure is caused by an upstream bug in the `go-scan` library. When `include-tests` is false, the scanner incorrectly filters out functions from non-test files (e.g., `app.go`) if they have a "Test" prefix.

## Remaining Issues & Failing Tests

One key test continues to fail due to the upstream bug.

1.  **`TestFindOrphans_WithIncludeTests` Fails**:
    -   **Symptom**: In `app` mode with `--include-tests=false`, a function named `TestShouldBeOrphan` located in a non-test file (`not_a_test.go`) is not reported as an orphan, even though it is never called.
    -   **Analysis**: The root cause has been traced to the `goscan.Scanner`. When `WithIncludeTests(false)` is active, the scanner provides an incomplete list of functions to the `find-orphans` analyzer, omitting `TestShouldBeOrphan`. The analyzer cannot report a function as an orphan if it is never made aware of its existence. The logic within `find-orphans` itself appears correct, but it is acting on incomplete data.

## Next Steps

The fix for `TestFindOrphans_intraPackageMethodCall` is complete and robust. The user has suggested submitting this partial fix. The remaining `TestFindOrphans_WithIncludeTests` failure requires a separate fix within the `go-scan` or `scanner` packages.
