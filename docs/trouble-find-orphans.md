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

## 4. Troubleshooting Log and Current Status

An attempt was made to implement the proposed solution. This log details the steps taken, the issues encountered, and the current (unresolved) state.

### 4.1. Initial Implementation
The initial implementation focused on two main changes:
1.  **`object.Variable` Update**: The `LastConcreteType` field was replaced with `PossibleConcreteTypes map[string]*scanner.TypeInfo` to track a set of types.
2.  **Control-Flow Merging**: A copy-on-write strategy for assignments and a merge mechanism in `evalIfStmt` were implemented. The idea was that assignments within `if/else` branches would create shadowed variables in a local scope, and `evalIfStmt` would merge the `PossibleConcreteTypes` from these shadowed variables back into the parent variable after the branches were executed.

### 4.2. Test-Driven Debugging Cycle
A test case, `TestInterfaceTypeFlow`, was created to verify this functionality. This test immediately failed, leading to a long and difficult debugging cycle.

**Problem 1: Test Setup and `go-scan` API**
-   **Initial Issue**: The test failed because the `go-scan` library's `Scan` method performs an `os.Stat` check before consulting the in-memory file overlay. This made testing with purely in-memory files impossible.
-   **Solution**: The core `goscan.Scanner` and internal `scanner.Scanner` were modified to be overlay-aware, checking for file existence in the overlay before falling back to the filesystem. This was a necessary prerequisite for testing.
-   **Further Test Issues**: A cascade of test setup errors occurred, including `testify` dependency violations (per `AGENTS.md`), `go.mod` resolution problems, and Go test package compilation errors (`flag redefined`, `import cycle`). These were eventually resolved by using a temporary directory with a real `go.mod` file for testing and carefully managing test package declarations.

**Problem 2: The Core Evaluator Bug**
After fixing the test setup, the test still failed. The `PossibleConcreteTypes` map was empty.
-   **Hypothesis 1**: The merge logic in `evalIfStmt` was flawed.
-   **Investigation 1**: Using detailed logging (as suggested by the user), it was discovered that the `mergeBranchEnvs` function was not finding any variables to merge.
-   **Root Cause Discovered**: The `evalBlockStatement` function was creating an unnecessary, extra-nested scope. This caused the shadowed variables created in the `if/else` blocks to be created in a grand-child scope, which was discarded before the `mergeBranchEnvs` function could inspect them.
-   **Fix 1**: The extra scope creation in `evalBlockStatement` was removed.

**Problem 3: Regressions and a New Bug**
-   **New Failure**: After fixing the scope issue, the new test (`TestInterfaceTypeFlow`) passed! However, running the full test suite (`make test`) revealed that this fix had caused numerous regressions in other tests.
-   **Root Cause 2**: The regressions were caused by a change to `evalIdent` to not unwrap `object.Variable` types. This was done to pass type metadata around, but broke existing tests that expected raw values.
-   **Attempted Fix 2**: A new strategy was devised: `evalIdent` would be reverted to its original unwrapping behavior. Instead, `evalSelectorExpr` and `evalIdentAssignment` would be made smarter, fetching the raw `*object.Variable` from the environment when needed, instead of its evaluated value.

### 4.3. Current Unresolved State: Infinite Recursion / Duplicate Case
-   **Current Blocker**: The implementation of "Attempted Fix 2" has proven extremely difficult. It led to a `fatal error: stack overflow` due to an infinite recursion in `evalSelectorExpr`.
-   My attempts to fix the recursion resulted in a `duplicate case *object.Variable in type switch` compiler error. I have been unable to resolve this final compiler error after multiple attempts, indicating a fundamental misunderstanding of the required evaluator logic.

**Conclusion**: The feature is partially implemented but **not functional**. The core logic in `evaluator.go` is in a broken state, and the `TestInterfaceTypeFlow` test fails. The work is being submitted in this state for review, as I have exhausted my current debugging capabilities.
