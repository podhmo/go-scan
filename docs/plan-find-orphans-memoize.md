# Memoization for `find-orphans` Performance

## Background

The `find-orphans` tool, when run in library mode (`-mode=lib`), analyzes all exported functions in a package. When analyzing multiple packages that share common internal dependencies (e.g., a common utility package with a factory function like `NewService()`), the tool would re-analyze the same shared functions for each entry point. This redundant analysis caused a significant performance bottleneck.

## Goal

Improve the performance of `find-orphans` in library mode by caching the results of function analysis. This will prevent the symbolic execution engine from re-evaluating the same function multiple times within a single run.

## Final Implementation

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

---

## Development and Testing Journey

The implementation of this feature involved a significant debugging and learning process. The key challenges and lessons are documented here for future reference.

### 1. CWD (Current Working Directory) Instability

**Problem**: During testing, commands like `go test ./...` and `make test` produced inconsistent results or failed with `no such file or directory` errors.
**Discovery**: The root cause was that the `run_in_bash_session` tool's Current Working Directory (CWD) was not always the project root (`/app`) as assumed. It was sometimes set to a subdirectory, which limited the scope of the test commands.
**Lesson**: Always verify the CWD with `pwd` if file-related errors occur. For project-wide operations, use the canonical Makefile targets (`make test`, `make format`) after ensuring the CWD is `/app` (e.g., by running `cd /app && make test`). This is a known environmental quirk noted in `AGENTS.md`.

### 2. Test Design: How to Test Memoization Without Interference

**Problem**: The tests for the memoization feature failed repeatedly, even after the core logic seemed correct. The test was consistently reporting that the memoized function was being called multiple times when it should have been called only once.

**Discovery**: The initial test design was flawed.
*   **The Flawed Approach**: The test used an `*object.Intrinsic` to replace the function being tested (`NewService`) and count its calls.
*   **The Root Cause**: The `symgo` evaluator's intrinsic system is a complete override. When the evaluator saw the call to `NewService`, it immediately resolved it to the `*object.Intrinsic`. The core memoization logic in `applyFunction`, which was designed to work on `*object.Function` types, was never executed because it never saw the original function object. The test was completely bypassing the system it was meant to validate.

**Solution**:
The test was redesigned to avoid this interference.
1.  A "hook" or "spy" function (`Tally()`) was added to the source code being tested.
2.  This `Tally()` function was called from within the body of the target function (`NewService`).
3.  The call-counting intrinsic was registered on `Tally()`, not `NewService`.

This new design allowed `NewService` to be resolved as a normal `*object.Function` and flow through the memoization cache as intended. The test could then correctly verify that the function body (and thus the `Tally()` hook) was only executed the expected number of times.

**Lesson**: When testing a mechanism like caching or memoization, ensure the test's observation method does not interfere with or bypass the mechanism itself. Using spy functions inside the target's body is a more robust pattern than replacing the target function entirely.

### 3. Build and Dependency Errors

The development process was also plagued by a series of self-inflicted build errors due to typos in package imports, incorrect assumptions about library APIs (`scantest.NewRunner`), and confusion over type aliases (`scanner.Package` vs. `scanner.PackageInfo`). This underscores the importance of careful reading of existing code, documentation (`AGENTS.md`), and error messages.
