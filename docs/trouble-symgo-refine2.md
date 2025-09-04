# `symgo` Refinement Plan 2: Analysis of a "Dogfooding" Failure in the E2E Test

## Summary

A re-investigation into the timeout issue occurring during the `find-orphans` e2e test (`make -C examples/find-orphans e2e`) was conducted. The test was run with a 15-second timeout, and the resulting logs were analyzed in detail, incorporating critical user feedback on how to interpret the log messages.

The analysis reveals that the timeout is caused by a "dogfooding" failure: the `symgo` symbolic execution engine is unable to analyze the source code of its own core components, specifically the `minigo` package. When `symgo` attempts to parse `minigo`, it fails to resolve fundamental types, which triggers a catastrophic infinite recursion, causing the process to hang.

This document categorizes the observed errors and provides a corrected analysis for each group.

## Reproduction Steps

1.  **Modify the Makefile**: Remove the redirection in `examples/find-orphans/Makefile` to ensure logs are printed directly to the console.
    ```makefile
    e2e:
        @echo "Running end-to-end test for find-orphans..."
        go run . --workspace-root ../.. ./...
        @echo "e2e test finished."
    ```

2.  **Run the Test**: Execute the following command from the repository's root directory.
    ```sh
    timeout 15s make -C examples/find-orphans e2e
    ```
    The process will be terminated by the timeout after 15 seconds, producing a large volume of logs to stderr.

## Corrected Error Log Analysis

> **Important Note on Log Interpretation**: The user has provided a critical clarification for reading these logs. The `in_func` and `in_func_pos` fields do **not** refer to a function in the `symgo` engine's own call stack. Instead, they refer to the location within the **source code being analyzed**. For example, a log message with `in_func=EvalToplevel` means the error occurred while `symgo` was attempting to analyze the `EvalToplevel` function from the `minigo` package, not that the error is inside `symgo`'s own `EvalToplevel` execution frame. This distinction is fundamental to the analysis below.

The key to this analysis is understanding that the `in_func` and `in_func_pos` fields in the logs refer to the location in the code being **analyzed** by `symgo`, not the location within `symgo`'s own execution stack where the error occurred.

### Group 1: Standard Library Symbol Resolution Failure

*   **Symptom**: `not a function: TYPE`
*   **Log Excerpt**:
    ```
    time=... level=ERROR msg="not a function: TYPE" in_func=Usage in_func_pos=/app/examples/convert/main.go:80:3
    ```
*   **Analysis**: This analysis remains correct. `symgo` fails when analyzing code in `examples/convert/main.go` that calls `flag.Usage`. It incorrectly identifies the `flag.Usage` variable (which is of type `func()`) as a non-callable `TYPE`. This points to a weakness in resolving function-typed variables from external packages.

### Group 2: Flawed Multi-Return Value Handling

*   **Symptom**: `expected multi-return value on RHS of assignment`
*   **Log Excerpt**:
    ```
    time=... level=WARN msg="expected multi-return value on RHS of assignment" in_func=ResolvePkgPath in_func_pos=/app/locator/locator.go:539:1
    ```
*   **Analysis**: This analysis also remains correct. When analyzing `locator/locator.go`, `symgo` creates inadequate symbolic placeholders for functions that return multiple values, causing warnings during assignment.

### Group 3: Failure to Analyze `minigo`'s Core Types

*   **Symptom**: `identifier not found` when analyzing `minigo` source code.
*   **Log Excerpt**:
    ```
    time=... level=ERROR msg="identifier not found: Config" in_func=New in_func_pos=/app/minigo/evaluator/evaluator.go:426:1
    time=... level=ERROR msg="identifier not found: Environment" in_func=NewEnclosedEnvironment in_func_pos=/app/minigo/object/object.go:1116:1
    ```
*   **Corrected Analysis**: These errors occur when `symgo` is tasked with analyzing the source files of the `minigo` package (e.g., `/app/minigo/evaluator/evaluator.go`). `symgo` fails to resolve identifiers like `Config` and `Environment`, which are fundamental types defined within the very package it is analyzing. This indicates that `symgo` cannot correctly build a model of the `minigo` package's scope and contents, which is a prerequisite for any further analysis. This is the root of the "dogfooding" failure.

### Group 4: Infinite Recursion Triggered by Analyzing `minigo`

*   **Symptom**: `infinite recursion detected` when the analysis target is a function inside `minigo`.
*   **Log Excerpt**:
    ```
    time=... level=WARN msg="infinite recursion detected, aborting" in_func=EvalToplevel in_func_pos=/app/minigo/evaluator/evaluator.go:2620:1
    ```
*   **Corrected Analysis**: This is the direct cause of the timeout. The infinite recursion is not a bug *in* `symgo`'s `EvalToplevel` function itself. Rather, it's a bug that occurs when `symgo` *attempts to analyze* the `EvalToplevel` function located in `/app/minigo/evaluator/evaluator.go`. Unable to resolve the fundamental types from Group 3, the engine likely enters a non-terminating loop trying to find them.

### Group 5: State Confusion when Analyzing `minigo`

*   **Symptom**: `undefined field or method: Pos on <Symbolic: ...>`
*   **Log Excerpt**:
    ```
    time=... level=ERROR msg="undefined field or method: Pos on <Symbolic: type switch case variable *ast.BranchStmt>" in_func=evalBranchStmt in_func_pos=/app/minigo/evaluator/evaluator.go:2604:1
    ```
*   **Corrected Analysis**: This demonstrates `symgo`'s state becoming corrupted *while analyzing `minigo`'s source code*. When `symgo` analyzes the `evalBranchStmt` function, it encounters a type switch. It seems to incorrectly wrap the case variable (`*ast.BranchStmt`) in a symbolic object, then fails when it tries to access a field (`.Pos`) that only exists on the concrete AST node. This shows a breakdown in how `symgo` represents the code it is analyzing.

## Conclusion

The `find-orphans` timeout is a "dogfooding" failure. The `symgo` engine is not mature enough to analyze its own complex components, specifically the `minigo` package. This leads to a cascading failure:
1.  `symgo` fails to resolve basic types within the `minigo` source code.
2.  This resolution failure causes the symbolic execution process to enter an infinite loop.
3.  This infinite loop consumes all available time, resulting in a timeout.

While other, more minor issues exist (Groups 1 and 2), the inability for `symgo` to analyze `minigo` is the critical bug that must be fixed first.
