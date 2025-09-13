# Trouble: `symgo` Evaluator Enters Infinite Loop on Recursive Method Calls

This document details a bug in the `symgo` evaluator that caused it to enter an infinite loop when analyzing code with simple recursive method calls.

## 1. Problem Description

The `symgo` symbolic execution engine is designed to trace Go code to build call graphs and other analyses. A key feature for robustness is its ability to handle recursive functions without getting stuck in an infinite loop. The engine is supposed to use a "bounded recursion" strategy, where it explores one level of recursion and then halts analysis for that path, returning a symbolic placeholder.

However, a bug was discovered where a direct recursive method call (e.g., a method `Recurse` on a struct `S` that calls itself on the same receiver `s.Recurse()`) was not being detected. This caused the evaluator to loop indefinitely until the program either crashed or was terminated by a timeout.

This contradicted the design outlined in `docs/analysis-symgo-implementation.md`, which stated that recursion detection should be state-aware and handle method calls correctly.

## 2. Root Cause Analysis

The root cause was traced to the recursion detection logic in `symgo/evaluator/evaluator.go`, specifically within the `applyFunction` method. The logic for detecting a recursive method call was implemented as follows:

```go
// Previous (buggy) logic
if f.Receiver != nil {
    // For methods, check if the receiver expression's source position is the same.
    if frame.Fn.Receiver != nil && frame.ReceiverPos.IsValid() && frame.ReceiverPos == f.ReceiverPos {
        recursionCount++
    }
}
```

This code attempted to detect recursion by comparing the source code position of the method call's receiver (`f.ReceiverPos`) with the receiver position of a previous call on the call stack (`frame.ReceiverPos`).

This approach is flawed for typical recursion. In a function like:

```go
func (s *S) Recurse() {
    // ...
    s.Recurse() // The recursive call is at a different source position.
}
```

The initial call to `s.Recurse()` and the recursive call occur at different lines of code. Therefore, their `ReceiverPos` values will never be equal, and the `recursionCount` will never increment. The check fails to detect the recursion.

The intended behavior, as described in the documentation, was to check if the call was on the **same receiver object**. The information to do this was available, but the check was implemented incorrectly.

## 3. The Fix

The fix was to change the comparison from the receiver's source position to a direct comparison of the receiver objects themselves. The `object.Function` struct already stores a reference to the receiver object (`Receiver`).

The corrected logic is:

```go
// Corrected logic
if f.Receiver != nil {
    // For methods, check if it's the same receiver *object*.
    // This correctly detects recursion on the same instance.
    if frame.Fn.Receiver == f.Receiver {
        recursionCount++
    }
}
```

By comparing `frame.Fn.Receiver == f.Receiver`, we are checking if the pointer to the receiver object in the current call frame is the same as the pointer to the receiver object in a previous call frame on the stack. This correctly identifies that the method is being called on the exact same instance and allows the bounded recursion to trigger as designed.

## 4. Verification

A new test case, `TestRecursiveMethodCallNotCached`, was added to `symgo/evaluator/evaluator_test.go`. This test defines a simple recursive method and uses a test intrinsic to count the number of times it is symbolically executed.

-   **Before the fix**: The test would time out, as the evaluator entered an infinite loop.
-   **After the fix**: The test passes. The recursion is correctly bounded, and the method is called exactly twice (the initial call, and one level of recursion) before the analysis for that path is halted. All other existing tests continued to pass, confirming that the fix did not introduce any regressions.
