# Trouble: `symgo` Evaluator Enters Infinite Loop on Recursive Method Calls

This document details a bug in the `symgo` evaluator that caused it to enter an infinite loop when analyzing code with simple recursive method calls.

## 1. Problem Description

The `symgo` symbolic execution engine is designed to trace Go code to build call graphs and other analyses. A key feature for robustness is its ability to handle recursive functions without getting stuck in an infinite loop. The engine is supposed to use a "bounded recursion" strategy, where it explores one level of recursion and then halts analysis for that path.

However, a bug was discovered where a direct recursive method call (e.g., a method `Recurse` on a struct `S` that calls itself on the same receiver `s.Recurse()`) was not being detected. This caused the evaluator to loop indefinitely until the program either crashed or was terminated by a timeout.

## 2. Root Cause Analysis

The root cause was an incorrect and unreliable check in the recursion detection logic within `symgo/evaluator/evaluator.go`. The logic has gone through several incorrect iterations:

1.  **Original Buggy Logic**: Compared the source position of the method call's *receiver* (`f.ReceiverPos`). This is flawed because in a typical recursive call, the call site within the method is at a different source position than the initial call site, so their positions would never match.
2.  **Incorrect Fix Attempt**: Changed the logic to compare the receiver *object pointers* (`f.Receiver == frame.Fn.Receiver`). This was also incorrect because the `symgo` engine does not guarantee object identity across different evaluation contexts.

The core misunderstanding was thinking that instance-level tracking was necessary. The design of `symgo` is to trace the call graph at the **type definition level**. For recursion detection, it doesn't matter if `s1.Method()` calls `s1.Method()` or if `s1.Method()` calls `s2.Method()`; if `s1` and `s2` are the same type, it is a recursion on the method *definition*.

## 3. The Fix

The final, correct fix is to unify the recursion check for both methods and plain functions by using the most stable identifier for a function definition: the source position of its declaration.

The logic now compares the `Pos()` of the `*ast.FuncDecl` node associated with the function definition (`f.Def.AstDecl.Pos()`). This is a canonical, robust way to identify a specific function definition in the source code.

The corrected logic is:

```go
// Corrected logic in applyFunction's loop over the call stack
if frame.Fn != nil && frame.Fn.Def != nil && frame.Fn.Def.AstDecl != nil &&
    f.Def.AstDecl != nil && frame.Fn.Def.AstDecl.Pos() == f.Def.AstDecl.Pos() {
    recursionCount++
}
```

This correctly identifies when the same function/method *definition* appears again on the call stack and allows the bounded recursion to trigger as designed.

## 4. Verification

A new regression test, `TestRecursiveMethodCallNotCached`, was added to `symgo/evaluator/evaluator_test.go`. This test defines a simple recursive method and uses a test intrinsic to count the number of times it is symbolically executed.

-   **Before the fix**: The evaluator would enter an infinite loop, causing the test to time out.
-   **After the fix**: The test passes. The recursion is correctly bounded, and the method is called exactly twice (the initial call, and one level of recursion) before the analysis for that path is halted. All other existing tests continued to pass, confirming that the fix did not introduce any regressions.
