# Solution Summary: `find-orphans` and Interface Method Calls

This document details the investigation and final solution for a bug in the `find-orphans` tool related to tracking the usage of interface methods across control-flow branches.

## 1. The Core Problem

The `find-orphans` tool needs to determine if a concrete method (e.g., `(*Dog).Speak`) is used. A key challenge is when such a method is called polymorphically through an interface variable, especially when the concrete type of that variable can change depending on control flow.

For example:
```go
var s Speaker
if someCondition {
    s = &Dog{}
} else {
    s = &Cat{}
}
s.Speak() // How to know this could be Dog.Speak OR Cat.Speak?
```

The analysis requires two key pieces of information:
1.  That a method of the `Speaker` interface was called.
2.  The complete set of possible concrete types that the `Speaker` variable could hold at the call site.

## 2. Investigation Summary

The investigation revealed a limitation in how the `symgo` symbolic execution engine handled variable state across different execution paths. Initial attempts to solve this by creating "shadow" variables in each branch proved overly complex and broke other use cases, such as updating package-level variables from within functions.

## 3. Implemented Solution: Type-Directed Assignment

The final solution is a more elegant model that makes the behavior of an assignment (`=`) dependent on the static type of the variable being assigned to.

### 3.1. Core Principles

1.  **Default to In-Place Updates**: The standard behavior for an assignment (`=`) is to find the variable in its lexical scope and modify it directly. This aligns with Go's semantics and correctly handles most cases, including updates to global variables.

2.  **Special Behavior for Interfaces**: When the variable being assigned to is statically known to be an **interface type**, the logic for tracking possible concrete types changes from "replace" to "append".

### 3.2. How It Works

-   When the evaluator encounters an assignment to an interface variable (e.g., `s = &Dog{}`), it finds the original `s` variable and **adds** the concrete type of the right-hand side (`*Dog`) to a set of `PossibleConcreteTypes` stored on the variable.
-   If another assignment happens in a different branch (e.g., `s = &Cat{}`), it again finds the *same* original `s` and **adds** the new type to the set.
-   By the end of the `if/else` block, the `s` variable's `PossibleConcreteTypes` set correctly contains both `{*Dog, *Cat}`.
-   When `s.Speak()` is called, the `SymbolicPlaceholder` for the call is populated with this complete set of possible types.

### 3.3. `find-orphans` Tool Update

The `find-orphans` tool was updated to consume this richer information. Its intrinsic now:
1.  Receives a `SymbolicPlaceholder` for `Speaker.Speak()`.
2.  Inspects the `PossibleConcreteTypes` field.
3.  Iterates through the concrete types (`*Dog`, `*Cat`) and marks only their `Speak` methods as used.

This solution is both precise and robust, correctly handling control flow without breaking standard assignment semantics in other parts of the evaluator. It also correctly uses `*scanner.FieldType` to preserve pointer information.
