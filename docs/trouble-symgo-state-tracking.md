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
