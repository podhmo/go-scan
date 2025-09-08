# Plan: Robust Interface Resolution in `symgo`

This document outlines the plan and history for implementing a robust, two-phase deferred resolution mechanism for interface method calls in the `symgo` symbolic execution engine.

## 1. The Goal

The goal is to implement a two-phase mechanism to ensure that all concrete implementations of a called interface method are correctly identified and marked as "used" by analysis tools.

-   **Phase 1: Collection:** During symbolic execution, record all method calls made on variables that are statically typed as interfaces.
-   **Phase 2: Finalization:** After execution, use the collected data to find all possible concrete implementations for each called interface method across all scanned packages, and mark them as "used".

## 2. Problem Details & Investigation History

### Initial Investigation: Method Set Logic

The initial hypothesis was that the `isImplementer` function in `symgo/evaluator/evaluator.go` did not correctly handle Go's method set rules for pointer receivers. This was addressed by patching the function to check methods on both value types (`T`) and pointer types (`*T`). This part of the logic is now considered correct.

### Deeper Issue: Package Discovery in `Finalize`

Despite the fix to `isImplementer`, tests continued to fail. The investigation revealed that the `Finalize` function's type discovery mechanism was flawed. It relied on an internal, manually-populated `seenPackages` map, which did not include the in-memory packages created by `scantest` for the test cases. As a result, `Finalize` would run with an empty or incomplete set of types.

This was addressed by:
1.  Adding a new `AllSeenPackages()` method to `goscan.Scanner` to expose its complete, internal package cache.
2.  Modifying `Finalize` to use this method as its source of packages, ensuring all `scantest` packages are included.
3.  Filtering these packages against the active `ScanPolicy` to ensure only intended packages are analyzed.

### Latest Findings: State Management Failure in Evaluator

Even with the package discovery issue resolved, the key interface resolution tests (`TestInterfaceResolution`, `TestEval_InterfaceMethodCall_AcrossControlFlow`, etc.) still fail.

A detailed investigation into these failures revealed the current root cause: **the evaluator does not correctly track the state of variables across control-flow branches.**

The `TestEval_InterfaceMethodCall_AcrossControlFlow` test highlights this perfectly. The test uses code similar to the following:
```go
var a Animal // Interface type
if condition {
    a = &Dog{}
} else {
    a = &Cat{}
}
a.Speak() // This call should be linked to both Dog.Speak and Cat.Speak
```
The evaluator correctly explores both the `if` and `else` branches. However, the state modification from one branch (e.g., assigning `&Dog{}` to `a`) is not merged or retained when the other branch is explored. The `PossibleTypes` map on the `Variable` object for `a`, which is supposed to accumulate all possible concrete types, ends up containing only the type from the last-evaluated branch.

This is a fundamental limitation in the evaluator's design. It is path-insensitive (it explores all branches) but does not correctly merge the resulting states. Because the `PossibleTypes` map is incomplete, the `Finalize` function, which relies on this map to connect the `a.Speak()` call to its concrete implementations, cannot find all the correct methods.

## 3. Current Status & Next Steps

-   [x] **Interface Method Set Logic:** The `isImplementer` function correctly handles pointer and value receivers.
-   [x] **Package Discovery:** The `Finalize` function now correctly discovers all packages from the scanner, including in-memory test packages, and filters them by the scan policy.
-   [ ] **State Management Across Branches:** **This is the primary blocker.** The evaluator must be fixed to correctly accumulate possible concrete types for a single interface variable that is assigned different types in different control-flow branches.

The next concrete task is to fix this state management issue within the evaluator. This will likely involve changing how environments and variable states are handled in the `evalIfStmt` function and potentially other control-flow handlers to ensure that side effects from all explored paths are correctly merged or accumulated. After this is fixed, the `Finalize` logic should have the correct data to resolve interface calls properly.
