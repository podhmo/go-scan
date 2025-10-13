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
# `symgo`: Method Call on Pointer to Named Function Type Fails

This document outlines a bug where method calls on pointers to named function types fail during metacircular analysis (i.e., when `symgo` analyzes its own logic).

## 1. Problem

When analyzing code that involves a type assertion to a pointer-to-named-function-type, the `symgo` evaluator fails with an `undefined method or field` error. This scenario is particularly triggered when `symgo` analyzes its own test code or internal logic which uses these patterns.

**Error Log:**
```
level=ERROR msg="undefined method or field: WithReceiver for pointer type FUNCTION"
```

**Example Go Code:**
```go
package main

// A named function type with a method
type MyFunc func()
func (f *MyFunc) WithReceiver(r interface{}) *MyFunc {
	return f
}

// A function returning the type as an interface{}
func Get() interface{} {
	var f MyFunc = func() {}
	return &f
}

func main() {
	// Type assertion to the pointer type
	f, ok := Get().(*MyFunc)
	if !ok {
		return
	}
	// This call fails during symgo analysis
	f.WithReceiver(nil)
}
```

## 2. Root Cause Analysis

The problem was a combination of two distinct issues in the evaluator:

1.  **Missing `TypeInfo` Propagation in `evalGenDecl`**: When the statement `var f MyFunc = func() {}` was evaluated, the `*object.Function` created for the `func() {}` literal was not being assigned the `TypeInfo` of `MyFunc`. Without this type information, the evaluator had no way of knowing that methods like `WithReceiver` were associated with it.

2.  **Incorrect `ReturnValue` Handling in `evalAssignStmt`**: When the type assertion `f, ok := Get().(*MyFunc)` was evaluated, the `Get()` function call correctly returned a `*object.Pointer`. However, this result was wrapped in an `*object.ReturnValue` by the call evaluation logic. The `evalAssignStmt` function failed to "unwrap" this `ReturnValue` before processing the type assertion. As a result, it was attempting to clone the `ReturnValue` object itself, not the `Pointer` object inside it, leading to incorrect object types being assigned to `f`.

These two issues combined meant that even if one was fixed, the other would still cause the analysis to fail, which complicated the debugging process.

## 3. Solution

A two-part fix was implemented:

1.  **In `evaluator_eval_gen_decl.go`**: The `evalGenDecl` function was modified. When it detects a `var` declaration where a function literal is being assigned to a variable with an explicit named type (e.g., `MyFunc`), it now correctly propagates the `TypeInfo` for that named type to the `*object.Function` object.

2.  **In `evaluator_eval_assign_stmt.go`**: The `evalAssignStmt` function was modified. In the block that handles two-value type assertions (`v, ok := x.(T)`), a check was added to unwrap any `*object.ReturnValue` from the object being asserted (`x`) before any further processing.

Together, these changes ensure that function objects receive their correct type information at their declaration site, and that type assertions correctly operate on the underlying values, not on temporary wrapper objects.

## 4. Plan

1.  **[COMPLETED]** Add a new regression test (`TestTypeAssertion_PointerToFuncType`) to `symgo/evaluator/` to reproduce the failure.
2.  **[COMPLETED]** Implement the two-part fix in `evalGenDecl` and `evalAssignStmt`.
3.  **[COMPLETED]** Verify that all tests pass.
4.  **[COMPLETED]** Update this document.
5.  **[COMPLETED]** Update `TODO.md`.