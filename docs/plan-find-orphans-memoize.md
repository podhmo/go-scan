# Plan: Memoize `find-orphans` Analysis via `symgo`

## 1. Problem

The `find-orphans` tool, particularly in library mode (`--mode=lib`), analyzes every exported function as a potential entry point. This leads to significant performance overhead when multiple entry points share common internal dependencies. For example, if exported functions `A()` and `B()` both call an internal function `C()`, the current implementation will symbolically execute `C()` twice. This redundancy grows with the number of shared functions and entry points, slowing down the analysis.

## 2. Goal

Introduce a memoization (caching) mechanism to avoid re-analyzing functions that have already been symbolically executed within a single `find-orphans` run. This will significantly improve performance, especially for large codebases in library mode.

## 3. Proposed Solution

The memoization logic will be implemented directly within the `symgo` symbolic execution engine, rather than in the `find-orphans` tool itself. This makes the optimization more robust, reusable, and cleanly separated from the tool's specific logic.

The core idea is to add a cache to the `symgo/evaluator.Evaluator` that tracks which `*object.Function` instances have already had their bodies analyzed.

### 3.1. Implementation Details

The implementation will consist of three main steps:

1.  **Modify the `Evaluator` Struct**:
    -   In `symgo/evaluator/evaluator.go`, a new map field will be added to the `Evaluator` struct:
        ```go
        analysisMemo map[*object.Function]bool
        ```
    -   This map will use the pointer to an `object.Function` as a key, which uniquely identifies a specific function definition. The value will be a boolean `true` to indicate that the function has been analyzed.

2.  **Initialize the Cache**:
    -   In the `New` constructor for the `Evaluator` in `symgo/evaluator/evaluator.go`, the new `analysisMemo` map will be initialized.
        ```go
        e := &Evaluator{
            // ... existing fields
            analysisMemo:           make(map[*object.Function]bool),
        }
        ```

3.  **Add Memoization Logic to `applyFunction`**:
    -   The primary evaluation function, `applyFunction` in `symgo/evaluator/evaluator.go`, is the ideal location for the cache check.
    -   At the beginning of the `case *object.Function:` block within `applyFunction`, the following logic will be inserted:
        -   **Check Cache**: Check if the current function object `fn` is present in `e.analysisMemo`.
        -   **Cache Hit**: If `fn` is found in the cache, it means the function's body has already been evaluated. The function will immediately return a symbolic placeholder (e.g., `&object.ReturnValue{Value: &object.SymbolicPlaceholder{Reason: "memoized function call"}}`). This stops further evaluation of the function's body, preventing redundant analysis of its internal calls. A debug log message will be added to confirm the cache hit.
        -   **Cache Miss**: If `fn` is not in the cache, it will be added to `e.analysisMemo` immediately (`e.analysisMemo[fn] = true`). The evaluation will then proceed as normal.

## 4. Expected Outcome

-   Each function body within the analysis scope will be symbolically executed at most once per `find-orphans` run.
-   The overall execution time for `find-orphans` in library mode will be significantly reduced.
-   The change is self-contained within `symgo`, ensuring that any other tools using the `symgo` library can also benefit from this performance improvement.
