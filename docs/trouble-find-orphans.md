# Trouble Analysis: `find-orphans` and Interface Method Calls

This document details the investigation into a bug in the `find-orphans` tool related to tracking the usage of interface methods, and outlines a proposed path forward.

## 1. The Core Problem

The `find-orphans` tool needs to determine if a concrete method (e.g., `(*Dog).Speak`) is used. A key challenge is when such a method is called polymorphically through an interface variable (e.g., `var s Speaker = &Dog{}; s.Speak()`).

The analysis requires two key pieces of information:
1.  That a method of the `Speaker` interface was called.
2.  The set of possible concrete types that the `Speaker` variable could hold at the call site.

## 2. Investigation Summary

The investigation revealed a limitation in the `symgo` symbolic execution engine.

-   **Original Behavior:** When `symgo` could determine the concrete type of an interface variable (e.g., it knew `s` held a `*Dog`), it would resolve the method call `s.Speak()` directly to a concrete call to `(*Dog).Speak()`. This was precise, but it completely hid the polymorphic nature of the call from downstream tools like `find-orphans`. The tool never knew that `Speaker.Speak()` was involved.

-   **Attempted Fix:** A fix was implemented in `symgo` to force it to prioritize the variable's static type. This ensured that a call on an interface variable always generated a generic `SymbolicPlaceholder`. While this correctly signaled that a polymorphic call occurred, it went too far in the other direction: it discarded the known concrete type information, losing precision.

This led to the realization that a more sophisticated approach is needed.

## 3. Proposed Future Solution

The ideal solution is to enhance `symgo` to provide richer information to its consumer tools.

### 3.1. Enhance `SymbolicPlaceholder`

The `object.SymbolicPlaceholder` generated for an interface method call should be enhanced. It should contain not just the interface method that was called, but also the set of *possible concrete types* that the interface variable could hold at that point in the execution.

For example:
```go
// object/object.go
type SymbolicPlaceholder struct {
    // ... existing fields ...
    PossibleConcreteTypes []*scanner.TypeInfo // NEW FIELD
}
```

### 3.2. Enhance `symgo`'s Analysis

The `symgo` evaluator needs to be enhanced to track these possible concrete types.

-   When a variable is assigned a value (e.g., `s = &Dog{}`), the evaluator should record that `*Dog` is a possible type for `s`.
-   Crucially, as pointed out during the investigation, the evaluator must handle control flow. If a variable can be assigned different concrete types in different branches, `symgo` should be able to determine that the set of possible types includes all candidates from all reachable paths.
    ```go
    var s Speaker
    if someCondition {
        s = &Dog{}
    } else {
        s = &Cat{}
    }
    s.Speak() // At this point, PossibleConcreteTypes should be {*Dog, *Cat}
    ```

### 3.3. Update `find-orphans`

With this richer `SymbolicPlaceholder`, the `find-orphans` tool can be made much smarter. Its intrinsic would:
1.  Receive a `SymbolicPlaceholder` for `Speaker.Speak()`.
2.  Inspect the new `PossibleConcreteTypes` field.
3.  **If the set is not empty:** Iterate through the concrete types (`*Dog`, `*Cat`) and mark only their `Speak` methods as used. This provides a highly precise analysis.
4.  **If the set is empty** (because `symgo` could not determine any concrete types): Fall back to the original, imprecise strategy of marking all implementations of `Speaker` in the entire codebase as used.

This approach provides the best of both worlds: precision when possible, and a safe fallback when not. This is the recommended path forward to fully resolve the issue.

## 4. Resolution

The proposed solution has been successfully implemented. The final fix required addressing a subtle bug in the symbolic evaluator's environment handling.

### Implementation Details
The following changes were made to the `symgo` evaluator:
1.  **`object.Variable` Update**: The `LastConcreteType` field was replaced with `PossibleConcreteTypes map[string]*scanner.TypeInfo` to track a set of possible concrete types.
2.  **Copy-on-Write Assignments**: The `assignIdentifier` function was modified to implement a copy-on-write strategy. When an assignment (`=`) occurs in a new scope (like an `if` or `else` block) to a variable from an outer scope, the evaluator now creates a new, shadowed variable in the current scope instead of modifying the outer variable directly. This is crucial for isolating the state changes within each control-flow branch.
3.  **Control-Flow Merging**: The `evalIfStmt` function was enhanced. After evaluating the `if` and `else` branches (which now have their own shadowed variables), a new `mergeBranchEnvs` function is called. This function inspects the *local* variables in each branch environment and merges their `PossibleConcreteTypes` back into the original variable in the parent environment.
4.  **Correct Scope Evaluation**: The root cause of the bug was identified and fixed. The `evalBlockStatement` function was incorrectly creating an additional nested scope. This caused the shadowed variables created by the copy-on-write logic to be discarded before they could be merged. The fix was to remove the unnecessary scope creation, ensuring that assignments within a block modify the environment that the `if/else` logic expects.

With these changes, the evaluator can now correctly track all possible concrete types assigned to an interface variable across different `if/else` branches, significantly improving the precision of tools like `find-orphans`. A new test case, `TestInterfaceTypeFlow`, was added to verify this behavior.
