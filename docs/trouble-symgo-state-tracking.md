# State Tracking Issue in `symgo`: Global Variables and Method Calls

## Overview

The `symgo` symbolic execution engine currently has an issue with tracking type information across state changes, specifically when the result of a function call is assigned to a global variable and methods are subsequently called on that variable.

This can cause tools like `find-orphans` to incorrectly report methods as "orphans," as well as the factory functions that create the objects on which those methods are called.

## Problem Details

The root cause lies in how the `symgo` evaluator handles the evaluation of variables.

1.  **Function Call and Variable Assignment**:
    Given the following code:
    ```go
    // a.go
    var instance = factory.New() // New() returns *MyType
    ```
    `symgo` correctly evaluates the call to `factory.New()` and marks `New()` itself as "used."

2.  **Loss of Type Information**:
    The `*MyType` object returned by `New()` is assigned to the global variable `instance`. At this point, the `symgo` `Variable` object does not fully retain the detailed type information (`*MyType` in this case) from the value it was assigned (`*object.Instance`).

3.  **Failed Method Call Resolution**:
    The problem becomes apparent when the `instance` variable is used later in another function (e.g., `init()`).
    ```go
    // a.go
    func init() {
        instance.DoSomething()
    }
    ```
    When `symgo` evaluates `instance.DoSomething()`, it evaluates the `instance` identifier, but in the process, the precise type information (`*MyType`) that the variable should have is lost. Therefore, it cannot find the method named `DoSomething`.

As a result, the `(*MyType).DoSomething` method is considered "unused." Furthermore, a tool like `find-orphans` may see that none of the methods of the object returned by `New()` are ever used, and may ultimately conclude that the `New()` function itself is also an orphan.

## Example of Affected Code

The following code provides a minimal example that reproduces this issue.

```go
package main

type MyType struct {}

// This factory function returns *MyType
func NewMyType() *MyType {
	return &MyType{}
}

// This method is called in init(), but symgo fails to trace it.
func (t *MyType) DoSomething() {}

// A global variable holds the result of the factory function.
var instance = NewMyType()

func init() {
	// Because the type info for 'instance' is lost, this method call is not resolved.
	instance.DoSomething()
}

func main() {}
```

When analyzing this code with `find-orphans`, both `NewMyType` and `(*MyType).DoSomething` are likely to be reported as orphans.

## Future Action

To resolve this issue, the `symgo/evaluator` needs to be modified to ensure that `object.Variable` correctly retains and propagates the type information of the values assigned to it. This requires careful changes to the core of the engine.

This issue is reproduced in the test case located at `symgo/integration_test/global_var_state_test.go` and will be used to verify a future fix.

## Failed Attempts and Analysis (2025-08-29)

A significant attempt was made to fix this issue by ensuring that the `object.Variable` for `instance` correctly received and stored the type information from the value returned by `NewMyType()`. The primary hypothesis was that the `Variable` object itself was losing the type, which prevented `evalSelectorExpr` from resolving the method `DoSomething`.

### The Attempted Fix

A three-part fix was implemented in `symgo/evaluator/evaluator.go`:

1.  **Modified `evalGenDecl`**: The logic for `var` declarations was changed to prioritize the type information (`TypeInfo` and `FieldType`) from the evaluated right-hand side (RHS) value over any statically declared type on the left-hand side. The goal was for `var instance = NewMyType()` to create a `Variable` for `instance` that inherited the type of the `*object.Pointer` returned by the RHS.

2.  **Modified `assignIdentifier`**: The logic for standard `=` assignments was updated. After `v.Value = val`, the code was modified to also execute `v.SetTypeInfo(val.TypeInfo())` and `v.SetFieldType(val.FieldType())`. This was to ensure that re-assigning a variable would also update its type.

3.  **Cleaned up `evalIdent`**: A block of code was removed from `evalIdent` that copied `TypeInfo` from a `Variable` back to its `Value`. This logic seemed to be a patch for the very problem being fixed, and its removal was intended to prevent incorrect type propagation.

### Analysis of Failure

Despite these logically sound changes, the test case continued to fail. The method call `instance.DoSomething()` was still not being resolved. This indicates a more subtle issue in the evaluator's logic or the underlying type system.

My logical trace of the execution was as follows:
1.  `evalGenDecl` evaluates `NewMyType()`, which results in an `*object.Pointer`. My changes ensure the `FieldType` of this pointer (which correctly indicates `IsPointer: true`) and its `TypeInfo` are copied to the new `*object.Variable` for `instance`.
2.  Later, `evalSelectorExpr` for `instance.DoSomething` correctly retrieves the `*object.Variable` from the environment.
3.  The `switch` statement enters the `case *object.Variable:`.
4.  It calls `findMethodOnType`, passing the `TypeInfo` from the variable.
5.  Inside `findMethodOnType`, the pointer compatibility check `if isMethodPtrRecv && !isVarPointer` should pass. `isMethodPtrRecv` is true for `DoSomething`, and `isVarPointer` should also be true because it is derived from the `Variable`'s `FieldType`, which was correctly set in step 1.

The continued failure implies that one of these steps is not behaving as expected. The "actual values" at runtime must be different from my trace. For example, `v.FieldType().IsPointer` might be unexpectedly false, or the `TypeInfo` passed to `findMethodOnType` might be `nil` or incorrect in a way that causes the base type name comparison to fail.

## Possible Subtasks for Resolution

This problem is difficult to debug without a step-through debugger. To move forward, it can be broken down into the following subtasks:

1.  **Subtask 1: Enhanced Logging**: Add fine-grained logging inside `evalSelectorExpr` (in the `*object.Variable` case) and `findMethodOnType`. The logs should print the exact `TypeInfo.Name`, the full `FieldType` struct, and the results of pointer-ness checks (`isMethodPtrRecv`, `isVarPointer`) for both the variable being accessed and the method being considered. This would definitively reveal the mismatch.

2.  **Subtask 2: Pointer Type Representation Review**: Conduct a dedicated review of how pointer types are handled across `symgo` and `go-scan`. A key question: should an `*object.Pointer` have its own unique `TypeInfo` that represents a pointer type, instead of borrowing the `TypeInfo` of the element it points to? This might require changes in `evalUnaryExpr` (for the `&` operator) to create or fetch a `TypeInfo` for `*T` when it sees a `T`.

3.  **Subtask 3: Isolate `findMethodOnType`**: Write a new, focused unit test specifically for the `findMethodOnType` function. This test should manually construct `object.Variable`, `scanner.TypeInfo`, and `scanner.FieldType` objects to test the pointer compatibility logic in complete isolation from the rest of the evaluator. This would confirm whether the method matching logic itself is flawed.
