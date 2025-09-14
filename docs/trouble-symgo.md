# Improving `symgo` Robustness: A Troubleshooting and Implementation Plan

This document analyzes common errors encountered when using the `symgo` engine via the `find-orphans` tool. It also proposes a plan to enhance `symgo` to handle these errors more gracefully, removing the need for complex, tool-specific configuration.

## 1. The Goal: A More Resilient `symgo`

The `find-orphans` tool is powerful, but its reliance on `symgo` currently exposes several rough edges in the symbolic execution engine. When `symgo` encounters code it cannot fully analyze (e.g., functions from unscanned standard library packages), it often returns errors.

The ideal user experience is to call `find-orphans` by only specifying the target packages for analysis (the "primary analysis scope"). The tool should work without requiring the user to:
-   Register individual "intrinsic" functions to handle calls into standard library or other external packages.
-   Manually specify every dependency that needs to be parsed for type information using `WithSymbolicDependencyScope`.

To achieve this, the `symgo` engine needs to be made more resilient. When it encounters an unknown type or function, it should "gracefully fail" by treating the unknown entity as a symbolic placeholder, allowing the analysis to continue rather than halting with an error.

## 2. Analysis of Common Errors

Running `make -C examples/find-orphans` reveals several categories of errors that stem from `symgo`'s current strictness.

---

### Error Group 1: Undefined Method on Symbolic Placeholder
-   **Error:** `undefined method or field: Set for pointer type SYMBOLIC_PLACEHOLDER`
-   **Cause:** The engine tries to evaluate a method call (e.g., `logLevel.Set(...)`) on a variable that is a symbolic placeholder (because `logLevel`'s value is not known). The engine cannot find the `Set` method because the type (`*slog.LevelVar`) is in an unscanned package (`log/slog`).

---

### Error Group 2: Unsupported Unary Operator
-   **Error:** `unary operator - not supported for type SYMBOLIC_PLACEHOLDER`
-   **Cause:** The engine encounters a unary expression (e.g., `-x`) where `x` is a symbolic placeholder. The engine does not know how to perform this operation on a symbolic value.

---

### Error Group 3: Invalid Dereference (`invalid indirect`)
-   **Error:** `invalid indirect of <Unresolved Type: strings.Builder>`
-   **Cause:** The engine encounters a pointer dereference (e.g., `*p`) where `p` is a variable of an unresolved type from an unscanned package. The engine errors because it cannot dereference an unknown type.

---

### Error Group 4: Selector on Unresolved Type
-   **Error:** `expected a package, instance, or pointer on the left side of selector, but got UNRESOLVED_TYPE`
-   **Cause:** The engine encounters a field or method access (e.g., `os.Stdout.Write`) where the base (`os.Stdout`) is an unresolved type because the `os` package was not scanned.

---

### Error Group 5: Internal Interpreter Errors
-   **Error:** `identifier not found: varDecls`
-   **Cause:** These errors originate deep within the `minigo` interpreter used by `symgo`. They suggest an internal logic or state management issue that is triggered when analyzing certain code paths.

## 3. Implementation and Test Plan

To address these issues and achieve the goal of a more resilient `symgo` engine, the following changes will be made to the `symgo/evaluator/evaluator.go` file.

### Step 1: Graceful Handling of Unresolved Types and Pointers

-   **File:** `symgo/evaluator/evaluator.go`
-   **Function to Modify:** `evalStarExpr` (handles pointer dereferencing `*x`)
-   **Change:** Modify the function to check if the value being dereferenced is an `object.UnresolvedType`. If it is, return a new `object.SymbolicPlaceholder` instead of the `invalid indirect` error. This will fix **Error Group 3**.
-   **Function to Modify:** `evalSelectorExpr` (handles field/method access `x.Y`)
-   **Change:** Modify the `default` case of the main switch. If the object on the left of the selector is an `object.UnresolvedType`, return a `SymbolicPlaceholder` instead of the `selector on unresolved type` error. This will fix **Error Group 4**.

### Step 2: Graceful Handling of Operations on Symbolic Values

-   **File:** `symgo/evaluator/evaluator.go`
-   **Function to Modify:** `evalNumericUnaryExpression`
-   **Change:** Add a check at the beginning of the function. If the operand is a `SymbolicPlaceholder`, return a new `SymbolicPlaceholder` immediately, preventing the `unsupported unary operator` error. This will fix **Error Group 2**.
-   **Function to Modify:** `evalSelectorExpr` (the `case *object.Pointer` block)
-   **Change:** When the code encounters a method call on a pointer to a `SymbolicPlaceholder`, and the method cannot be found (because it's in an unscanned package), the function should return a *callable* `SymbolicPlaceholder` with a synthetic `UnderlyingFunc`. This will allow the `applyFunction` to handle the call gracefully, fixing the `undefined method` error from **Error Group 1**.

### Step 3: Investigate and Fix Internal Errors

-   **File:** `minigo/evaluator/evaluator.go`
-   **Change:** The `identifier not found: varDecls` error (**Error Group 5**) points to a deeper issue in the `minigo` interpreter. A detailed investigation of the `registerDecls` function and its call sites will be required to understand the scoping problem and implement a fix.

### Step 4: Verification and Testing

-   **Test Plan:**
    1.  After each of the implementation steps above, re-run `make -C examples/find-orphans`.
    2.  Use `grep "ERROR " examples/find-orphans/find-orphans.out` to confirm that the corresponding error group has been eliminated.
    3.  After all changes are made, perform a final run to ensure that the command completes with zero "ERROR" messages in the output log.
    4.  Manually inspect the final output of `find-orphans` to ensure that the results are still accurate and that the changes have not introduced any regressions.

By implementing this plan, the `symgo` engine will become significantly more robust and user-friendly, aligning with the goal of a low-configuration, high-power static analysis tool.
