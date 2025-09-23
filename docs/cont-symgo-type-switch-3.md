# Continuation 3: Fixing Regressions in Type-Narrowed Member Access

This document records the continuation of the work on `symgo`'s handling of type assertions, picking up from the state where the primary feature was implemented but caused regressions.

## Goal

The goal is to fix the regressions introduced while implementing support for member access on type-narrowed variables. The final implementation must pass all tests in the `./symgo/...` suite.

## Previous State

The core feature was implemented successfully, meaning that the tests in `symgo/evaluator/evaluator_if_typeswitch_test.go` now pass. This allows the evaluator to trace method calls and field access on variables narrowed to **concrete types**.

However, this change introduced numerous regressions, primarily in two areas:
1.  **Interface Method Resolution:** Tests involving symbolic resolution of interface methods began to fail (e.g., `TestEval_InterfaceMethodCall`, `TestInterfaceBinding`).
2.  **Unresolved/Anonymous Types:** Tests involving field access on unresolved or anonymous struct types began to fail, with the evaluator misinterpreting field access as a method call (e.g., `TestFeature_FieldAccessOnPointerToUnresolvedStruct`).

## Analysis and Current Hypothesis

The regressions are caused by a combination of two distinct issues in `symgo/evaluator/evaluator.go`:

1.  **Problem 1: Premature Unwrapping of Interfaces.** The logic added to `evalSelectorExpr` to handle type assertions is too aggressive. It "unwraps" a `SymbolicPlaceholder` to its concrete `Original` value immediately. While this is correct for a `v.(MyStruct)` assertion, it is incorrect for an `v.(MyInterface)` assertion. For interfaces, the placeholder must remain symbolic to allow the evaluator's interface resolution logic to function correctly.

2.  **Problem 2: Incorrect Member Lookup Order.** In `evalSelectorExprForObject` and `evalSymbolicSelection`, the logic attempts to find a **method** before it attempts to find a **field**. This is problematic for unresolved types, where method resolution is ambiguous and can fail, leading the evaluator to incorrectly create a placeholder for a "method call" even when the intended operation was a field access.

## The Blocker

I have been unable to reliably apply the necessary code changes using the available file modification tools (`replace_with_git_merge_diff`, `overwrite_file_with_block`), which have been failing consistently. A clear plan to fix the issues exists, but I am blocked on its implementation.

## Next Steps (for the next agent)

The next agent must apply a two-part fix to `symgo/evaluator/evaluator.go`. It is recommended to use a robust method (such as reading the file, modifying the content, and overwriting it completely) to ensure the changes are applied correctly.

**The required fixes are:**

1.  **Implement Conditional Unwrapping in `evalSelectorExpr`:**
    -   The logic that unwraps a `SymbolicPlaceholder` with a non-nil `Original` field must be made conditional.
    -   It should first inspect the placeholder's `TypeInfo` (`placeholder.TypeInfo()`).
    -   The unwrapping should **only** occur if `resolvedNarrowedType.Kind` is **not** `scan.InterfaceKind`.
    -   If the kind *is* an interface, the logic should proceed with the original `SymbolicPlaceholder`, allowing `evalSelectorExprForObject` to handle it.

2.  **Implement Field-First Lookup:**
    -   In `evalSelectorExprForObject` (for both the `*object.Instance` and `*object.Pointer` cases), modify the logic to search for a struct field **before** searching for a method.
    -   In `evalSymbolicSelection`, also modify the logic to search for a field on the placeholder's `TypeInfo` **before** searching for a method.

After applying these two fixes, run the entire test suite via `go test -v -timeout 60s ./symgo/...` to confirm that all tests, including the original feature tests and the regression tests, now pass.
