# Continuation Document: Fixing the `symgo` Recursive Method Bug

## 1. Initial Prompt

(Translated from Japanese)
"Please implement one task from TODO.md. If necessary, break it down into sub-tasks. You may write the decomposed tasks back to TODO.md.

Then, proceed with the work. Please also add test code. Continue to modify the code until the tests succeed. Be sure to update TODO.md at the very end of your work.

Please choose a task related to symgo.

It appears that caching is not working correctly around `evalSelectorExpr` in symgo. For example, when trying to analyze the `buildkey` method in `./scanner/scan.go`, it outputs a deep stack trace. This contradicts the analysis results in `docs/analysis-symgo-implementation.md`. It seems that recursion suppression is not working correctly. It appears to be performing a depth-first search repeatedly without reading the whole context.
Please investigate the cause, output a document to `docs/trouble-symgo-recursive-method-not-cached.md`, write a test to reproduce it, and fix the code."

## 2. Goal

The primary goal was to fix a bug in the `symgo` symbolic evaluator where it would enter an infinite loop when analyzing a recursive method call. The task required adding a regression test, fixing the bug, and documenting the findings.

## 3. Initial Implementation Attempt

My first approach was to correctly identify the bug in the recursion detection logic within `symgo/evaluator/evaluator.go`. I created a new test file with a test case, `TestRecursiveMethodCallNotCached`, that defined a simple recursive method.

The initial bug analysis was that the check was comparing the source position of the method's receiver, which is incorrect for simple recursion.
My first proposed fix was to change this to compare the receiver *object instances* directly (`frame.Fn.Receiver == f.Receiver`).

```go
// First attempted fix
if f.Receiver != nil {
    if frame.Fn.Receiver == f.Receiver {
        recursionCount++
    }
} else {
    recursionCount++
}
```
This fix seemed logical from the perspective of a standard interpreter, where tracking object identity is key. The new test case passed with this change because the test used a single object instance.

## 4. Roadblocks & Key Discoveries

The main roadblock was not a system error, but a fundamental misunderstanding of the `symgo` engine's design philosophy.

*   **Incorrect Hypothesis**: I assumed `symgo` worked like a standard interpreter, where tracking the state of individual object instances was important. I believed that distinguishing between calls on `s1.Method()` and `s2.Method()` was necessary.

*   **Key Discovery (from user feedback)**: The user provided critical feedback that corrected my understanding.
    1.  **Object comparison is a known-bad pattern**: This approach had been tried before and caused regressions.
    2.  **`symgo` is not an interpreter**: It is a symbolic tracer. Its goal is to trace the call graph at the *type definition level*, not the instance level.
    3.  **`s1.Method()` vs. `s2.Method()`**: For the purposes of call graph analysis and recursion detection in `symgo`, if `s1` and `s2` are of the same type, these two calls should be treated as the same for detecting recursion.

This was the key insight. The goal is not to see if the *same object* is calling itself, but if the *same method definition* is being called again in the same call stack.

## 5. Major Refactoring Effort

Based on this new understanding, I abandoned the object-identity comparison. The correct approach was to make the recursion check rely only on the canonical definition of the function being called.

The `object.Function` struct contains a field `Def *scanner.FunctionInfo`, which points to the unique definition of the function or method as parsed by the scanner. This `FunctionInfo` is the correct identifier to use for recursion detection.

The logic in `applyFunction` was refactored to be much simpler and more correct. The special handling for methods was removed, and now both plain functions and methods are treated identically for recursion detection.

```go
// Corrected logic
// In a loop over the call stack frames...
if frame.Fn == nil || frame.Fn.Def != f.Def {
    continue
}
// If we reach here, it means frame.Fn.Def == f.Def.
// This is a recursive call, regardless of whether it's a method or a plain function.
recursionCount++
```
This correctly identifies when the same function/method *definition* appears again on the call stack and allows the bounding logic to trigger.

## 6. Current Status

The code is now correct. The fix has been implemented in `symgo/evaluator/evaluator.go`, and a regression test (`TestRecursiveMethodCallNotCached`) has been added to `symgo/evaluator/evaluator_test.go`. All tests in the suite pass, verifying the fix and ensuring no regressions were introduced. A troubleshooting document has also been created. The work is ready for submission.

## 7. References

*   `docs/analysis-symgo-implementation.md`: This document provides the high-level design of the `symgo` engine, which was crucial context.
*   `docs/summary-symgo.md`: The user referenced this as a key document for understanding `symgo`'s non-interpreter design.

## 8. TODO / Next Steps

1.  Update the troubleshooting document `docs/trouble-symgo-recursive-method-not-cached.md` with the final, correct explanation of the bug and fix.
2.  Submit the finalized changes.
