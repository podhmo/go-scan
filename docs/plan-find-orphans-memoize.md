# Memoization for `find-orphans` Performance

## Background

The `find-orphans` tool, when run in library mode (`-mode=lib`), analyzes all exported functions in a package. When analyzing multiple packages that share common internal dependencies (e.g., a common utility package with a factory function like `NewService()`), the tool would re-analyze the same shared functions for each entry point. This redundant analysis caused a significant performance bottleneck.

## Goal

Improve the performance of `find-orphans` in library mode by caching the results of function analysis. This will prevent the symbolic execution engine from re-evaluating the same function multiple times within a single run.

## Implementation Details

The solution was to implement a configurable memoization (caching) layer directly within the `symgo` symbolic execution engine.

1.  **Configurable Memoization:** A new option, `symgo.WithMemoization(bool)`, was added to the `symgo.Interpreter`. This allows tools built on `symgo` to opt-in to this behavior. It is disabled by default to avoid unintended side effects in tools that might rely on re-evaluating functions.

2.  **Cache Implementation:**
    *   A new cache, `memoizationCache`, was added to the `symgo/evaluator.Evaluator` struct. The cache is a map of `map[*object.Function]object.Object`.
    *   The key is a pointer to the `object.Function` being executed. Using the function object's pointer ensures that we are caching based on the specific function definition, not just its name.
    *   The value is the resulting `object.Object` from the function's execution. This is crucial because it caches the actual return value (e.g., an `*object.Instance` or `*object.SymbolicPlaceholder`), not just a boolean flag indicating that the function was run. This allows subsequent code to correctly interact with the cached return value.

3.  **Caching Logic:**
    *   A wrapper function, `applyFunction`, was introduced in the evaluator.
    *   When a function is about to be applied, this wrapper first checks if memoization is enabled and if a result for the given `*object.Function` already exists in the cache.
    *   If a cached result is found, it is returned immediately, and the function's body is not re-evaluated.
    *   If no result is found, the original function `applyFunctionImpl` is called to execute the function body.
    *   The result of the execution is then stored in the cache before being returned.

4.  **Integration with `find-orphans`:** The `find-orphans` tool was updated to enable the `WithMemoization(true)` option when creating its `symgo.Interpreter`.

## Verification

A new test, `TestMemoization_WithScantest`, was added in `symgo/evaluator/evaluator_memo_test.go`. This test confirms:
*   When memoization is **enabled**, a shared factory function called by two different entry points is only executed **once**.
*   When memoization is **disabled**, the same factory function is executed **twice**.

This confirms the feature is working as intended and is correctly controlled by the configuration option.
