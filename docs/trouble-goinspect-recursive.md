# `symgo` Evaluator Debugging Journal

This document tracks the process of debugging and fixing a complex set of failures within the `symgo` symbolic evaluator.

## Initial State & Task Overview

The initial task was to fix a failing test in the `symgo` package, specifically `TestEval_LocalTypeDefinition`, and then address any other failures to get the entire suite passing. The process involved updating `TODO.md` and documenting the journey in this file.

## Debugging and Refactoring Journey

The path to a stable test suite was iterative and involved fixing several fundamental issues in the evaluator's logic.

### 1. The Pointer Dereference Problem (`*p`)

-   **Initial Symptom**: `TestEval_LocalTypeDefinition` failed with `expected pointer to point to an instance, but got *object.Variable`. This indicated that dereferencing a pointer `*p` was not correctly unwrapping the underlying variable `v` that `p` pointed to.
-   **Initial (Incorrect) Attempts**:
    -   Modifying `evalStarExpr` to return the underlying value fixed read operations but broke assignments (`*p = 1`) because the "location" information was lost.
    -   Modifying `evalAssignStmt` to handle pointer assignments fixed those specific cases but broke read operations.
-   **Root Cause**: A fundamental ambiguity in the `Eval` function. It did not distinguish between evaluating an expression for its **value (RHS)** versus its **location (LHS)**.
-   **The Fix**: A major refactoring was undertaken:
    1.  **New LHS Object Types**: Introduced internal objects (`FIELD_LHS_OBJ`, `INDEX_LHS_OBJ`, `UNRESOLVED_IDENTIFIER_OBJ`) to explicitly represent "locations" for assignment.
    2.  **`evalLHS` Function**: A new helper, `evalLHS`, was created. Its sole purpose is to evaluate an expression in an LHS context and return a location object.
    3.  **Refactored `evalAssignStmt`**: This function was completely rewritten to use `evalLHS` to get the destination and `Eval` to get the source value, cleanly separating the two concerns.
    4.  **`evalUnaryExpr` (`&` operator)**: The logic for the address-of operator was corrected. `&v` now creates a pointer to `v`'s *value*, not to the `*object.Variable` wrapper for `v`. This was the final key to fixing `TestEval_LocalTypeDefinition`.

### 2. Variable Shadowing in Nested Scopes

-   **Symptom**: `TestNestedBlockVariableScoping` failed. The test checks that `x := 2` in an inner scope correctly *shadows* an outer `x`, rather than *re-assigning* it. The test was getting `[2, 2]` instead of the expected `[1, 2]`.
-   **Root Cause**: The `evalLHS` logic for an identifier was using `env.Get()`, which searches all parent scopes. It did not respect the difference between `=` (assignment, searches all scopes) and `:=` (declaration, should only check/create in the local scope).
-   **The Fix**:
    1.  Added an `env.GetLocal()` method to `object.Environment`.
    2.  Modified `evalLHS` to accept the assignment token (`token.Token`).
    3.  If the token is `token.DEFINE` (`:=`), `evalLHS` now uses `env.GetLocal()` to check for an existing variable.
    4.  `performAssignment` was updated to use `env.SetLocal()` for `:=`, ensuring the new variable is always created in the current scope.

### 3. Broken Control Flow

-   **Symptom**: `TestEvaluator_LabeledStatement` failed because a `break` inside an `if` statement was not being propagated to the outer loop.
-   **Root Cause**: `evalIfStmt` was only propagating critical errors (`*object.Error`). It was swallowing other control-flow objects like `*object.Break`, `*object.Continue`, and `*object.ReturnValue`.
-   **The Fix**: The logic in `evalIfStmt` was changed to check for any control-flow object returned from its `then` or `else` branches and propagate it upwards, allowing statements like `break` to correctly terminate outer loops.

### 4. Panics and Minor Regressions

-   **Panics**: Several tests were panicking due to `nil pointer dereference` in `performAssignment`. This happened when `e.resolver.ResolveType()` returned `nil` and the code immediately tried to access `.Kind` on it. This was fixed with a nil-safe check.
-   **Regressions**: Along the way, several regressions were introduced and fixed:
    -   **Generics**: `forceEval` was incorrectly propagating a generic type parameter's `FieldType` onto a concrete value. A check was added to prevent this.
    -   **Interface Type Tracking**: The `PossibleTypes` map was storing `*` for unnamed pointer types. The logic was improved to create a fully-qualified name (e.g., `pkg.*MyType`).
    -   **Build Errors**: Several build errors occurred due to using the wrong package aliases (`scan` vs. `scanner` vs. `goscan`) for `Kind` constants. The final correct alias is `scan`.

After this extensive series of fixes, the test suite is significantly more stable. The remaining failures will be addressed next.