# Sym-Go Trouble Shooting Log: Type-Switch and Interface Resolution

This document is a consolidated log of the planning, implementation, and troubleshooting process for enhancing `symgo`'s handling of type switches and interface method resolution.

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
// Test logic would register intrinsics for get_name() and inspect()
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
    *   If the types **do** match, the wrapper will "unwrap" the placeholder and call `evalSelectorExprForObject` with the concrete `Original` object, allowing the method/field access to succeed.

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

# Continuation of Sym-Go Type Switch Implementation (4)

## Initial Prompt

(Translated from Japanese)
"Please read one task from TODO.md and implement it. If necessary, break it down into sub-tasks. After breaking it down, you can write it in TODO.md. Then, please proceed with the work. Keep modifying the code until the tests pass. After finishing the work, please be sure to update TODO.md at the end. The task to choose should be a symgo task. The origin is docs/plan-symgo-type-switch.md, and you can see the overall progress here. The implementation itself is a continuation of docs/cont-symgo-type-switch-3.md. Please do your best to modify the code so that the test code passes. Once it is somewhat complete, please also pay attention to the behavior inside and outside the policy. Please especially address the parts that are in progress. If you cannot complete it, please add it to TODO.md."

## Goal

The primary objective is to fix the remaining test failures related to the `symgo-type-switch` feature, with an immediate focus on making `TestInterfaceBinding` pass. This requires correctly implementing the logic for `interp.BindInterface` within the `symgo` evaluator, ensuring that calls to bound interface methods are correctly dispatched to their concrete implementations.

## Initial Implementation Attempt

My first attempt to fix `TestInterfaceBinding` involved adding logic directly into `applyFunctionImpl` to handle the dispatch from an interface method to a concrete one. While this seemed straightforward, it failed because it bypassed the standard function call machinery, which is responsible for checking for and executing registered intrinsics. The test specifically failed because it expected an intrinsic for `(*bytes.Buffer).Write` to be called, but my implementation called the method's body directly, skipping the intrinsic check.

## Roadblocks & Key Discoveries

My work was characterized by a key insight followed by a significant implementation roadblock.

*   **Key Discovery**: I realized that to solve the `TestInterfaceBinding` failure, `applyFunctionImpl` couldn't just execute the concrete method's body. It needed to re-initiate the entire function application process for the *concrete* method. This means recursively calling the wrapper function `applyFunction`, not `applyFunctionImpl`. This is the only way to ensure the full evaluation pipeline, including intrinsic checks, is triggered for the dispatched call.

*   **Roadblock (Cascading Build Errors)**: The key discovery necessitated a major refactoring: passing an `*object.Environment` through the entire `applyFunction` call stack. This change, while correct, caused a massive cascade of build failures across more than a dozen test files. I then became stuck in a debugging loop that consumed all the available time.

### Analysis of the Debugging Loop

To assist the next attempt, here is a breakdown of the trial-and-error loop I was stuck in while trying to fix the build errors:

1.  **Initial Thought Process**: "The build failed with many similar errors about swapped arguments in `applyFunction` calls in test files. I can fix all of them at once."
2.  **Action**: I used `grep` to find all instances and constructed a large, multi-block `replace_with_git_merge_diff` command to patch all affected test files simultaneously.
3.  **Result**: The command failed with "ambiguous" or "not found" errors. This is because my local understanding of the files became stale after the first few (successful or unsuccessful) patch applications, and the subsequent search blocks in the same command no longer matched the now-modified files.
4.  **Flawed Second Thought**: "My `replace_with_git_merge_diff` command must have been syntactically wrong, or I'm misreading the error messages. I'll read one of the failing files again and build another large patch command."
5.  **Action & Result**: I repeated steps 2 and 3 multiple times, sometimes focusing on a different file but always using the same flawed "fix everything at once" strategy. Each time, the complex patch would fail, leaving the codebase in a partially-modified state and leading to a new, but confusingly similar, list of build errors on the next `go test` run. I was unable to recognize that the strategy itself was the problem.

This loop prevented me from making methodical progress. The key takeaway is that when facing numerous, similar, cascading build errors after a refactoring, a **one-by-one approach** is safer and more reliable than attempting a single, complex fix.

## Major Refactoring Effort

Based on the key discovery, I undertook a significant refactoring of `symgo/evaluator/evaluator.go`:

1.  I changed the signatures of `applyFunction`, `applyFunctionImpl`, `Apply`, and `ApplyFunction` to accept an additional `*object.Environment` parameter.
2.  I implemented the new interface dispatch logic inside `applyFunctionImpl`. This new logic correctly finds the concrete method and re-dispatches the call by invoking `e.applyFunction(...)` with the new concrete function object.
3.  I began the process of updating all call sites across the codebase to pass the new `env` parameter. This process is incomplete and is the source of the current build failures.

## Current Status

The codebase is currently in a **non-building state**.

*   The core logic in `symgo/evaluator/evaluator.go` has been updated with the correct approach for interface binding dispatch.
*   However, numerous test files still have incorrect calls to `applyFunction` and `Apply`, resulting in build compilation errors (typically "cannot use token.Pos as *object.Environment" due to swapped arguments). I have been unable to resolve these cascading errors in the allotted time due to the debugging loop described above.

## References

*   `docs/plan-symgo-type-switch.md`
*   `docs/cont-symgo-type-switch-3.md`
*   `symgo/symgo_interface_binding_test.go`

## TODO / Next Steps

The immediate and only priority is to get the code back into a buildable state. A methodical, one-at-a-time approach is required.

1.  **Systematically Fix Build Errors**:
    *   Run `go test -v ./symgo/...` to get a fresh, definitive list of build errors.
    *   Pick **one** failing file from the list (e.g., `symgo/evaluator/evaluator_test.go`).
    *   Read that file to get its current, exact content.
    *   Fix **only the first** incorrect call in that file using `replace_with_git_merge_diff`.
    *   Run `go test -v ./symgo/...` again. If the error for that line is gone, repeat the process for the next error in the same file.
    *   Once a file is clean, move to the next file in the build error list.
2.  **Verify `TestInterfaceBinding`**: Once the build is fixed, run the tests again, focusing on `TestInterfaceBinding`. It is hoped that the refactoring has fixed this test.
3.  **Address Regressions**: Systematically debug and fix any other test failures that may have been introduced.

---
---

# Continuation of Sym-Go Type Switch Implementation (5)

## Goal

The primary objective remains to fix the test failures related to `symgo`'s type switch (`switch v := i.(type)`) and type assertion (`if v, ok := i.(T)`) capabilities. The key failing tests are `TestInterfaceBinding`, `TestTypeSwitch_MethodCall`, `TestIfOk_FieldAccess`, and `TestTypeSwitch_Complex`.

## Work Summary & Current Status

1.  **Build & Test Suite Stabilization**: The initial state of the codebase was a non-building one due to a major refactoring of `applyFunction`. I have methodically fixed all build errors across the test suite. I also diagnosed and fixed several panics that arose after the build was repaired, which were caused by incorrect assumptions in test setup about how the evaluator's environment caching works. The test suite is now stable (no panics), providing a reliable baseline for feature work.

2.  **Root Cause Analysis**: After stabilizing the tests, I was able to reliably reproduce the core feature failures. My analysis, aided by extensive logging, has pinpointed the root cause:
    When a type switch or `if-ok` assertion creates a new, type-narrowed variable (e.g., `v`), the evaluator correctly identifies the new type but creates a new, generic `SymbolicPlaceholder` for it. This new placeholder loses the connection to the *original concrete object* that the interface variable (`i`) held. Consequently, when a method call or field access is performed on `v` (e.g., `v.Greet()` or `v.Name`), the evaluator cannot access the fields or dispatch to the methods of the original concrete object.

## The Confirmed Plan

To fix this, the link between the new variable `v` and the original object `i` must be maintained. The `symgo/object/object.go` file already contains the necessary field for this on the `SymbolicPlaceholder` struct: `Original object.Object`.

The plan consists of three parts:

1.  **Set the `Original` field**: In `evaluator.go`, modify `evalTypeSwitchStmt` and the `if-ok` logic within `evalAssignStmt`. When creating the `SymbolicPlaceholder` for the new type-narrowed variable, set its `Original` field to point to the original object that was being asserted.

2.  **Use the `Original` field**: In `evaluator.go`, modify `evalSelectorExpr`. This function handles expressions like `v.Name`. Add logic at the beginning to check if the receiver (`v`) is a placeholder with a non-nil `Original` field. If it is, the selector logic (finding the field or method) must be performed on the `placeholder.Original` object, not the placeholder itself. This will correctly resolve members on the concrete value.

3.  **Rename `evalSelectorExpr`**: To implement the above cleanly, the existing `evalSelectorExpr` logic should be renamed to `evalSelectorExprForObject`, and a new `evalSelectorExpr` wrapper function should be created to contain the new logic that checks for the `Original` field.

## Blockage

I have repeatedly failed to apply the necessary patches to `evaluator.go` with the `replace_with_git_merge_diff` tool. The tool consistently reports that the search block cannot be found, which indicates my local understanding of the file's content is out of sync with the true state on the machine, likely due to prior, partially successful patch attempts.

## Next Steps for Successor

1.  Carefully apply the three logical changes described above to `symgo/evaluator/evaluator.go`.
2.  Run `go test -v github.com/podhmo/go-scan/symgo/evaluator` to confirm that `TestTypeSwitch_MethodCall`, `TestIfOk_FieldAccess`, and `TestTypeSwitch_Complex` now pass.
3.  Address any remaining test failures.
4.  Submit the final, passing code.

---
---

# Continuing `symgo` Type Switch and Assertion Implementation (6)

This document outlines the next steps for completing the `symgo` type switch and `if-ok` type assertion feature.

## Current Status

The following work has been completed:
1.  **Environment Population Fix**: The `symgo` evaluator now correctly populates package environments with type definitions, resolving "identifier not found" errors for type names.
2.  **Struct Literal Evaluation Fix**: The `evalCompositeLit` function now correctly populates the fields of struct instances created from literals (both keyed and positional), enabling correct field/method access on them.
3.  **Core Type-Narrowing Verified**: The combination of the above fixes has unblocked the existing type-narrowing logic in `evalSelectorExpr`. The primary tests for this feature (`TestTypeSwitch_MethodCall`, `TestIfOk_FieldAccess`, `TestTypeSwitch_Complex`) are now passing.

## Remaining Tasks

While the core functionality for struct types is working, further work is needed to make the implementation robust and complete.

### 1. Fix Failures in Interface-Related Tests

Several tests related to interface method calls are still failing. These failures are likely related but need to be investigated individually. The primary goal is to ensure that when a type assertion or type switch narrows a value to an interface type, method calls on that narrowed value are resolved correctly.

**Relevant Failing Tests:**
-   `TestEval_ExternalInterfaceMethodCall`
-   `TestInterfaceBinding`
-   `TestEval_InterfaceMethodCall`
-   `TestDefaultIntrinsic_InterfaceMethodCall`

The investigation should focus on how the evaluator handles method calls on symbolic placeholders that represent interface types.

### 2. Add Tests for Scan Policy Behavior

The current tests do not explicitly cover how type assertions and type switches behave when the target types are in packages that are inside or outside the configured scan policy. New tests should be added to cover these scenarios:

-   **In-Policy Assertion**: A test where `i.(T)` is evaluated, and the package containing type `T` is within the scan policy. The assertion should behave as it does now.
-   **Out-of-Policy Assertion**: A test where `i.(T)` is evaluated, but the package for `T` is *not* in the scan policy. The evaluator should handle this gracefully, likely by treating `T` as an unresolved type and the result of the assertion as a symbolic placeholder, rather than crashing. This is crucial for the robustness of tools like `find-orphans`.
-   These tests should be created for both `if-ok` style assertions and `switch-type` statements.

This will ensure the feature is fully integrated with `symgo`'s core philosophy of handling external dependencies symbolically.

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
