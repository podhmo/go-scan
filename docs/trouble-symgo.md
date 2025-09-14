# Improving `symgo` Robustness: A Troubleshooting and Implementation Plan

This document analyzes common errors encountered when using the `symgo` engine via the `find-orphans` tool. It also proposes a detailed, step-by-step plan to enhance `symgo` to handle these errors more gracefully, removing the need for complex, tool-specific configuration.

## 1. The Goal: A More Resilient `symgo`

The `find-orphans` tool is powerful, but its reliance on `symgo` currently exposes several rough edges in the symbolic execution engine. When `symgo` encounters code it cannot fully analyze (e.g., functions from unscanned standard library packages), it often returns errors.

The ideal user experience is to call `find-orphans` by only specifying the target packages for analysis (the "primary analysis scope"). The tool should work without requiring the user to:
-   Register individual "intrinsic" functions to handle calls into standard library or other external packages.
-   Manually specify every dependency that needs to be parsed for type information using `WithSymbolicDependencyScope`.

To achieve this, the `symgo` engine needs to be made more resilient. When it encounters an unknown type or function, it should "gracefully fail" by treating the unknown entity as a symbolic placeholder, allowing the analysis to continue rather than halting with an error.

## 2. Analysis of Common Errors

Running `make -C examples/find-orphans` reveals several categories of errors that stem from `symgo`'s current strictness.

-   **Error Group 1: Undefined Method on Symbolic Placeholder** (`undefined method or field: Set for pointer type SYMBOLIC_PLACEHOLDER`)
-   **Error Group 2: Unsupported Unary Operator** (`unary operator - not supported for type SYMBOLIC_PLACEHOLDER`)
-   **Error Group 3: Invalid Dereference (`invalid indirect`)** (`invalid indirect of <Unresolved Type: strings.Builder>`)
-   **Error Group 4: Selector on Unresolved Type** (`expected a package, instance, or pointer on the left side of selector, but got UNRESOLVED_TYPE`)
-   **Error Group 5: Internal Interpreter Errors** (`identifier not found: varDecls`)

## 3. Granular Implementation and Test Plan

This plan details the step-by-step process to improve the `symgo` engine's robustness. Each step includes an implementation change followed by a specific verification test.

### Phase 1: Graceful Handling of Unresolved Types

-   **Task 1.1: Fix `invalid indirect` error (Group 3)**
    -   **Implementation:** Modify `evalStarExpr` in `symgo/evaluator/evaluator.go`. Add a case to handle `*object.UnresolvedType`. If the value being dereferenced is an `UnresolvedType`, return a `SymbolicPlaceholder` instead of an error.
    -   **Verification:** Run `make -C examples/find-orphans` and `grep` the output for `invalid indirect`. The error should no longer be present.

-   **Task 1.2: Fix `selector on unresolved type` error (Group 4)**
    -   **Implementation:** Modify the `default` case in the main `switch` of `evalSelectorExpr` in `symgo/evaluator/evaluator.go`. Add a case for `*object.UnresolvedType` to return a `SymbolicPlaceholder` instead of an error.
    -   **Verification:** Run `make -C examples/find-orphans` and `grep` the output for `selector on unresolved type`. The error should no longer be present.

### Phase 2: Graceful Handling of Operations on Symbolic Values

-   **Task 2.1: Fix `unary operator` error (Group 2)**
    -   **Implementation:** Modify `evalNumericUnaryExpression` in `symgo/evaluator/evaluator.go`. Add a check at the beginning of the function to see if the operand is a `SymbolicPlaceholder`. If so, return a new `SymbolicPlaceholder`.
    -   **Verification:** Run `make -C examples/find-orphans` and `grep` the output for `unary operator - not supported`. The error should no longer be present.

-   **Task 2.2: Fix `undefined method` error (Group 1)**
    -   **Implementation:** Modify the `case *object.Pointer` block in `evalSelectorExpr`. When the pointee is a `SymbolicPlaceholder`, and a method is not found, return a *callable* `SymbolicPlaceholder` with a synthetic `UnderlyingFunc`.
    -   **Verification:** Run `make -C examples/find-orphans` and `grep` the output for `undefined method or field`. The error should no longer be present.

### Phase 3: Internal Interpreter Fixes

-   **Task 3.1: Fix `identifier not found` error (Group 5)**
    -   **Implementation:** Investigate the `identifier not found: varDecls` error. This will likely require debugging the scoping logic within `minigo/evaluator/evaluator.go`, specifically in the `registerDecls` function and its call sites.
    -   **Verification:** Run `make -C examples/find-orphans` and `grep` the output for `identifier not found`. The errors should no longer be present.

### Phase 4: Final Validation

-   **Task 4.1: Final Verification**
    -   **Implementation:** No code changes.
    -   **Verification:** Run `make -C examples/find-orphans` and confirm that the command completes with zero "ERROR" messages in the output log. Manually inspect the final output of `find-orphans` to ensure the results are still accurate and no regressions have been introduced.
