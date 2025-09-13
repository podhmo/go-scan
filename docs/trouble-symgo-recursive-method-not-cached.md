# Trouble: `symgo` Evaluator Enters Infinite Loop on Recursive Method Calls

This document details a bug in the `symgo` evaluator that caused it to enter an infinite loop when analyzing code with simple recursive method calls.

## 1. Problem Description

The `symgo` symbolic execution engine is designed to trace Go code to build call graphs and other analyses. A key feature for robustness is its ability to handle recursive functions without getting stuck in an infinite loop. The engine is supposed to use a "bounded recursion" strategy, where it explores one level of recursion and then halts analysis for that path, returning a symbolic placeholder.

However, a bug was discovered where a direct recursive method call (e.g., a method `Recurse` on a struct `S` that calls itself on the same receiver `s.Recurse()`) was not being detected. This caused the evaluator to loop indefinitely until the program either crashed or was terminated by a timeout.

This contradicted the design outlined in `docs/analysis-symgo-implementation.md`, which stated that recursion detection should be robust.

## 2. Root Cause Analysis

The root cause was an incorrect assumption in the recursion detection logic within `symgo/evaluator/evaluator.go`, inside the `applyFunction` method. The logic attempted to distinguish between plain functions and methods, applying a different check for methods.

```go
// Previous (buggy) logic
if f.Receiver != nil {
    // For methods, check if the receiver expression's source position is the same.
    if frame.Fn.Receiver != nil && frame.ReceiverPos.IsValid() && frame.ReceiverPos == f.ReceiverPos {
        recursionCount++
    }
} else {
    // For plain functions...
    recursionCount++
}
```

The check for methods compared the source code position of the method call's receiver (`f.ReceiverPos`). This is flawed because in a typical recursive call like `s.Recurse()`, the call site within the method is at a different source position than the initial call site, so their positions would never match.

The core misunderstanding was thinking that instance-level tracking (e.g., comparing receiver objects or positions) was necessary. The design of `symgo` is to trace the call graph at the **type definition level**. For recursion detection, it doesn't matter if `s1.Method()` calls `s1.Method()` or if `s1.Method()` calls `s2.Method()`; if `s1` and `s2` are the same type, it is a recursion on the method *definition*.

## 3. The Fix

The fix was to simplify and unify the recursion detection logic. The check now relies on the canonical `*scanner.FunctionInfo` object (stored in `f.Def`) which uniquely represents a function or method definition.

The special `if f.Receiver != nil` block was removed entirely. The logic now correctly checks if the `FunctionInfo` definition from the current call frame (`f.Def`) has been seen before in a previous frame on the call stack.

The corrected logic is:

```go
// Corrected logic in applyFunction's loop over the call stack
if frame.Fn == nil || frame.Fn.Def != f.Def {
    continue
}
// If we reach here, it means frame.Fn.Def == f.Def.
// This is a recursive call, regardless of whether it's a method or a plain function.
recursionCount++
```

This correctly identifies when the same function/method *definition* appears again on the call stack and allows the bounded recursion to trigger as designed, aligning with `symgo`'s purpose as a type-level symbolic tracer.

## 4. Verification

A new regression test, `TestRecursiveMethodCallNotCached`, was added to `symgo/evaluator/evaluator_test.go`. This test defines a simple recursive method and uses a test intrinsic to count the number of times it is symbolically executed.

-   **Before the fix**: The evaluator would enter an infinite loop, causing the test to time out.
-   **After the fix**: The test passes. The recursion is correctly bounded, and the method is called exactly twice (the initial call, and one level of recursion) before the analysis for that path is halted. All other existing tests continued to pass, confirming that the fix did not introduce any regressions.
