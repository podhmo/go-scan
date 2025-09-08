> [!NOTE]
> This feature has been implemented.

# Plan: Robust Interface Resolution in `symgo`

This document outlines the plan to implement a robust, two-phase deferred resolution mechanism for interface method calls in the `symgo` symbolic execution engine.

## 1. The Problem

The current implementation of `symgo` fails to correctly identify all concrete implementations of an interface method, particularly when those implementations exist in different packages and are not directly instantiated in the code being analyzed. This leads to tools like `find-orphans` incorrectly reporting used methods as unused.

## 2. The Goal

The goal is to implement a two-phase mechanism:

-   **Phase 1: Collection:** During symbolic execution, record all method calls made on variables that are statically typed as interfaces.
-   **Phase 2: Finalization:** After execution, use the collected data to find all possible concrete implementations for each called interface method across all scanned packages, and mark them as "used".

## 3. Original Plan

The original high-level plan was as follows:

1.  **Analyze and Prepare:** Confirm existing test failures and understand the scope of the issue.
2.  **Update Core `symgo` Documentation:** Update `docs/analysis-symgo-implementation.md` with the new design.
3.  **Implement Collection Logic:** Modify `evalSelectorExpr` to record calls on interface-typed variables into a new `calledInterfaceMethods` map in the `Evaluator`.
4.  **Implement Finalization Logic:** Create a new public `Finalize()` method on the `Evaluator`. This method would:
    -   Collect all struct and interface definitions from all scanned packages.
    -   Build a map of which structs implement which interfaces.
    -   Iterate through `calledInterfaceMethods` and mark the concrete methods on all implementers as "used".
5.  **Add Comprehensive Tests:** Create a new test file, `symgo/symgo_interface_resolution_internal_test.go`, to specifically validate the new mechanism with various scenarios (value/pointer receivers, multiple implementers).
6.  **Fix Existing Tests:** Modify the `find-orphans` tool to call the new `Finalize()` method, which should fix the existing `TestFindOrphans_interface` failure.
7.  **Submit** the final, working changes.

## 4. Implementation History and Challenges (Update)

The execution of the plan proved to be extremely challenging. While the high-level design is believed to be sound, a series of implementation errors and issues with the development tools led to an impasse.

### Attempt 1: Naive Post-Evaluation Check

-   **Hypothesis:** A check could be added at the end of `evalSelectorExpr`.
-   **Result:** Failed. The evaluator had already resolved the interface variable to its concrete type, so the check never detected an interface.

### Attempt 2: Pre-Evaluation Static Type Check

-   **Hypothesis:** The check must happen at the top of `evalSelectorExpr` before the receiver is evaluated.
-   **Implementation:** `if e.typeOf(expr.X).IsInterface() { ... }`
-   **Result:** Failed. The `calledInterfaceMethods` map remained empty. Logs suggested that `e.typeOf()` or its underlying dependencies were failing to resolve the type of the variable at that point in the evaluation.

### Attempt 3: The Impasse and Tooling Failures

-   **Hypothesis:** My understanding of the required code was correct, but my repeated, small patches were corrupting the file state. A single, comprehensive patch after a full `reset_all()` would be the most reliable way forward.
-   **Proposed Design:** A detailed, multi-file patch to `evaluator.go` and `accessor.go` was formulated to implement the full logic correctly in one go.
-   **Result:** This is where the process broke down completely. I repeatedly failed to construct the correct `replace_with_git_merge_diff` commands due to:
    -   Using incorrect file paths.
    -   Providing incorrect `SEARCH` blocks due to losing track of the file's state.
    -   Introducing new syntax/build errors during the patching process itself.
    -   Discovering type mismatches (`*scanner.MethodInfo` vs. `*scanner.FunctionInfo`) only after applying large patches, making recovery difficult.
    -   Inadvertently deleting this very plan document with a `reset_all()` command.

## 5. Current Status

The implementation is paused. The code has been reset to its original state. This document has been restored and updated to reflect the history of the implementation attempt. The next step requires a successful application of the comprehensive patch described in "Attempt 3". Assistance is required to overcome the tooling issues and apply the changes correctly.

## 6. Current Progress

Here is a summary of progress against the original plan:

-   [x] **1. Analyze and Prepare:** Complete.
-   [x] **2. Update Core `symgo` Documentation:** Complete.
-   [ ] **3. Implement Collection Logic:** In progress, but currently blocked.
-   [ ] **4. Implement Finalization Logic:** In progress, but currently blocked.
-   [ ] **5. Add Comprehensive Tests:** In progress, but currently blocked.
-   [ ] **6. Fix Existing Tests:** Not started.
-   [ ] **7. Submit:** Not started.

I am currently stuck on steps 3, 4, and 5. The core logic for these steps has been designed, but I have been unable to apply the code changes successfully due to repeated tooling errors.

For a detailed breakdown of the implementation attempts and the specific errors encountered, please see the corresponding troubleshooting document: [trouble-symgo-interface-resolution.md](./trouble-symgo-interface-resolution.md).
