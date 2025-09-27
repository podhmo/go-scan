# Issue: `find-orphans` Hangs Due to Infinite Recursion in `symgo`

This document outlines an issue where the `find-orphans` tool hangs, the investigation process, and the proposed solution.

## Problem

When running the `find-orphans` end-to-end test via `make -C examples/find-orphans`, the process times out. Verbose logging reveals that the tool enters an infinite loop, generating a massive amount of repetitive log output.

The issue appears to be an infinite recursion bug within the `symgo` symbolic execution engine, specifically in its package loading and dependency resolution logic.

## Reproduction Steps

1.  Navigate to the repository root.
2.  Run the `find-orphans` test with a timeout. The command will be terminated.
    ```bash
    timeout 5s make -C examples/find-orphans
    ```
3.  To observe the infinite logging, run the tool with verbose output and redirect to a file. The command will hang until the pipe is closed (e.g., by `head`).
    ```bash
    cd examples/find-orphans
    go run . -v --workspace-root ../.. ./... 2>&1 | head -c 1000000 > find-orphans-verbose-head.log
    ```

## Log Analysis

The verbose log file (`find-orphans-verbose-head.log`) is filled with a rapidly repeating sequence of debug messages from the `symgo/evaluator`. The key repeating messages are:

- `getOrLoadPackage: requesting package`
- `ResolvePackage: checking policy`
- `ScanPackageFromImportPath CACHE HIT`
- `ensurePackageEnvPopulated: checking package`

This pattern indicates that the evaluator is continuously trying to load and initialize packages it has already processed. The call stack is not correctly tracking which packages are currently under analysis, leading to a recursive reentry. For example, while analyzing package `A` which imports `B`, the evaluator starts analyzing `B`. If `B` (or a dependency of `B`) in turn imports `A`, the evaluator re-enters the analysis for `A` without realizing it's already in progress, leading to an infinite loop.

The issue seems to be triggered by the complex dependency graph of the entire workspace, with `examples/docgen` and `minigo/evaluator` being prominently featured in the logs before the loop becomes uncontrollable.

## Proposed Solution

The root cause is the lack of a mechanism to detect and prevent re-entrant analysis of the same package within a single evaluation stack.

The proposed solution is to introduce a recursion guard in the `symgo` evaluator. This can be implemented by:

1.  Adding a map to the `symgo.Interpreter` or `evaluator.Evaluator` to track the import paths of packages currently being evaluated in the active call stack.
2.  Before starting the evaluation of a package, the interpreter will check if the package's import path is already in this tracking map.
3.  If the path is present, it signifies a recursive entry. The interpreter should immediately stop and return a symbolic placeholder or a specific error, rather than re-evaluating the package.
4.  If the path is not present, it should be added to the map before evaluation begins.
5.  Crucially, the path must be removed from the map after its evaluation is complete (either successfully or with an error) to allow other, independent analyses to process it. This is typically done using a `defer` statement.

This change will break the infinite loop and allow the `find-orphans` analysis to complete successfully.

---

# Past Issue: `symgo` Robustness Enhancement Report

This document details the analysis and resolution of several robustness issues within the `symgo` symbolic execution engine. These issues were primarily observed when running the `find-orphans` tool on a complex codebase, which exposed edge cases in `symgo`'s handling of unresolved types and symbolic values.

## 1. The Goal: A More Resilient `symgo`

The `find-orphans` tool is a key application of the `symgo` engine. Initial testing revealed that its effectiveness was hampered by `symgo`'s strictness. When the engine encountered code it could not fully analyze (e.g., types or functions from unscanned packages), it would frequently halt with an error.

The goal of this effort was to make `symgo` more resilient, allowing it to "gracefully fail" when encountering unknown entities. Instead of erroring, the engine should treat these unknowns as symbolic placeholders and continue the analysis. This removes the need for users to provide complex, tool-specific configurations (like intrinsics or exhaustive dependency lists) just to make the analysis complete.

## 2. Problems Found and Solutions Implemented

Running `make -C examples/find-orphans` on the codebase revealed several categories of errors. The following sections describe each problem and the fix that was implemented in `symgo/evaluator/evaluator.go`.

### Problem 1: `invalid indirect` on Unresolved Types

-   **Symptom:** The `find-orphans` run produced numerous errors like:
    `level=ERROR msg="invalid indirect of <Unresolved Type: strings.Builder> (type *object.UnresolvedType)"`
-   **Analysis:** This error occurred when `symgo` attempted to evaluate a dereference expression (`*T`) where `T` was a type from an unscanned package. The evaluator correctly identified `T` as an `*object.UnresolvedType`, but the `evalStarExpr` function had no logic to handle this specific object type and fell through to a final error case.
-   **Solution:** A new case was added to `evalStarExpr` to specifically check for `*object.UnresolvedType`. When found, the function now returns a `*object.SymbolicPlaceholder`, representing an instance of that unresolved type, allowing analysis to continue without error.

### Problem 2: `undefined method or field` on Pointers to Symbolic Values

-   **Symptom:** Tests failed with the error:
    `undefined method or field: N for pointer type SYMBOLIC_PLACEHOLDER`
-   **Analysis:** This occurred when accessing a field on a pointer where the pointee was a symbolic placeholder (e.g., `p.N` where `p` is `*SymbolicPlaceholder`). The logic for selector expressions on `*object.Pointer` in `evalSelectorExpr` only handled cases where the pointee was a concrete `*object.Instance`, not a symbolic value.
-   **Solution:** The `case *object.Pointer` block in `evalSelectorExpr` was enhanced. It now detects when the pointee is a `*object.SymbolicPlaceholder` and re-uses the existing logic for handling selections on symbolic values, correctly resolving the field or method. This involved refactoring the symbolic selection logic into a new `evalSymbolicSelection` helper function for clarity and reuse.

### Problem 3: Incorrect Handling of Operations on Symbolic Values

-   **Symptom:** While not always fatal, various operators behaved incorrectly when applied to symbolic placeholders.
    -   `v.N++`: Treated the symbolic `v.N` as `0` and converted it to a concrete integer `1`.
    -   `!b`: Treated the symbolic boolean `b` as "truthy" and returned a concrete `false`.
-   **Analysis:** The `evalIncDecStmt` and `evalBangOperatorExpression` functions did not account for symbolic operands. They would default to concrete behavior instead of preserving the symbolic nature of the value.
-   **Solution:** Both functions were modified to check if their operand is a `*object.SymbolicPlaceholder`. If so, they now return a new `*object.SymbolicPlaceholder`, ensuring that the "unknown" state of the value is correctly propagated through the analysis.

### Problem 4: `identifier not found` for Named Return Values

-   **Symptom:** The `find-orphans` log produced errors like:
    `level=ERROR msg="identifier not found: varDecls"`
-   **Analysis:** This error occurred when `symgo` analyzed a Go function that used named return values. Go automatically declares these named returns as variables within the function's scope. The `symgo` evaluator was not mimicking this behavior. When a named return variable was used as a value (e.g., passed to a function like `append`) before its first explicit assignment, the evaluator could not find it in the environment.
-   **Solution:** The `extendFunctionEnv` function was modified. It now inspects the function's AST (`*ast.FuncDecl`) for named return parameters. If any are found, it pre-declares them as symbolic variables in the function's environment *before* evaluating the body, correctly mirroring Go's scoping rules.

### Problem 5: `expected a package, instance, or pointer...` on Unresolved Type

-   **Symptom:** The `find-orphans` log showed errors like:
    `level=ERROR msg="expected a package, instance, or pointer on the left side of selector, but got UNRESOLVED_TYPE"`
-   **Analysis:** This happened when `symgo` encountered a selector expression (`foo.Bar`) where `foo` resolved to a raw `*object.UnresolvedType` object. This typically occurs when accessing a symbol from a package that is not scanned (e.g., due to the scan policy). The `evalSelectorExpr` function was missing a case to handle this specific type, causing it to fall through to a generic error.
-   **Solution:** A new case for `*object.UnresolvedType` was added to the `switch` statement in `evalSelectorExpr`. This case now gracefully handles the situation by returning a `*object.SymbolicPlaceholder`, representing the unknown result of the selection, which allows analysis to continue.

### Problem 6: `undefined method` on Field Access

-   **Symptom:** The `find-orphans` log showed errors like:
    `undefined method: Value on github.com/podhmo/go-scan/minigo.Result`
-   **Analysis:** When evaluating a selector `foo.Bar`, if `foo` was a symbolic instance (`*object.Instance`), the evaluator would only check for a method named `Bar`. It never checked if `Bar` was a field of the struct.
-   **Solution:** The `case *object.Instance` block in `evalSelectorExpr` was updated. After failing to find a method, it now proceeds to check for a field with the given name using the existing `accessor.findFieldOnType` helper. This allows correct resolution of both method calls and field access on symbolic struct instances.

## 3. Validation and Remaining Issues

After implementing these fixes, the `find-orphans` example runs with significantly fewer errors. The evaluator is now much more robust in handling common Go patterns and incomplete type information.

The primary remaining error seen in the logs is `identifier not found: r`, which appears to be related to the complex handling of **method values** (using a method as a value, e.g., `r.handleConvert`, rather than calling it). This is a more subtle issue in the evaluator's environment and scope management and is noted for future investigation.

## 4. `invalid indirect` Error on `*object.ReturnValue`

**Manifestation:**

When running `find-orphans` in library mode (`--mode lib`), the symbolic execution engine logs errors like:

```
level=ERROR msg="invalid indirect of &instance<...> (type *object.ReturnValue)"
```

This error occurs in `symgo/evaluator/evaluator.go` within the `evalStarExpr` function, which handles the dereference operator (`*`).

**Analysis:**

The root cause is that the evaluator does not properly unwrap `*object.ReturnValue` objects in all contexts. `*object.ReturnValue` is a wrapper used to propagate return values from function calls up the evaluation stack.

The error occurs when an expression involving a dereference is applied to the result of a function call, for example `*MyFunc()`. The evaluation proceeds as follows:
1. `MyFunc()` is evaluated, and its result is wrapped in an `*object.ReturnValue`.
2. The `*` operator is evaluated by `evalStarExpr`.
3. `evalStarExpr` receives the `*object.ReturnValue` object but fails to "unwrap" it to get the actual value that the function returned.
4. It then attempts to perform a dereference on the `*object.ReturnValue` wrapper itself, which is not a pointer, leading to the "invalid indirect" error.

This issue is more prevalent in library mode for `find-orphans` because it analyzes many generated functions (like those from the `convert` tool) that may not be perfectly formed or may use patterns that expose this unwrapping weakness.

**Solution:**

The solution is to ensure that `evalStarExpr` correctly unwraps any `*object.ReturnValue` it receives before attempting to process the underlying value. This can be done by adding a check at the beginning of the function:

```go
func (e *Evaluator) evalStarExpr(...) object.Object {
    val := e.Eval(ctx, node.X, env, pkg)
    if isError(val) {
        return val
    }

    // ADD THIS FIX: Unwrap the return value if present.
    if ret, ok := val.(*object.ReturnValue); ok {
        val = ret.Value
    }

    // ... rest of the function
}
```

This ensures that the rest of the function operates on the actual returned value, not the wrapper, resolving the error.

---

# Issue: `symgo` Fails to Evaluate Generic Functions with Union-Type Interface Constraints (New Strategy)

This document details the debugging process for implementing support for generic functions constrained by union-type interfaces in `symgo`.

## 1. The Goal: Support for Generic Union Interfaces

The objective is to enable `symgo` to correctly analyze code that uses generic functions with union-type interface constraints, a feature introduced in Go 1.18. An example of such a constraint is:

```go
type Loginable interface {
    *Foo | *Bar
}

func WithLogin[T Loginable](t T) {
    // ...
}
```

The evaluator should be able to:
1.  Validate that a type argument provided to `WithLogin` (e.g., `*Foo`) satisfies the `Loginable` constraint.
2.  Correctly handle the type of the parameter `t` inside the function body, allowing constructs like type switches to work as expected.

## 2. Initial Attempts and Regressions

Initial implementation attempts involved making broad changes to both the `scanner` and `evaluator` packages simultaneously. While these changes brought the dedicated tests for the new feature closer to passing, they consistently introduced a wide range of regressions across the existing test suite. The cycle of fixing one test only to break another indicated a fundamental issue, likely stemming from an incorrect or incomplete data model being passed from the `scanner` to the `evaluator`.

## 3. New Strategy: Scanner-First Development

To break the cycle of regressions, a more disciplined, bottom-up strategy was adopted. The core idea is to ensure the correctness of the `scanner` in isolation before attempting to build the `evaluator` logic on top of it.

### Step 1: Isolate and Perfect the Scanner (In Progress)

-   **Action:** A new, highly-focused test file (`scanner/union_test.go`) was created. This test's sole purpose is to scan a simple Go file containing a union-type interface and meticulously validate the resulting `scanner.TypeInfo` struct.
-   **Goal:** Ensure that the `InterfaceInfo.Union` field is correctly populated with all member types, and that each member's `FieldType` correctly represents its name and pointer status.
-   **Status:** The test has been created. The immediate next step is to implement the parsing logic in `scanner/scanner.go` to make this test pass, and then to run the full repository test suite to confirm that this core change introduces no regressions.

### Step 2: Re-implement Evaluator Logic Incrementally

-   **Action:** Once the scanner is verifiably correct and stable, the evaluator logic will be re-implemented.
-   **Goal:** Make the integration tests in `evaluator_generic_union_test.go` pass.
-   **Method:** This will be done in small, incremental steps. After each logical change (e.g., updating data models, adding constraint checking, fixing the type switch), the *entire* test suite (`go test ./...`) will be run to catch any regressions immediately.

This methodical approach ensures that each layer of the system is correct before the next is built, preventing the accumulation of errors and making the source of any new regression immediately obvious.
