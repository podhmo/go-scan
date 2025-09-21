# Troubleshooting Report: `symgo` Type Switch and Assertion Implementation

This document consolidates the planning, implementation, and troubleshooting history for the `symgo` type-narrowing feature.

---
## Part 1: Initial Plan (`docs/plan-symgo-type-switch.md`)

# Plan: Enhance `symgo` for Type-Narrowed Member Access

### 1. Goal

The objective is to enhance the `symgo` symbolic execution engine to correctly trace method calls and field accesses on variables whose types have been narrowed within a control flow structure. This applies to two primary Go idioms:

1.  **Type Switch:** In a `switch v := i.(type)` statement, the variable `v` should be recognized as having the specific type of each `case` block.
2.  **`if-ok` Type Assertion:** In an `if v, ok := i.(T); ok` statement, the variable `v` should be recognized as type `T` within the `if` block.

### 2. Investigation and Current State Analysis

A review of the `symgo` evaluator reveals that while the basic structures for handling these statements exist (creating scoped environments and `SymbolicPlaceholder` objects), the existing test suite does not contain any tests that perform a method call or field access on the narrowed variable. This indicates the functionality is unverified and likely incomplete.

### 3. Proposed Implementation Plan (TDD Approach)

1.  **Create New Test File:** `symgo/evaluator/evaluator_if_typeswitch_test.go`.
2.  **Add Failing Tests:** Add tests for method calls in type switches and field access in `if-ok` assertions.
3.  **Add Edge Case Tests:** Cover pointer receivers, embedded structs, and multiple `case` blocks.
4.  **Enhance the Evaluator:** Modify `evalSelectorExpr`, `evalSymbolicSelection`, and the `accessor` to correctly use the `TypeInfo` attached to symbolic placeholders to resolve members.
5.  **Handle Scan Policies:** Ensure the implementation is robust for both in-policy and out-of-policy type assertions.
6.  **Verify and Finalize:** Ensure all new and existing tests pass.

---
## Part 2: Continuation 1 (`docs/cont-symgo-type-switch.md`)

# Continuation: Enhancing `symgo` for Type-Narrowed Member Access

### The Plan

The fix involves a new, more precise strategy:

1.  **Fix `evalCompositeLit`**: Ensure that when evaluating a struct literal, the field values are stored in the resulting `*object.Instance`.
2.  **Move and Fix `resolveSymbolicField`**: Move this method from the resolver to the evaluator to break a dependency and implement it to check the instance's state map before creating a new placeholder.
3.  **Refactor `evalSelectorExpr`**: Refactor into a wrapper and a core function. The wrapper will check if a placeholder resulted from a type assertion. If so, it will perform a type compatibility check between the placeholder's narrowed type and the concrete type of the original object. If compatible, it unwraps the placeholder; otherwise, it prunes the path.
4.  **Add `Original` field**: Add an `Original object.Object` field to `object.SymbolicPlaceholder`.
5.  **Populate `Original` field**: Modify `evalTypeSwitchStmt` and `evalAssignStmt` to populate this `Original` field.

---
## Part 3: Continuation 2 (`docs/cont-symgo-type-switch-2.md`)

# Continuation 2: Enhancing `symgo` for Type-Narrowed Member Access

### Work Done

1.  **Added `Original` field to `SymbolicPlaceholder`**: The struct was modified as planned.
2.  **Populated the `Original` field**: `evalTypeSwitchStmt` and `evalAssignStmt` were updated.
3.  **Implemented stateful struct literals**: `evalCompositeLit` was enhanced to store field values in the created instance.

### Failures and Challenges

The primary blocker was the persistent failure of the `replace_with_git_merge_diff` tool when attempting to refactor `evalSelectorExpr`, preventing the implementation of the core logic.

---
## Part 4: Continuation 3 (`docs/cont-symgo-type-switch-3.md`)

# Continuation 3: Fixing Regressions in Type-Narrowed Member Access

### Previous State

The core feature was implemented, and tests for concrete types were passing. However, this introduced regressions in tests related to interface methods and unresolved types.

### Analysis and Hypothesis

1.  **Problem 1: Premature Unwrapping of Interfaces.** The logic in `evalSelectorExpr` unwraps placeholders too aggressively, breaking assertions that narrow to an interface type.
2.  **Problem 2: Incorrect Member Lookup Order.** The logic checks for methods before fields, causing issues with unresolved types where field access is misinterpreted as a method call.

### Next Steps

1.  **Implement Conditional Unwrapping in `evalSelectorExpr`**: The unwrapping of a `SymbolicPlaceholder` should only occur if the narrowed type is **not** an interface.
2.  **Implement Field-First Lookup**: In `evalSelectorExprForObject` and `evalSymbolicSelection`, modify the logic to search for a struct field **before** searching for a method.

---
## Part 5: Continuation 4 (`docs/cont-symgo-type-switch-4.md`)

# Continuation 4: Fixing `TestInterfaceBinding`

### Goal

The immediate goal was to fix `TestInterfaceBinding` by correctly implementing `interp.BindInterface` logic.

### Key Discovery & Roadblock

-   **Discovery**: The correct way to dispatch from an interface method to a concrete one is to re-call the top-level `applyFunction` with the concrete method object. This ensures the full pipeline, including intrinsic checks, is executed.
-   **Roadblock**: This change required passing an `*object.Environment` through the `applyFunction` call stack, which caused a massive cascade of build failures across the test suite. I got stuck in a debugging loop trying to fix all of them at once with large, multi-file patches that kept failing.

### Current Status & Next Steps

The codebase was left in a **non-building state**. The immediate priority for the next agent is to methodically fix the build errors one file at a time.

---
## Part 6: Continuation 5 (`docs/cont-symgo-type-switch-5.md`)

# Continuation 5: Stabilizing the Build and Re-evaluating

### Work Summary

1.  **Build & Test Suite Stabilization**: All build errors from the previous refactoring were fixed methodically. Several panics related to incorrect test assumptions about environment caching were also diagnosed and fixed.
2.  **Root Cause Analysis**: With a stable suite, the root cause of the type-narrowing failures was confirmed: the `SymbolicPlaceholder` created for the narrowed variable was losing the connection to the original concrete object.

### The Confirmed Plan

The plan from Continuation 2 was re-affirmed and refined:
1.  **Set the `Original` field** in `evalTypeSwitchStmt` and `evalAssignStmt`.
2.  **Use the `Original` field** in `evalSelectorExpr` by creating a wrapper that checks for a placeholder and its `Original` field, then unwraps it to perform the selection on the concrete object.
3.  **Rename `evalSelectorExpr`** to `evalSelectorExprForObject` as part of this refactoring.

---
## Part 7: Continuation 6 (`docs/cont-symgo-type-switch-6.md`)

# Continuation 6: Core Feature Implementation

### Current Status

1.  **Environment Population Fix**: Resolved "identifier not found" errors for type names.
2.  **Struct Literal Evaluation Fix**: `evalCompositeLit` now correctly populates fields of struct instances.
3.  **Core Type-Narrowing Verified**: The combination of fixes has unblocked the type-narrowing logic. The primary tests for concrete types (`TestTypeSwitch_MethodCall`, `TestIfOk_FieldAccess`, `TestTypeSwitch_Complex`) are now passing.

### Remaining Tasks

1.  **Fix Failures in Interface-Related Tests**: Several tests related to interface method calls are still failing (`TestEval_ExternalInterfaceMethodCall`, `TestInterfaceBinding`, etc.).
2.  **Add Tests for Scan Policy Behavior**: Add tests for in-policy vs. out-of-policy type assertions.

---
## Part 8: Final Troubleshooting (`docs/cont-symgo-type-switch-7.md`)

# Troubleshooting the Final Interface Failures

### 1. Panic Investigation

The effort began with fixing a new series of cascading panics:
- **Panic 1 (`TestEval_FieldAccessOnSymbolicPlaceholder`):** Fixed by adding a `nil` check in `applyFunction` for functions with no `*ast.Ident` name.
- **Panic 2 (`TestEvaluator_IfStmt_ResultIsNil`):** Fixed by a series of changes to `applyFunction` and `evalIfStmt` to correctly handle the implicit return value of functions ending in a statement block.

### 2. Fixing Concrete Type-Narrowing

With a stable test environment, the focus shifted to the feature's core logic.
- **`TestTypeSwitch_MethodCall`:** Fixed by refactoring `extendFunctionEnv` to correctly bind the receiver object in method calls.
- **`TestIfOk_FieldAccess`:** Fixed by improving `evalSymbolicSelection` to correctly unwrap `*object.Variable` placeholders to find the underlying concrete instance.
- **`TestTypeSwitch_Complex`:** Fixed by simplifying `*object.Variable` creation in `evalTypeSwitchStmt` to prevent state pollution between `case` blocks.

### 3. Current Impasse: Interface Method Calls

After fixing all issues related to concrete types, the remaining failures are all related to method calls on interface types.

- **Failing Tests:** `TestEval_InterfaceMethodCall_OnConcreteType`, `TestEval_InterfaceMethodCall_AcrossControlFlow`, `TestDefaultIntrinsic_InterfaceMethodCall`.
- **Core Symptom:** When a method is called on a variable whose static type is an interface (e.g., `var s Speaker; s.Speak()`), the evaluator resolves the call to the concrete method on the variable's underlying value (e.g., `Dog.Speak`) instead of treating it as a symbolic call on the `Speaker` interface. The desired behavior is to produce a `*object.SymbolicPlaceholder` for the interface call.
- **Root Cause Analysis:** The logic in `evalSelectorExpr` is intended to catch this case and return a placeholder. However, extensive debugging and diagnostic tests revealed that this logic block is never being entered. The reason is that at the time `evalSelectorExpr` is called for `s.Speak()`, the `FieldType` of the variable `s` is `nil`.
- **Conclusion:** The static type information of the interface variable, which is correctly set during its declaration, is being lost or overwritten somewhere before the method call is evaluated. The exact location of this state loss has not been identified, and multiple attempts to fix it by changing the assignment logic in `assignIdentifier` have failed. This represents the current blocker.
