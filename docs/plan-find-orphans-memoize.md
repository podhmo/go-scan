# Implemented: Memoize `find-orphans` Analysis via a Configurable `symgo` Option

## 1. Problem

The `find-orphans` tool, particularly in library mode (`--mode=lib`), analyzes every exported function as a potential entry point. This leads to significant performance overhead when multiple entry points share common internal dependencies. For example, if exported functions `A()` and `B()` both call an internal function `C()`, the original implementation would symbolically execute `C()` twice. This redundancy grows with the number of shared functions and entry points, slowing down the analysis.

## 2. Goal

A memoization (caching) mechanism was introduced to avoid re-analyzing functions that have already been symbolically executed within a single run. This significantly improves performance for tools like `find-orphans`.

This optimization was implemented carefully to avoid breaking other tools (like `docgen`) that might rely on re-evaluating functions to obtain concrete return values.

## 3. Implemented Solution

The memoization logic was implemented directly within the `symgo` symbolic execution engine as a **configurable, opt-in feature**. This provides the best balance of performance for tools that can use it, and correctness for tools that cannot. By default, memoization is **disabled** to ensure backward compatibility and prevent unexpected behavior in existing tools.

### 3.1. Implementation Details

1.  **Made Memoization Configurable in `symgo`**:
    -   In `symgo/symgo.go`, a new option function, `WithMemoization(enabled bool)`, was added.
    -   A corresponding `memoize` boolean field was added to the `Interpreter` struct. Its default value is `false`.
    -   The `NewInterpreter` constructor passes this option down to the underlying `evaluator`.

2.  **Implemented Caching in the `symgo` Evaluator**:
    -   In `symgo/evaluator/evaluator.go`, the `Evaluator` struct was modified to include a `memoize` flag and an `analysisMemo` map (`map[*object.Function]bool`).
    -   The `New` constructor was updated to accept the `memoize` option and initialize the map.
    -   The core `applyFunction` method was modified. When the `memoize` flag is `true`, it now checks the `analysisMemo` cache before executing a function's body.
        -   **Cache Hit**: If the function is in the cache, its execution is skipped, and a placeholder value is returned.
        -   **Cache Miss**: If the function is not in the cache, it is added to the cache, and execution proceeds normally.

3.  **Enabled Memoization in `find-orphans`**:
    -   In `examples/find-orphans/main.go`, the call to `symgo.NewInterpreter` was updated to include `symgo.WithMemoization(true)`, explicitly opting in to the performance improvement.

4.  **Added a Verification Test**:
    -   A new test file, `symgo/evaluator/evaluator_memo_test.go`, was created.
    -   This test verifies the configurability of the feature. It runs two sub-tests:
        -   One with memoization disabled (the default), asserting that a shared internal function is executed multiple times.
        -   One with memoization enabled, asserting that the same internal function is executed only once.

## 4. Outcome

-   A new, optional `WithMemoization` flag is available on the `symgo.Interpreter`.
-   The `find-orphans` tool uses this flag to gain a significant performance improvement.
-   Other tools are unaffected by default, ensuring no breaking changes.
-   The new behavior is verified by a dedicated test.
