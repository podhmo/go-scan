# `symgo` Refinement Plan 2: A Plan to Resolve Analysis Failures on Complex Packages

## Introduction

This document provides a concrete action plan to resolve the timeout issue that occurs during the `find-orphans` e2e test. The plan is based on the corrected analysis in [`docs/trouble-symgo-refine2.md`](./trouble-symgo-refine2.md), which identifies the root cause as `symgo`'s inability to analyze the `minigo` package, a complex but independent package in the same workspace.

## Basic Strategy

The top priority is to make `symgo` capable of analyzing the `minigo` package. This will likely resolve the most critical errors (`identifier not found`, infinite recursion) that cause the timeout. Other, less severe bugs in symbol resolution can be addressed afterward. The strategy is to create a focused test case that isolates this specific analysis problem and iterate on a fix.

Performance tuning will be considered separately, only after the e2e test can run to completion.

## Corrected Task List

### Task 1: Fix Analysis Failure on `minigo` Package (Top Priority)

*   **Goal**: Enable `symgo` to successfully analyze the `minigo` package without errors or infinite recursion.
*   **Details**:
    *   This task addresses the core issue identified in Groups 3, 4, and 5 of the trouble report, where `symgo` fails to analyze the `minigo` package.
    *   The likely cause is that `minigo`, as a complex package, uses advanced language features or structural patterns that `symgo`'s analysis does not yet support.
    *   **Proposed Fix**: Create a new, minimal test case that does nothing but run the `symgo.Interpreter` on the `minigo` package. This test should reproduce the `identifier not found` and `infinite recursion` errors in a controlled environment. Use this test to debug the resolution and evaluation loop in `symgo`. The fix will likely involve improving support for the specific complex types, interfaces, or scoping patterns used in `minigo`.
*   **Acceptance Criteria**:
    1.  The new, focused unit test that runs `symgo` on `minigo` passes without errors or timeouts.
    2.  Running the full `find-orphans` e2e test no longer produces the errors from Groups 3, 4, and 5 when analyzing the `minigo` package.

### Task 2: Strengthen Standard Library Symbol Resolution

*   **Goal**: Enable `symgo` to correctly resolve function-typed variables, such as `flag.Usage`.
*   **Details**: This task is unchanged from the previous plan and addresses the `not a function: TYPE` error from **Group 1**. The resolver must be taught to handle variables whose type is a function signature.
*   **Acceptance Criteria**:
    1.  A unit test that calls `flag.Usage()` passes.
    2.  The Group 1 errors no longer appear in the e2e test.

### Task 3: Correctly Represent Multi-Return Symbolic Values

*   **Goal**: Ensure that `symgo` correctly represents the result of a multi-value function call as a symbolic tuple.
*   **Details**: This task is unchanged and addresses the `expected multi-return value...` warnings from **Group 2**. The symbolic placeholder for a multi-return function must be a tuple-like object.
*   **Acceptance Criteria**:
    1.  A unit test that assigns the result of `os.Open()` to two variables passes without warnings.
    2.  The Group 2 warnings no longer appear in the e2e test.

### Task 4: Implement a Timeout Flag in `find-orphans`

*   **Goal**: Add a command-line timeout feature to `find-orphans` to facilitate future debugging.
*   **Details**: This task is unchanged. Add a `--timeout` flag that uses `context.WithTimeout`.
*   **Acceptance Criteria**:
    1.  Running `find-orphans --timeout 1ms` exits immediately with a timeout error.
    2.  The flag is documented.

### Task 5: Full Verification and Final Bug Hunt

*   **Goal**: Confirm that the fixes allow the e2e test to run to completion and identify any remaining issues.
*   **Details**: This task is unchanged. After fixing the `minigo` analysis issue and other bugs, run the full e2e test without a manual `timeout`.
*   **Acceptance Criteria**:
    1.  `make -C examples/find-orphans e2e` completes successfully in under 60 seconds.
    2.  No error logs are produced.
