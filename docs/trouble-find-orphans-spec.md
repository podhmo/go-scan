# Trouble Report: `find-orphans` Spec and Implementation

This document outlines the current state, remaining issues, and attempted solutions for the `find-orphans` task.

## Current State & Work Completed

The primary goal was to implement the `find-orphans` task from `TODO.md` and address any in-progress items. A significant amount of time was spent clarifying the exact desired behavior with the user, especially for library mode.

The following has been completed:

1.  **New Specification (`examples/find-orphans/spec.md`)**: A detailed specification document was created to define the behavior of the `find-orphans` tool. This document was created and refined through multiple iterations with the user. It clarifies:
    -   The concepts of "Analysis Scope" and "Target Scope".
    -   The behavior of Application Mode vs. Library Mode.
    -   The new, user-specified logic for Library Mode where exported functions are not automatically considered "used".
    -   The tool's error handling strategy (log and continue).
    -   A link to the `symgo` engine's specification document.

## Remaining Issues & Failing Tests

Despite the changes, two key tests continue to fail, pointing to a deeper issue in the symbolic execution engine's call-graph tracing.

1.  **`TestFindOrphans_WithIncludeTests` Fails**:
    -   **Symptom**: In `app` mode, a function named `TestShouldBeOrphan` located in a non-test file (`not_a_test.go`) is not reported as an orphan, even though it is never called.
    -   **Analysis**: This test runs in `app` mode, where the only entry point is `main.main`. The `TestShouldBeOrphan` function is not an entry point and is not called. The filtering logic correctly identifies it as a non-test function that *should* be reported if unused. The failure implies that it is being incorrectly added to the `usageMap`. The reason for this is unclear, as the call graph from `main.main` should not reach it.

2.  **`TestFindOrphans_intraPackageMethodCall` Fails**:
    -   **Symptom**: In `lib` mode, an exported method `ExportedMethod` calls an unexported method `unexportedMethod`. The test correctly expects `ExportedMethod` to be an orphan (since nothing calls it), but it *incorrectly* expects `unexportedMethod` to be considered "used". The test fails because `unexportedMethod` is also reported as an orphan.
    -   **Analysis**: This indicates that the symbolic execution trace starting from `ExportedMethod` is failing to register the call to `unexportedMethod`. The `interp.Apply()` call is not tracing the method call inside the function body as expected. This points to a fundamental issue in `symgo`'s ability to trace method calls in this context.

## Attempts and Dead Ends

-   The primary focus was on fixing the filtering and entry point logic in `find-orphans/main.go`. While these changes were necessary to match the user's desired specification for library mode, they did not resolve the two failing tests.
-   Multiple attempts to debug the `usageMap` contents via logging were unsuccessful due to issues with the test runner's log capture.
-   The two persistent failures suggest the root cause is not in the high-level logic of the `find-orphans` tool, but deeper within the `symgo` interpreter's implementation of call tracing, particularly for method calls. Resolving this will likely require debugging the `symgo` package itself, which was beyond the scope of the attempted fixes.
