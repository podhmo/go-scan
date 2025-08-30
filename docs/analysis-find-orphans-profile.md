# Performance Analysis of `find-orphans` Command

This document outlines the investigation into a performance degradation observed in the `find-orphans` command after a specific commit that added support for more Go built-in functions to the symbolic execution engine.

## 1. The Problem

A significant performance slowdown was noticed in the `find-orphans` command. The command's purpose is to identify unused (orphaned) functions within a Go project by performing a symbolic execution trace starting from known entry points (e.g., `main.main`, `init`, exported functions).

The performance issue was linked to a commit that introduced placeholder implementations for a wide range of Go's standard built-in functions, such as `len`, `append`, `make`, `new`, `cap`, etc.

## 2. Investigation and Root Cause Analysis

The investigation involved analyzing the interaction between the `find-orphans` command's logic and the `symgo` symbolic execution engine.

1.  **`find-orphans` Usage Tracking:** The command works by registering a "default intrinsic" function with the `symgo` interpreter. This default intrinsic acts as a global hook, being triggered for **every function call** encountered during the symbolic execution. Its job is to add the called function to a `usageMap`, thereby marking it as "used".

2.  **The Role of the New Built-ins:** Before the problematic commit, calls to built-in functions like `len()` were not resolved by the `symgo` interpreter and were effectively ignored. The commit changed this by making the interpreter aware of these built-ins, resolving them to special `object.Intrinsic` objects.

3.  **Identifying the Bottleneck:** The performance degradation occurred because the `find-orphans`'s generic usage-tracking hook (`defaultIntrinsic`) was now being executed for every single call to common built-ins like `len`, `append`, and `make` throughout the entire analyzed codebase. The logic inside this hook (which involves type assertions, lookups, and string formatting to generate a full function name) is non-trivial. Executing it for thousands of these common calls created a significant and unnecessary overhead, as the tool's goal is to track user-defined functions, not Go's built-ins.

## 3. The Solution

The solution was to make the `defaultIntrinsic` hook in `find-orphans` more intelligent. The goal was to prevent it from running its expensive tracking logic for built-in functions.

The fix, implemented in `examples/find-orphans/main.go`, was to add a check at the very beginning of the default intrinsic's body:

```go
interp.RegisterDefaultIntrinsic(func(i *symgo.Interpreter, args []object.Object) object.Object {
    // ... (omitted comments)

    // Performance optimization: If the function being called is a built-in intrinsic
    // (like `len`, `append`, etc.), we don't need to track its usage. This avoids
    // a significant overhead from processing these very common function calls.
    if len(args) > 0 {
        if _, ok := args[0].(*object.Intrinsic); ok {
            return nil
        }
    }

    // Original usage tracking logic...
    for _, arg := range args {
        markUsage(arg)
    }
    return nil
})
```

This change leverages the fact that the `symgo` engine represents built-in functions as `*object.Intrinsic` and user-defined functions as `*object.Function`. By checking if the called object (the first element in `args`) is an `*object.Intrinsic`, we can effectively and cheaply filter out all built-in calls, restoring the command's performance without affecting its correctness.

The change was verified by running the project's full test suite (`make test`), which passed successfully.
