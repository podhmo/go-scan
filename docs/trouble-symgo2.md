# `symgo`: Method Call on Pointer Return Value Fails

This document outlines a bug in the `symgo` evaluator where it fails to resolve method calls on pointer types returned directly from functions.

## 1. Problem

The `symgo` evaluator throws an `undefined method or field` error when analyzing code that calls a method on a pointer that was returned from a function.

**Error Log:**
```
level=ERROR msg="undefined method or field: String for pointer type RETURN_VALUE"
```

**Example Go Code:**
```go
// timeutil/date.go
package timeutil

type Date string

func (d Date) String() string {
    return string(d)
}

// main.go
package main

func GetDate() *timeutil.Date {
    d := timeutil.Date("2024-01-01")
    return &d
}

func main() {
    // This call fails during symgo analysis
    s := GetDate().String()
}
```
In standard Go, this is perfectly valid. The `String()` method, which has a value receiver `(d Date)`, is correctly called on the pointer `*timeutil.Date` via automatic pointer dereferencing. The `symgo` engine fails to replicate this behavior.

## 2. Root Cause Analysis

The issue lies in `symgo/evaluator/evaluator.go`, specifically within the `evalSelectorExpr` function, which handles expressions like `x.y`.

1.  **Function Call Evaluation:** The call to `GetDate()` is evaluated first. The `symgo` engine correctly determines it returns a pointer to a `Date` object. The result is wrapped in an `*object.ReturnValue`.
2.  **Selector Expression Evaluation:** The `evalSelectorExpr` function is then called to resolve `.String()`.
    -   It correctly unwraps the `*object.ReturnValue` at the beginning of the function, so the expression it analyzes (`left`) becomes an `*object.Pointer`.
    -   It enters the `case *object.Pointer:` block.
    -   Inside this block, it inspects the pointer's `Value` (the `pointee`).
3.  **The Failure:** The `pointee` in this scenario is an `*object.Instance` of `Date`. However, the logic inside the `case *object.Pointer:` block does not check for registered intrinsics, which are essential for testing and for handling certain standard library functions. It attempts to resolve the method directly from type information, but because the test relies on an intrinsic, the call is never registered.

As a result, the test assertion fails. The original user-reported error was slightly different but stemmed from the same core issue: incomplete logic for handling pointer receivers.

## 3. Proposed Solution

The fix is to enhance the `case *object.Pointer:` block in `evalSelectorExpr`.

1.  **Unwrap ReturnValue:** Add a check to see if the `pointee` is of type `*object.ReturnValue` and unwrap it. This handles cases where a function returns a pointer to a pointer.
2.  **Add Intrinsic Checks:** Before resolving the method from type info, add checks for registered intrinsics for both pointer (`(*T).Method`) and value (`(T).Method`) receivers, mirroring the logic that already exists for `*object.Instance` receivers.

This change will make the symbolic evaluator correctly model Go's automatic pointer dereferencing for method calls and ensure that intrinsics for pointer types are correctly resolved.

## 4. Plan

1.  **[COMPLETED]** Create this document (`docs/trouble-symgo2.md`).
2.  **[COMPLETED]** Add a new test case in `symgo/evaluator/evaluator_call_test.go` to reproduce the bug.
3.  **[COMPLETED]** Run tests to confirm the failure.
4.  **[COMPLETED]** Modify `evalSelectorExpr` in `symgo/evaluator/evaluator.go` to implement the fix.
5.  **[COMPLETED]** Run tests to verify the fix and check for regressions.
6.  **[COMPLETED]** Update `TODO.md`.