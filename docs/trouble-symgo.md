# `symgo` Robustness Enhancement Report

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

## 3. Validation and Remaining Issues

After implementing these fixes, the `find-orphans` example runs without any `invalid indirect`, `undefined method`, or `unary operator` errors. The evaluator is now significantly more resilient to analyzing code with incomplete type information.

However, the `find-orphans` log still shows some remaining errors, primarily:
-   `identifier not found: varDecls`
-   `expected a package, instance, or pointer on the left side of selector, but got UNRESOLVED_TYPE`

These appear to be separate issues, likely related to the `minigo` interpreter integration or other parts of the `symgo` engine, and are noted for future investigation.
