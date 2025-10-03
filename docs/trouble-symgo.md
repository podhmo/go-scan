# `symgo` Evaluator Error: `not a function: NIL`

This document outlines the investigation and resolution of a `not a function: NIL` error that occurs when running the `find-orphans` tool.

## 1. Problem Description

When running `make -C examples/find-orphans`, the output file `find-orphans.out` contains the following error message:

```
level=ERROR msg="not a function: NIL" in_func=nil in_func_pos=/app/examples/minigo/main_test.go:28:4 exec_pos=/app/symgo/evaluator/evaluator_apply_function.go:415
```

This error indicates that the `symgo` symbolic execution engine is attempting to apply (i.e., call) an object that has a value of `NIL`.

## 2. Investigation

The error originates from the analysis of `examples/minigo/main_test.go`. The position `:28:4` points to the area around the following code inside the `runCase` helper function:

```go
if setup != nil {
    setup(interp) // The call that can cause the issue
}
```

The `setup` parameter is a function of type `func(interp *minigo.Interpreter)`. In several test cases, `runCase` is called with `nil` for the `setup` argument.

`symgo` is designed to explore all possible code paths, including the bodies of `if` statements, regardless of the condition's static value. Therefore, it analyzes the `setup(interp)` call even in test cases where `setup` is `nil`.

The `symgo` evaluator (`applyFunctionImpl`) receives the `object.NIL` object as the function to be executed. The existing implementation does not have a specific case to handle `NIL`, so it falls through to the `default` case in the `switch` statement, which generates the "not a function: NIL" error.

A static analysis tool should not crash in this scenario. Instead, it should recognize the situation (a potential nil-dereference in the source code), log a warning, and continue the analysis by returning a symbolic placeholder for the result of the call.

## 3. Solution

The solution involves modifying the `symgo/evaluator/evaluator_apply_function.go` file to handle this case gracefully.

1.  **Add a `case object.NIL`:** A new case will be added to the `switch fn.(type)` block in the `applyFunctionImpl` function.
2.  **Log a Warning:** Inside this new case, the evaluator will log a `WARN` level message indicating that it has detected a call to a `nil` function. This provides visibility into potential runtime errors in the analyzed code.
3.  **Return a Placeholder:** The evaluator will return a `SymbolicPlaceholder` object. This allows the analysis to continue as if the function had been called and returned an unknown value, which is the correct behavior for a symbolic tracer.

This change prevents the evaluator from crashing and makes it more robust when analyzing code with potential nil-dereference bugs. A regression test will be added to ensure this behavior is maintained.