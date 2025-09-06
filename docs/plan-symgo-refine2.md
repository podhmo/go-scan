# `symgo` Refinement Plan 2: A Plan to Resolve Analysis Failures on Complex Packages

## Introduction

This document provides a concrete action plan to resolve the timeout issue that occurs during the `find-orphans` e2e test. The plan is based on the corrected analysis in [`docs/trouble-symgo-refine2.md`](./trouble-symgo-refine2.md), which identifies the root cause as `symgo`'s inability to analyze the `minigo` package, a complex but independent package in the same workspace.

## Basic Strategy

The top priority is to make `symgo` capable of analyzing the `minigo` package. This will likely resolve the most critical errors (`identifier not found`, infinite recursion) that cause the timeout. Other, less severe bugs in symbol resolution can be addressed afterward. The strategy is to create a focused test case that isolates this specific analysis problem and iterate on a fix.

Performance tuning will be considered separately, only after the e2e test can run to completion.

## Corrected Task List

### Task 1: Fix Analysis Failures (Completed)

*   **Goal**: Enable `symgo` to successfully analyze the entire workspace, including complex packages like `minigo` and tools like `find-orphans` analyzing itself, without errors or infinite recursion.
*   **Resolution**: This overarching issue was resolved by a combination of fixes that addressed both a package scoping bug and overly broad analysis of the standard library.
    1.  **Package Scoping Fix**: The `symgo` evaluator was creating placeholder package objects with an unenclosed environment. This was corrected by ensuring these environments inherit from the `UniverseEnv`, which solved the `identifier not found` errors for unexported package members and built-ins.
    2.  **Scan Policy Implementation**: An infinite recursion error was occurring because the `find-orphans` tool was attempting to symbolically execute the entire standard library. A `ScanPolicyFunc` was implemented for the tool to restrict deep analysis to only packages within the defined workspace.
    3.  **Unit Test Regressions**: The fixes above caused regressions in several unit tests that were not correctly configured to resolve standard library packages. These tests were updated to include the `goscan.WithGoModuleResolver()` option, aligning them with the e2e test configuration.
*   **Acceptance Criteria**:
    1.  `make test` completes successfully.
    2.  `make -C examples/find-orphans e2e` completes successfully with no errors or warnings in the output.

### Task 2: Implement a Timeout Flag in `find-orphans` (Future Work)

*   **Goal**: Add a command-line timeout feature to `find-orphans` to facilitate future debugging.
*   **Details**: This task is unchanged. Add a `--timeout` flag that uses `context.WithTimeout`.
*   **Acceptance Criteria**:
    1.  Running `find-orphans --timeout 1ms` exits immediately with a timeout error.
    2.  The flag is documented.
