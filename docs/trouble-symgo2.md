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

---
# (UNRESOLVED) `find-orphans` Fails with Metacircular Analysis Error

## 1. Problem

The `find-orphans` tool, when run via `make -C examples/find-orphans`, consistently fails with an error indicating a failure in metacircular analysis (the `symgo` engine analyzing its own code).

**Error Log:**
```
level=ERROR msg="undefined method or field: WithReceiver for pointer type INSTANCE"
```

The log indicates the error occurs within a `type switch case` when the evaluator tries to resolve a method on what it believes is an `*object.Instance`, but the method (`WithReceiver`) belongs to the `*object.Function` type.

## 2. Root Cause Analysis

The issue stems from a fundamental challenge in `symgo`'s design: how it represents its own internal object types during analysis.

1.  **Type-Switch Evaluation**: The error is triggered inside `evalTypeSwitchStmt`. When analyzing Go code that contains a `type-switch`, `symgo` correctly identifies the type in each `case` statement.
2.  **Metacircular Clash**: When the code being analyzed is `symgo`'s own source, a `case` might be of type `*object.Function`. The `scanner` correctly identifies `object.Function` as a `struct` (because it is defined as a struct in Go).
3.  **Incorrect Object Creation**: Consequently, `evalTypeSwitchStmt` enters a code path that creates an `*object.Instance` to represent the variable in the `case` block, because it resolved the type to `StructKind`.
4.  **Method Call Failure**: Later, when the analyzer encounters a method call on this variable (e.g., `baseFn.WithReceiver(...)`), it tries to find the method on the `*object.Instance`. The instance does not have the `WithReceiver` method, leading to the "undefined method" error.

The core problem is that the evaluator creates a generic `Instance` to represent a very specific internal type (`Function`) that has its own unique behavior and methods within the evaluator's object model. The evaluator lacks a mechanism to distinguish between a regular struct and its own internal types during this process.

## 3. Status

**Unresolved.** Multiple attempts to fix this by patching `evalAssignStmt` and `evalGenDecl` were unsuccessful, as they did not address the core issue within `evalTypeSwitchStmt`. The problem requires a more profound change to how the evaluator handles its own types during symbolic execution, which is beyond the scope of simple patches.