# `find-orphans`: Handling Workspaces with Multiple `main` Packages

## Problem

The `find-orphans` tool is designed to operate in two primary modes: "application mode" and "library mode".

-   **Application Mode**: Starts analysis from one or more `main.main` functions.
-   **Library Mode**: Starts analysis from all exported functions and `init` functions.

When running in "auto" mode, the tool detects the presence of a `main.main` function and switches to application mode.

A bug was identified where, in a workspace containing multiple modules with `main` packages (e.g., a repository with multiple commands in a `cmd/` directory), the tool would only identify the *last* `main.main` function it encountered. It would then proceed in application mode using only that single entry point.

This led to incorrect results: functions that were only called by the other, ignored `main` functions were incorrectly reported as orphans.

## Solution

The core of the issue was that the analyzer stored the discovered entry point in a single variable (`mainEntryPoint`), which was overwritten each time a new `main` package was found.

The fix involved the following changes:

1.  **Collect All Main Entry Points**: The analyzer was modified to store all discovered `main.main` functions in a slice (`mainEntryPoints`) instead of a single variable.

2.  **Update Analysis Logic**: The mode selection logic was updated to handle the slice of entry points:
    -   In `auto` or `app` mode, if the `mainEntryPoints` slice is not empty, the tool uses *all* functions in the slice as the starting points for the analysis.
    -   In `lib` mode, in addition to the exported functions, *all* discovered `main.main` functions are also added as entry points to ensure that functions they use are not flagged as orphans.

This ensures that in a multi-command workspace, all `main` functions are treated as valid entry points, leading to a correct and complete call-graph analysis. A test case (`TestFindOrphans_MultiMain`) was added to verify this specific scenario and prevent future regressions.
