# Troubleshooting (2): Member Access on Type-Narrowed Variables

This document details the complex and iterative troubleshooting process for implementing member access on variables narrowed by type assertions (`if v, ok := i.(T)`) and type switches (`switch v := i.(type)`). It supersedes the initial `trouble-symgo-narrowing.md` document.

## 1. Goal

The objective is to enable the `symgo` evaluator to correctly trace method calls and field accesses on variables whose types have been narrowed. For example:

```go
// if-ok Type Assertion
if v, ok := i.(User); ok {
    inspect(v.Name) // `v` should be a `User` with its fields accessible.
}

// Type Switch
switch v := i.(type) {
case Greeter:
    v.Greet() // `v` should be a `Greeter` and its methods callable.
}
```

## 2. The Winding Path of Debugging

The implementation journey was complex, involving several flawed approaches before the true root cause was isolated.

### Attempt 1: The `SymbolicPlaceholder` with `Underlying` Field

- **Idea:** The first attempt was to introduce an `Underlying` field to `object.SymbolicPlaceholder`. The placeholder would represent the new, narrowed type, while `Underlying` would hold a direct reference to the original object's value.
- **Problem:** This led to a tangled web of indirection. The `Underlying` object was often an `*object.Variable` itself, which required multiple layers of "unwrapping" (`v.Value`). The logic in `evalSelectorExpr` and `ResolveSymbolicField` became convoluted, and it was extremely difficult to pinpoint where the concrete value was being lost. This approach was abandoned due to its complexity and lack of success.

### Attempt 2: The "Clone and Overwrite" Strategy

- **Idea:** To simplify, the next approach was to `Clone()` the original object and directly overwrite the type information on the clone using `SetTypeInfo()`. This seemed more direct and robust.
- **Implementation:**
    1. An `object.Object.Clone()` method was added to the interface and implemented across all concrete types.
    2. `evalTypeSwitchStmt` and `evalAssignStmt` were modified to clone the original object and apply the new type.
- **Problem:** This led to a very confusing state: `TestTypeSwitch_MethodCall` **passed**, but `TestIfOk_FieldAccess` **consistently failed**. The `inspect` intrinsic was called, but with an empty string, indicating the `Name` field was not being accessed from a struct with the correct value.

### Attempt 3: The Flawed Compatibility Check

- **Idea:** The discrepancy between the two tests suggested an issue with how the success/failure paths were modeled. The `Clone()` should only happen on a *successful* assertion. A compatibility check using `e.scanner.Implements()` was added.
- **Problem:** This was the correct direction, but my initial implementations were still flawed, leading me to incorrectly believe the issue was elsewhere (e.g., in `evalCompositeLit` or `ResolveSymbolicField`). I went down several rabbit holes, including adding extensive logging, only to return to the same point: the `if-ok` test was still failing.

## 3. The Root Cause, Finally Identified

After multiple resets and re-evaluations, the true, subtle difference between the two control flow structures in the `symgo` evaluator was identified:

- **`type switch`:** The `case` blocks are explored unconditionally by the symbolic tracer. My implementation correctly created a properly-valued clone for the matching `case` and a placeholder for the non-matching `case`. Because the test only inspects the success path, it passed.
- **`if-ok` assertion:** The execution of the `if` block is conditional on the value of the `ok` variable. My implementation was correctly creating a cloned object `v`, but the `ok` variable was being assigned a `SymbolicPlaceholder` for a boolean, not a concrete `object.TRUE` or `object.FALSE`. When `evalIfStmt` evaluated the condition, it could not determine concretely whether to enter the block. While `symgo` is designed to explore both branches of an `if`, the lack of a concrete `true` on the success path appears to be the reason the block's contents were not being evaluated with the correctly-valued `v`.

## 4. The Path Forward: The Definitive Fix

The final, correct strategy is to ensure the `ok` variable in an `if-ok` assertion is assigned a **concrete boolean value**.

1.  **Re-implement the Compatibility Check:** The logic in `evalAssignStmt` for the `if-ok` case must be identical to the successful logic in `evalTypeSwitchStmt`. It will:
    a. Unwrap the original object to get its concrete value (e.g., an `*object.Instance`).
    b. Resolve the target type from the assertion.
    c. Use `e.scanner.Implements()` to check if the original object's type is compatible with the target type.

2.  **Handle Success Path (`isCompatible == true`):**
    - The `v` variable will be assigned a **clone** of the original object with its type info updated.
    - The `ok` variable will be assigned the concrete value **`object.TRUE`**.

3.  **Handle Failure Path (`isCompatible == false`):**
    - The `v` variable will be assigned a **`SymbolicPlaceholder`** representing the zero value of the target type.
    - The `ok` variable will be assigned the concrete value **`object.FALSE`**.

This ensures that when `evalIfStmt` evaluates the condition, it sees a concrete `true` and correctly proceeds to evaluate the `if` block with the properly valued and typed variable `v`. This will be verified by re-running the tests after applying this final, targeted fix to `evalAssignStmt`.