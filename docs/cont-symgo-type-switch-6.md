# Continuing `symgo` Type Switch and Assertion Implementation

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
