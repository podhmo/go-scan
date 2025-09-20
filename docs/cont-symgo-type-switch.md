# Continuation: Enhancing `symgo` for Type-Narrowed Member Access

## Initial Prompt

The user requested that I select and implement a task from `TODO.md`, specifically a `symgo` task from `docs/plan-symgo-type-switch.md`. The instructions were to implement the feature, add tests, ensure the tests pass, and update `TODO.md` upon completion or to document progress if the task could not be completed.

(User prompt was in Japanese, this is a faithful translation).

## Goal

The goal is to enhance the `symgo` symbolic execution engine to correctly trace method calls and field accesses on variables whose types have been narrowed within a `switch v := i.(type)` statement or an `if v, ok := i.(T); ok` type assertion. The key is to resolve members based on the narrowed, concrete type, not the original interface type.

## Initial Implementation Attempt

My first approach followed a Test-Driven Development (TDD) methodology as outlined in `docs/plan-symgo-type-switch.md`.

1.  **Created Test File:** I created a new test file, `symgo/evaluator/evaluator_if_typeswitch_test.go`.
2.  **Added Failing Tests:** I successfully added two failing tests to this file:
    *   `TestTypeSwitch_MethodCall`: This test verifies that a method call on a variable narrowed in a type switch is correctly traced.
    *   `TestIfOk_FieldAccess`: This test verifies that a field access on a variable narrowed in an `if-ok` assertion is correctly traced.
    These tests failed as expected, confirming that the current implementation does not handle these cases.

My initial implementation attempt focused on modifying the `symgo/evaluator/evaluator.go` file. The plan was to:
1.  Add an `Original` field to the `object.SymbolicPlaceholder` struct to store the original object being type-asserted.
2.  Modify `evalTypeSwitchStmt` and `evalAssignStmt` (for the `if-ok` case) to populate this `Original` field in the new `SymbolicPlaceholder` created for the narrowed variable.
3.  Enhance `evalSymbolicSelection` to check for this `Original` field. If present, it would "unwrap" the placeholder and perform the method call or field access on the concrete `Original` object.

## Roadblocks & Key Discoveries

The primary roadblock was a series of frustrating and repeated failures with the `replace_with_git_merge_diff` tool when trying to modify `symgo/object/object.go` and `symgo/evaluator/evaluator.go`. This led to a lot of time spent restoring files and re-attempting changes, which obscured the core technical issue for a while.

After many failed attempts to apply the changes, I ran the tests with the modifications in place. The tests *still* failed with the exact same errors as before. This was a key discovery: my changes to `evalSymbolicSelection` were not being triggered as I expected.

My initial hypothesis was that `evalSelectorExpr` was not correctly identifying the narrowed variable `v` as a `SymbolicPlaceholder`. After carefully re-reading `evalSelectorExpr`, I realized that it has a large, special-case block at the beginning for handling selectors on identifiers (`if ident, ok := n.X.(*ast.Ident); ok`). This block was being entered, but it only contained logic for resolving methods on *interfaces*. It did not have logic to handle the case where the variable's type was a concrete struct (as in my tests), and it didn't know to look inside the `SymbolicPlaceholder` for the `Original` object.

This discovery invalidated my initial implementation plan of only changing `evalSymbolicSelection`. The fix needs to be applied earlier, inside `evalSelectorExpr`.

## Major Refactoring Effort

Based on the discovery above, I attempted a new approach. The plan was to modify the `if ident, ok := n.X.(*ast.Ident); ok` block in `evalSelectorExpr`. I would add logic to check if the `object.Object` retrieved from the environment (`obj, found := env.Get(ident.Name)`) was a `Variable` holding a `SymbolicPlaceholder` with my new `Original` field. If so, I would perform the member lookup on the `Original` object directly within that block.

Unfortunately, I was unable to successfully implement this refactoring due to persistent issues with applying the necessary code modifications, which led to a cycle of failed test runs and rollbacks. The implementation was reverted to a clean state to avoid submitting broken code.

## Current Status

The codebase has been reverted to its original state, with the exception of the following additions:
- The new test file `symgo/evaluator/evaluator_if_typeswitch_test.go` containing two failing tests that correctly define the feature requirement.
- An update to `TODO.md` marking the feature as "In Progress".

The implementation itself is incomplete. The core logic in `symgo/evaluator/evaluator.go` remains unchanged.

## References

- `docs/plan-symgo-type-switch.md`: The original plan document for this feature.
- `docs/analysis-symgo-implementation.md`: General documentation on the `symgo` evaluator's design.
- `symgo/evaluator/evaluator.go`: The file requiring modification.
- `symgo/object/object.go`: The file requiring the addition of the `Original` field.

## TODO / Next Steps

1.  **Re-implement the `Original` field:** Add the `Original Object` field to the `object.SymbolicPlaceholder` struct in `symgo/object/object.go`.
2.  **Populate the `Original` field:** Modify `evalTypeSwitchStmt` and `evalAssignStmt` in `symgo/evaluator/evaluator.go` to correctly populate the `Original` field of the `SymbolicPlaceholder` they create.
3.  **Enhance `evalSelectorExpr`:** This is the most critical step. Modify the `if ident, ok := n.X.(*ast.Ident); ok` block at the beginning of `evalSelectorExpr`. Add logic to detect when the identifier resolves to a `Variable` containing a `SymbolicPlaceholder` that has a non-nil `Original` object.
4.  **Delegate to Original Object:** When the condition in step 3 is met, the logic should "unwrap" the placeholder and perform the method or field lookup on the `Original` object. This will involve replicating the logic from the later `switch val := left.(type)` block, but operating on `val.Original`.
5.  **Run Tests:** Run the tests in `symgo/evaluator/evaluator_if_typeswitch_test.go` and iterate on the implementation until they pass.
6.  **Submit:** Once all tests pass, submit the complete solution.
