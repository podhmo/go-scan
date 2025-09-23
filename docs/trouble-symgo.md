# Sym-Go Trouble Shooting Log: Type-Switch and Interface Resolution

This document is a consolidated log of the planning, implementation, and troubleshooting process for enhancing `symgo`'s handling of type switches and interface method resolution.

---
---

# Continuation 9: The `BoundMethod` and Interface Resolution Saga

## Goal

The primary objective is to fix all remaining test failures for the `symgo` type-switch and interface handling features to make the implementation robust and complete.

## Work Summary & Current Status

This session was characterized by a significant regression followed by a series of fixes that have brought the codebase closer to a stable state, but have revealed a new set of failures.

1.  **Reverting to a Baseline**: After getting stuck in a loop of incorrect patches, I reverted `symgo/evaluator/evaluator.go` to its original state. This was a necessary reset, but it immediately caused a massive number of tests to fail with the error `not a function: BOUND_METHOD`.

2.  **The `BoundMethod` Regression Fix**: This widespread failure indicated that a critical piece of logic for handling method calls had been lost. The `evalSelectorExpr` was correctly creating `*object.BoundMethod` objects (which pair a function with its receiver), but `applyFunctionImpl` did not have a `case` to handle this object type, so it didn't know how to call it. I successfully re-introduced the `case *object.BoundMethod:` block into `applyFunctionImpl`. This fix resolved the `BOUND_METHOD` errors and stabilized the test suite.

3.  **The Interface Resolution Regression**: After fixing the `BoundMethod` issue, a new major regression appeared: the entire `TestInterfaceResolution` suite started failing, along with related tests like `TestFindOrphans_interface`. The error messages (e.g., `expected (*Dog).Speak to be called via interface resolution, but it was not`) indicate that the `Finalize` step is no longer connecting symbolic interface method calls to their concrete implementations. This is almost certainly because the `calledInterfaceMethods` map is not being populated correctly during the initial symbolic execution pass.

4.  **Field vs. Method Lookup Issue**: The original target of this plan step, `TestFeature_FieldAccessOnPointerToUnresolvedStruct`, is still failing with its original error: `expected reason to contain 'field access on symbolic value', but got "symbolic method call Name on unresolved symbolic type Data"`. This confirms that the logic for handling selectors on symbolic pointers is incorrect.

## Current Hypothesis

The two major remaining issues seem to be:
1.  **Interface Call Tracking**: The refactoring of `evalSelectorExprForObject` to handle `*object.Variable` directly has likely broken the logic that detects a symbolic call on an interface method, and therefore it no longer gets added to the `calledInterfaceMethods` map.
2.  **Symbolic Pointer Dereferencing**: The `evalSymbolicSelection` function does not correctly "look through" a pointer to its underlying type when checking for fields.

## TODO / Next Steps

The path forward is to address these two core issues. Fixing the interface resolution regression is the highest priority, as it was previously working functionality.

1.  **Fix Interface Resolution Regression**:
    *   **Investigate `applyFunctionImpl`**: This is the most likely culprit. When an abstract function (like an interface method with no body) is called, `applyFunctionImpl` needs to recognize this and add the call and its receiver to the `e.calledInterfaceMethods` map. This logic appears to be missing or incorrect.
    *   Add the necessary logic to ensure that symbolic calls to interface methods are correctly tracked.

2.  **Fix Field Access on Symbolic Pointers**:
    *   Modify `evalSymbolicSelection` in `evaluator.go`.
    *   Add logic at the beginning of the function to check if the incoming `typeInfo` is a pointer kind.
    *   If it is, resolve its underlying element type and use *that* type for the subsequent field and method lookups.
    *   Ensure the lookup order is **field-first**, then method, to correctly handle shadowed members.

3.  **Run All Tests**: After applying these two fixes, run the entire `./...` test suite to verify that all tests now pass.

---
---

# Continuation 8: The Great Refactoring and Finalization

## Goal

The final goal is to fix all remaining test failures related to the `symgo` type-switch and interface handling features, ensuring the implementation is robust, complete, and passes the entire test suite.

## Work Summary & Current Status

This phase of the work involved a major architectural refactoring of the evaluator to correctly model Go's distinction between method values (e.g., `h.Hex`) and method calls (e.g., `h.Hex()`).

1.  **The Great Refactoring**: The core change was to stop treating every selector on an interface as an immediate method call.
    *   A new `*object.BoundMethod` type was introduced to represent a method value—the pairing of a function and a receiver.
    *   `evalSelectorExpr` was changed to produce these `*object.BoundMethod` objects instead of `SymbolicPlaceholder`s.
    *   The logic for executing a call was moved into `applyFunction`, which now has a dedicated case for `*object.BoundMethod`.

2.  **Build Stabilization**: This refactoring required creating the `BoundMethod` type in `symgo/object/object.go` and fixing a cascade of build errors across the evaluator and its tests. This was a complex process that involved several incorrect assumptions before the correct types and function signatures were stabilized.

3.  **Dynamic Dispatch Fix**: After the build was stable, tests revealed that the evaluator was not correctly handling interface variables that held concrete types. New logic was added to `applyFunction` to first check the concrete type of an interface variable's value and dispatch the call to its method directly, mirroring Go's runtime behavior. This required adding a new `findMethodOnObject` helper to the accessor.

4.  **Unresolved Type Fix**: It was discovered that the evaluator was too aggressive in treating `scan.UnknownKind` as an interface, causing field access on unresolved struct types to be misinterpreted as method calls. This was fixed by making the interface check more specific.

5.  **`Finalize()` Logic Fix**: The most recent failures in `TestInterfaceResolution` and `find-orphans` were traced to incorrect string parsing logic in the `Finalize` method. The code for parsing the interface and method name from the `calledInterfaceMethods` map key was not robust. This has been replaced with a more reliable parsing strategy.

## Current Status

All known logical flaws discovered during this extensive process have been addressed. The final step is to run the full test suite to verify that the fix to the `Finalize` method, in combination with all the previous refactoring, results in a fully passing test suite for the `symgo` package and its related components.

## Next Steps

1.  Run `make test`.
2.  If all tests pass, the task is complete. Update `TODO.md` accordingly and prepare for submission.
3.  If any tests still fail, analyze the failures and create a new plan to address them.

---
---

# Plan: Enhance `symgo` for Type-Narrowed Member Access

## 1. Goal

The objective is to enhance the `symgo` symbolic execution engine to correctly trace method calls and field accesses on variables whose types have been narrowed within a control flow structure. The key is to resolve members of the **concrete type** (e.g., a specific struct's methods), not just members that might be part of the original interface's method set. This applies to two primary Go idioms:

1.  **Type Switch:** In a `switch v := i.(type)` statement, the variable `v` should be recognized as having the specific type of each `case` block, allowing symbolic execution to trace calls like `v.Method()` or access fields like `v.Field`.
2.  **`if-ok` Type Assertion:** In an `if v, ok := i.(T); ok` statement, the variable `v` should be recognized as type `T` within the `if` block, enabling the tracing of member access on `v`.

## 2. Investigation and Current State Analysis

A review of the `symgo` evaluator (`symgo/evaluator/evaluator.go`) and its tests reveals the following:

-   **Core Design Philosophy:** `symgo` is a symbolic tracer, not a standard interpreter. As documented in `docs/analysis-symgo-implementation.md`, it correctly explores all possible branches of control flow statements like `if` and `switch` to discover all potential code paths. This enhancement must adhere to that design.

-   **`evalTypeSwitchStmt`:** The current implementation correctly handles `switch v := i.(type)`. For each `case` clause, it creates a new, scoped environment (`caseEnv`). Within this environment, it defines a new variable `v` and assigns it a `SymbolicPlaceholder` object. This placeholder is correctly imbued with the `TypeInfo` and `FieldType` corresponding to the `case`'s type.

-   **`evalAssignStmt` & `evalIfStmt`:** The evaluator correctly handles the `v, ok := i.(T)` idiom. `evalAssignStmt` creates a `SymbolicPlaceholder` with the type information for `T` and assigns it to `v`. `evalIfStmt` correctly creates a new scope for the `if` block, ensuring the typed variable `v` is properly scoped.

-   **The Gap:** The existing test suite (`symgo/evaluator/evaluator_typeswitch_test.go`) verifies that the type-switched variable has the correct *type name* within each case. However, **it does not contain any tests that perform a method call or field access on the narrowed variable.** This indicates that while the mechanism for creating the typed variable exists, its utility in resolving member access is unverified and likely incomplete. The logic chain from `evalSelectorExpr` -> `evalSymbolicSelection` -> `accessor.findMethodOnType` seems plausible but has not been exercised by tests for this specific scenario.

## 3. Proposed Implementation Plan (TDD Approach)

This plan should be executed by a future engineer to implement the feature. It follows a test-driven development (TDD) methodology.

### Step 1: Create a New Test File

Create a new file `symgo/evaluator/evaluator_if_typeswitch_test.go` to isolate the new tests for this feature.

### Step 2: Add Failing Test for Method Call in Type Switch

Add a test case that defines a custom type with a method, uses a type switch to narrow an interface to that type, and calls the method. Use an intrinsic to verify the call is traced.

**Example Test Snippet:**

```go
// In test file
const typeSwitchMethodSource = `
package main

type Greeter struct { Name string }
func (g Greeter) Greet() { inspect(g.Name) }

func inspect(s string) {} // Intrinsic

func main() {
	var i any = Greeter{Name: "World"}
	switch v := i.(type) {
	case Greeter:
		v.Greet() // This method call should be traced
	case int:
		// Other case
	}
}
`
// Test logic would register an intrinsic for inspect() and
// assert that it was called with the value "World".
// This test should fail initially.
```

### Step 3: Add Failing Test for Field Access in `if-ok` Assertion

Add a test case that defines a struct with a field, uses an `if-ok` type assertion, and accesses the field. The field's value could be another function call to make tracing easier to verify.

**Example Test Snippet:**

```go
// In test file
const ifOkFieldAccessSource = `
package main

func get_name() string { return "Alice" }
func inspect(s string) {} // Intrinsic

type User struct {
	Name string
}

func main() {
	var i any = User{Name: get_name()}
	if v, ok := i.(User); ok {
		inspect(v.Name) // This field access should be traced
	}
}
`
// Test logic would register an intrinsic for get_name() and inspect()
// and assert that inspect() was called with the result of get_name().
// This test should also fail initially.
```

### Step 4: Add Tests for Pointer Receivers and Other Edge Cases

Expand the test suite to cover:
-   Types with pointer receivers (e.g., `func (g *Greeter) Greet()`).
-   Accessing members on embedded structs within a type assertion.
-   Multiple `case` blocks in a type switch.

### Step 5: Enhance the Evaluator

Modify the `symgo` evaluator to make the tests pass. The likely areas for modification are:

-   **`evalSelectorExpr`**: When evaluating `v.Greet()`, `v` will resolve to a `SymbolicPlaceholder`. The logic must robustly use the `TypeInfo` attached to this placeholder.
-   **`evalSymbolicSelection`**: This helper function will likely be the primary focus. It receives the `SymbolicPlaceholder` and must correctly delegate to the `accessor` to find the method or field.
-   **`accessor`**: Ensure `findMethodOnType` and `findFieldOnType` work correctly when given the `TypeInfo` from a symbolic placeholder. This includes handling both value and pointer receiver methods correctly based on how the variable is defined.

The core of the implementation will be ensuring that the `TypeInfo` stored on the symbolic variable is fully utilized during method and field resolution, successfully connecting the type-narrowed variable to its members.

**Handling Scan Policies:**
The implementation must be robust with respect to `symgo`'s scan policy. The tests should cover the following scenarios:
1.  **Intra-Policy Assertion:** The type assertion occurs in a package that is within the primary analysis scope, and the target type (`T`) is also defined within that scope. This is the baseline case where full source is available.
2.  **Extra-Policy Assertion:** The type assertion occurs in a package within the primary analysis scope, but the target type `T` is defined in an external package that is *not* part of the source-scanned policy. `symgo` should still be able to symbolically trace method calls on the narrowed variable, likely by creating a `SymbolicPlaceholder` for the method's result based on a shallow scan of the external type's definition.

### Step 6: Verify and Finalize

Once all new tests pass and existing tests continue to pass, the feature is complete. The implementation has been successfully guided and verified by the test suite.

---
---

# Continuation: Enhancing `symgo` for Type-Narrowed Member Access

This document outlines the plan to fix `symgo`'s handling of type assertions and type switches.

## Goal

The goal is to enhance the `symgo` symbolic execution engine to correctly trace method calls and field accesses on variables whose types have been narrowed within a `switch v := i.(type)` statement or an `if v, ok := i.(T); ok` type assertion.

## Current State

The work is starting from a clean repository state, with the exception of this document and a new test file, `symgo/evaluator/evaluator_if_typeswitch_test.go`, which contains three failing tests that define the required behavior.

## Core Problem

Previous attempts failed because of two related bugs:
1.  `evalCompositeLit`: When evaluating a struct literal (e.g., `User{Name: "Alice"}`), the evaluator was correctly determining the type of the literal but was discarding the evaluated field values (`"Alice"`). The resulting `*object.Instance` only contained type information, not the state of the fields.
2.  `resolver.ResolveSymbolicField`: This function was a stub. When asked to resolve a field access like `v.Name`, it would always return a new placeholder object instead of attempting to look up the value from the receiver.

These two bugs combined meant that even when a type assertion was correctly identified, the concrete values of the fields were lost, making it impossible to trace them.

## The Plan

The fix involves a new, more precise strategy:

1.  **Fix `evalCompositeLit` in `evaluator.go`**:
    *   The function will be modified to differentiate between struct literals and other composite literals (maps, slices).
    *   For struct literals, it will evaluate each field's value and store the result in the `State` map of the `*object.Instance` it creates. This ensures the instance carries its state.
    *   The logic for maps and slices will remain unchanged to prevent regressions.

2.  **Move and Fix `resolveSymbolicField`**:
    *   The method `ResolveSymbolicField` will be moved from `resolver.go` to `evaluator.go` and renamed to `resolveSymbolicField` to break a circular dependency that would be created by giving the resolver access to the evaluator's `forceEval` method.
    *   The call sites in `evaluator.go` will be updated to call `e.resolveSymbolicField`.
    *   The new `e.resolveSymbolicField` method will be implemented to:
        *   Check if the receiver is an `*object.Instance` (or a pointer to one).
        *   If so, look up the requested field name in the instance's `State` map.
        *   If the value is found, return it (after running it through `forceEval` to handle lazy variables).
        *   If the field is not in the state map, or if the receiver is not an instance, fall back to the old behavior of returning a new placeholder.

3.  **Fix `evalSelectorExpr` in `evaluator.go`**:
    *   This function will be refactored into a wrapper (`evalSelectorExpr`) and a core logic function (`evalSelectorExprForObject`).
    *   The wrapper will be responsible for detecting if the expression being selected upon (e.g., `v` in `v.DoA()`) is a placeholder that resulted from a type assertion (i.e., its `Original` field is not nil).
    *   If it is, it will perform a **type compatibility check**: does the concrete type of the `Original` object match the narrowed type of the placeholder?
    *   If the types do **not** match, it means we are in a symbolic path that is impossible at runtime (e.g., evaluating `case B:` for an object of type `A`). The wrapper will prune this path by returning a generic placeholder.
    *   If the types **do** match, the wrapper will "unwrap" the placeholder and call the core logic function (`evalSelectorExprForObject`) with the concrete `Original` object, allowing the method/field access to succeed.

4.  **Add `Original` field**: The `object.SymbolicPlaceholder` struct in `symgo/object/object.go` will be modified to include an `Original object.Object` field.

5.  **Populate `Original` field**: `evalTypeSwitchStmt` and `evalAssignStmt` in `evaluator.go` will be modified to populate this `Original` field when they create placeholders for type-narrowed variables.

This comprehensive plan addresses the root causes of the bugs and should lead to a successful implementation.

---
---

# Continuation 2: Enhancing `symgo` for Type-Narrowed Member Access

This document records the continuation of the work to enhance `symgo`'s handling of type assertions and type switches, picking up from `docs/cont-symgo-type-switch.md`.

## Goal

The goal remains the same: to enhance the `symgo` symbolic execution engine to correctly trace method calls and field accesses on variables whose types have been narrowed within a `switch v := i.(type)` statement or an `if v, ok := i.(T); ok` type assertion.

## Previous State

The work started from the state defined in `docs/cont-symgo-type-switch.md`, which involved a clean repository with a new test file containing failing tests.

## Work Done

The following steps from the plan in `docs/cont-symgo-type-switch.md` have been successfully implemented and verified to be present in the codebase:

1.  **Added `Original` field to `SymbolicPlaceholder`**: The struct `object.SymbolicPlaceholder` in `symgo/object/object.go` was modified to include an `Original object.Object` field. This field is intended to hold the concrete object that a placeholder represents after a type assertion.

2.  **Populated the `Original` field in type assertions**: The `evalTypeSwitchStmt` and `evalAssignStmt` functions in `symgo/evaluator/evaluator.go` were modified to correctly populate this new `Original` field on the `SymbolicPlaceholder` objects they create for type-narrowed variables.

3.  **Implemented stateful struct literals**: The `evalCompositeLit` function in `symgo/evaluator/evaluator.go` was enhanced. It now correctly evaluates the values of fields in a struct literal and stores them in the `State` map of the `*object.Instance` it creates, ensuring the instance carries its state.

## Failures and Challenges

The primary blocker in this session was the persistent failure of the `replace_with_git_merge_diff` tool when attempting to perform a key refactoring step on the `evalSelectorExpr` function in `symgo/evaluator/evaluator.go`.

-   Multiple attempts were made to refactor the function, both in a single large step and in smaller, incremental steps.
-   These attempts consistently failed, often leaving the `evaluator.go` file in a syntactically incorrect state, which required repeated restoration.
-   This consumed a significant amount of time and prevented the implementation of the core logic required to make the tests pass.

## Next Steps (for the next agent)

The original plan remains sound. The next agent should focus on completing the refactoring of `evalSelectorExpr` and the related logic.

1.  **Move and Fix `resolveSymbolicField`**:
    *   The method `ResolveSymbolicField` should be moved from `symgo/evaluator/resolver.go` to `symgo/evaluator/evaluator.go` and renamed to `resolveSymbolicField`.
    *   The implementation of `resolveSymbolicField` must be updated to first check the `State` map of the receiver (`*object.Instance` or `*object.Pointer` to one) before falling back to creating a new placeholder.

2.  **Refactor `evalSelectorExpr`**:
    *   This is the most critical step. The function `evalSelectorExpr` in `symgo/evaluator/evaluator.go` needs to be refactored into a wrapper and a core logic function (e.g., `evalSelectorExprForObject`).
    *   The new `evalSelectorExpr` wrapper must inspect its `leftObj`. If it's a `SymbolicPlaceholder` with a non-nil `Original` field, it must perform a type compatibility check.
    *   **Type Check**: The check should verify that the concrete type of the `Original` object is compatible with the narrowed type of the `SymbolicPlaceholder`.
    *   **Unwrap or Prune**: If the types are compatible, the wrapper should "unwrap" the placeholder and call the core logic function (`evalSelectorExprForObject`) with the `Original` object. If they are not compatible, it should return a generic placeholder to prune the impossible symbolic path.

3.  **Run Tests and Fix**:
    *   After the refactoring, run `go test -v ./symgo/evaluator/...`.
    *   The tests in `symgo/evaluator/evaluator_if_typeswitch_test.go` should now pass. Debug any remaining issues until they do.

---
---

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

---
---

# Continuation 7: Stabilizing the Test Suite and Fixing Core Logic

## Initial Prompt

(Translated from Japanese)
"Please read one task from TODO.md and implement it. If necessary, break it down into sub-tasks... Please do your best to modify the code so that the test code passes. Once it is somewhat complete, please also pay attention to the behavior inside and outside the policy."

## Goal

Fix all remaining test failures for the `symgo` type-switch and interface handling features to make the implementation robust and complete.

## Initial Implementation Attempt

My first attempt was to fix a large number of cascading build failures caused by a previous refactoring. My initial strategy was to fix them all at once with a large `replace_with_git_merge_diff` command. This repeatedly failed because the file state changed during the operation, causing subsequent search blocks to be invalid. This left the codebase in a broken state and consumed a lot of time.

## Roadblocks & Key Discoveries

*   **Discovery 1:** The "fix all at once" strategy for widespread, similar build errors is flawed. It's more effective to fix them one file at a time, re-running the build after each fix to get a fresh state. This methodical approach allowed me to finally stabilize the build.

*   **Discovery 2:** After fixing the build, I discovered that many tests were still failing due to an incorrect test setup pattern. The `evaluator.New()` was being called inside the `scantest.Run` helper without a properly configured package environment, leading to inconsistent and incorrect behavior.

*   **Discovery 3:** The solution to the test setup problem was to introduce a `WithPackages` functional option for the `evaluator.New` constructor. This allows a pre-configured package environment to be injected, ensuring tests run in a consistent and correct context.

*   **Discovery 4:** With the test suite stabilized, I found that method calls on `nil` interface variables were failing (e.g., `TestDefaultIntrinsic_InterfaceMethodCall`). The `evalSelectorExprForObject` function was unwrapping the `*object.Variable` (which held the `nil` interface) to its underlying `*object.Nil` value *before* looking up the method. This lost the variable itself as the receiver.

## Major Refactoring Effort

1.  **Build Stabilization**: I systematically fixed all build errors across the test suite by adopting a one-at-a-time approach.
2.  **`WithPackages` Option**: I added a new `WithPackages` functional option to `evaluator.New` in `evaluator.go` to solve the test setup problem.
3.  **Test Refactoring**: I methodically refactored over a dozen test files (`evaluator_test.go`, `evaluator_label_test.go`, etc.) to use the new, correct test setup pattern with `WithPackages`.
4.  **Receiver Logic Fix**: I refactored `evalSelectorExprForObject` to correctly handle `*object.Variable` receivers. It now operates on the variable's underlying value without discarding the variable itself, preserving it as the receiver for method calls. This fixed the `nil` interface method call bug.

## Current Status

The codebase is in a much better state. The vast majority of tests are passing, and the test suite is reliable. The remaining failures are more subtle edge cases. I am currently focused on `TestFeature_FieldAccessOnPointerToUnresolvedStruct`, where a field access is being misidentified as a method call. My analysis points to the lookup order in `evalSymbolicSelection` being the cause.

## References

*   `docs/plan-symgo-type-switch.md`
*   `docs/cont-symgo-type-switch-6.md`
*   `symgo/evaluator/evaluator.go`
*   `symgo/features_test.go`

## TODO / Next Steps

1.  Modify `evalSymbolicSelection` in `evaluator.go` to prioritize checking for a field on a struct-like `TypeInfo` before attempting to resolve a method. This should fix `TestFeature_FieldAccessOnPointerToUnresolvedStruct`.
2.  Debug the failures in `TestAnonymousTypes_Interface`, which relate to method resolution on anonymous interfaces.
3.  Systematically address the remaining `TestEval_InterfaceMethodCall...` failures.
4.  Fix the regression in `TestTypeSwitchStmt`.
5.  Fix the final test setup issue in `TestTypeInfoPropagation`.
---
---

# Continuation 10: Fixing Interface Method Resolution

## Goal

The primary objective is to fix the test failures related to `symgo`'s type switch (`switch v := i.(type)`) and type assertion (`if v, ok := i.(T)`) capabilities, with a focus on resolving interface method calls correctly.

## Work Summary & Current Status

This work session focused on implementing a robust model for abstract interface method calls within the `symgo` evaluator.

1.  **Core Strategy**: The main plan was to make the evaluator explicitly aware of abstract interface methods.
    -   An `IsAbstract` boolean field was added to the `object.Function` struct.
    -   The `object.BoundMethod`'s `Function` field was changed from `*object.Function` to the more general `object.Object` to allow it to hold different kinds of callable objects.
    -   The `evalSelectorExprForObject` function was refactored to check if a variable has an interface type. If it does, it now creates a `BoundMethod` that wraps a new, abstract `Function` object.
    -   The `applyFunctionImpl` function was modified to detect these abstract functions and dispatch them to the `defaultIntrinsic` for symbolic tracking.

2.  **Build Error Debugging Loop**: The initial implementation of this strategy was plagued by a series of cascading build errors.
    -   I initially made incorrect assumptions about the API of the `scanner` package, trying to call `typeInfo.Info()` and `typeInfo.FindMethod()`, which do not exist.
    -   After several attempts, I corrected this by accessing `typeInfo.Interface.Methods` and looping through the slice to find the correct method.
    -   This process was slow and iterative, but it successfully resolved all build errors.

## Current Status

The codebase is currently in a **non-building state** due to a final, subtle build error that was just discovered:

```
# github.com/podhmo/go-scan/symgo/evaluator
symgo/evaluator/evaluator.go:1522:17: cannot use m (variable of type *"github.com/podhmo/go-scan/scanner".MethodInfo) as *"github.com/podhmo/go-scan/scanner".FunctionInfo value in assignment
symgo/evaluator/evaluator.go:1665:18: cannot use m (variable of type *"github.com/podhmo/go-scan/scanner".MethodInfo) as *"github.com/podhmo/go-scan/scanner".FunctionInfo value in assignment
```

This error indicates that `typeInfo.Interface.Methods` is a slice of `*scanner.MethodInfo`, but my code is trying to assign its elements to a variable of type `*scanner.FunctionInfo`.

## Next Steps

1.  **Fix Final Build Error**:
    -   Investigate the definition of `scanner.MethodInfo` to find out how it relates to `scanner.FunctionInfo`. It likely contains `FunctionInfo` as an embedded field or a named field.
    -   Correct the assignment in `evalSelectorExprForObject` and `evalSymbolicSelection` to access the underlying `FunctionInfo` correctly (e.g., `methodDef = m.FunctionInfo`).

2.  **Run All Tests**: Once the build is fixed, run the entire `./...` test suite.

3.  **Analyze Results**: Carefully analyze the test results. The changes made are significant and should fix many of the interface-related failures (`TestInterfaceBinding`, `TestDefaultIntrinsic_InterfaceMethodCall`, etc.), but they may also have introduced new regressions that will need to be addressed.
