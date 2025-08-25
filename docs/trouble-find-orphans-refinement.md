# Post-Mortem: Refining `find-orphans` and the `symgo` Evaluator

This document details the debugging and implementation process for refining the `symgo` evaluator to improve the accuracy of the `find-orphans` tool.

## 1. Initial Goal

The primary goal was to improve the `find-orphans` tool's accuracy. The user reported that the tool was likely failing to detect function and method usages in two specific scenarios:
1.  When a method call occurs within the argument list of another call (e.g., `f(ob.m())`).
2.  When a method or function is passed as a value (e.g., `h(ob.m)`).

Additionally, the user requested that the analysis be strictly scoped to the user's workspace, ignoring usages of or by external packages.

## 2. Implementation Results & Outlook

The task was successfully completed. Several subtle bugs in the `symgo` evaluator were identified and fixed, and the `find-orphans` tool was updated accordingly.

- **`symgo` Evaluator Fixes**:
    - **Block Statement Evaluation**: `evalBlockStatement` no longer terminates prematurely when it encounters a function call that returns a value. It now correctly continues execution unless an explicit `ast.ReturnStmt` is found.
    - **Method Chain Evaluation**: A series of bugs preventing the correct evaluation of method chains (e.g., `a.b().c()`) were fixed. `evalSelectorExpr` now correctly unwraps `*object.ReturnValue` objects from previous calls in a chain and can resolve methods on `*object.Instance` types, not just variables.
    - **Intrinsic Application**: `applyFunction` now correctly checks for and applies named intrinsics, ensuring they can reliably override the behavior of any function.

- **`find-orphans` Tool Enhancements**:
    - The usage-marking intrinsic was refactored to check all arguments of a function call, correctly detecting when functions are passed as values.
    - The intrinsic now correctly scopes its analysis, only marking functions as "used" if they belong to a package within the scanned workspace.

- **Testing**:
    - A comprehensive test case was added to the `find-orphans` suite to validate all of the above scenarios.
    - A focused regression test was added to the `symgo` suite to lock in the correct `evalBlockStatement` behavior.

The `symgo` engine is now significantly more robust and accurate, which will benefit all tools that rely on it, including `find-orphans` and `docgen`.

## 3. Decision Making & Pivots

The debugging process for this task was complex and involved several pivots.

1.  **Initial Hypothesis (Incorrect)**: My first assumption was that the `find-orphans` intrinsic was simply not inspecting the arguments of function calls. I implemented a fix for this, but the primary test case (`f(ob.m())`) still failed. This was the first indication that the bug was deeper in the evaluator.

2.  **The `evalBlockStatement` Bug**: Through a process of elimination and logging, I discovered that `evalBlockStatement` was terminating its loop as soon as it encountered a `ReturnValue` object from an `ExprStmt`. My first fix for this (making `ExprStmt` return `nil`) was too broad and broke other tests in the `symgo/evaluator` suite. The pivot to a more precise fix—checking for `is *ast.ReturnStmt` inside `evalBlockStatement`—was crucial. This was the key to fixing the first reported issue.

3.  **The Method Chain Saga (Multiple Bugs)**: After fixing the block statement issue, I added a test for method chains (`a.b().c()`), which failed. This led to a series of discoveries and fixes:
    - **Pivot 1**: I initially thought the problem was that `evalSelectorExpr` couldn't resolve methods on `*object.Instance` types. I added this logic. The test still failed.
    - **Pivot 2**: I then realized that `evalSelectorExpr` wasn't unwrapping the `*object.ReturnValue` from the previous call in the chain (e.g., from `a.b()`). I added this fix. The test *still* failed.
    - **The `reset_all` Moment**: At this point, it was clear my incremental fixes were not working, likely because of interactions between them. I took the significant step of calling `reset_all()` to revert all my changes and start fresh. This was a critical pivot. By re-implementing the known-good fixes (for `evalBlockStatement`, method resolution on instances, and `ReturnValue` unwrapping) together in a clean state, the logic finally worked as a cohesive whole.

4.  **The Intrinsic Application Bug**: While writing a focused regression test for the block statement fix, I discovered that named intrinsics (`RegisterIntrinsic`) were not being correctly applied because the check was missing from `applyFunction`. This was a separate bug that the `find-orphans` tests didn't catch because `find-orphans` uses the *default* intrinsic, which is handled differently. Fixing this made the evaluator more robust and my new test pass.

## 4. Obstacles That Should Have Been Known

- **Circular Dependencies**: I wasted significant time trying to use the `scantest` harness from within the `symgo/evaluator` package. A test in package `foo` cannot import package `bar` if `bar` imports `foo`. Even though `symgo_test` is a separate package, it lives in the `symgo` directory and the Go toolchain prevents this circular dependency. The correct approach was to write a manual test without the harness, which I eventually did. I should have recognized this structural limitation earlier.
- **Test Environment Quirks**: My attempts to get debug logs from the `slog` logger were initially fruitless, as the `go test` runner was suppressing the output. This led me down a path of more primitive debugging (`panic`, etc.). Knowing the intricacies of how the test runner handles `stdout`/`stderr` and logging levels would have saved time. The final fix for the `find-orphans` test involved a simple `t.Logf` to dump the full output, which was effective.
