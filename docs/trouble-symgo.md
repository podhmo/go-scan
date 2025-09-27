# `symgo` Robustness Report: `invalid indirect` on Unresolved Function Types

This document details the analysis and resolution of a critical robustness issue within the `symgo` symbolic execution engine, discovered during large-scale analysis with the `find-orphans` tool.

## 1. Problem Description

When running `symgo`-based tools that analyze a wide range of dependencies (including the standard library), the evaluator would frequently crash with an "invalid indirect" error.

**Symptom:**

The logs would show numerous errors of the following form:

```
level=ERROR msg="invalid indirect of <Unresolved Function: path/filepath.WalkFunc> (type *object.UnresolvedFunction)"
level=ERROR msg="invalid indirect of <Unresolved Function: net/http.HandlerFunc> (type *object.UnresolvedFunction)"
```

This error halted the analysis, preventing tools like `find-orphans` from completing their run on a whole-program scope.

## 2. Root Cause Analysis

The issue stemmed from how the evaluator's dereference logic (`*` operator) interacted with the package scanning policy.

1.  **Scanning Policy and Placeholders:** `symgo` uses a `ScanPolicy` to decide which packages to analyze from source. When it encounters a symbol from a package that is *not* in the policy (e.g., most of the standard library in a typical `find-orphans` run), it creates a placeholder object to represent that symbol without analyzing its source.
2.  **`UnresolvedFunction` Type:** For symbols that appeared to be function types or variables of a function type (like `var v *http.HandlerFunc`), the evaluator would create an `*object.UnresolvedFunction` object.
3.  **Dereference Failure:** The core of the bug was in the `evalStarExpr` function, which implements the dereference (`*`) operator. This function had logic to handle dereferencing pointers, instances, and even unresolved *struct/interface* types. However, it was missing a specific case for `*object.UnresolvedFunction`.
4.  **The Crash:** When the evaluator encountered code that involved dereferencing one of these function types (often in `var` declarations like `var V *a.MyFunc`), `evalStarExpr` would be called with the `*object.UnresolvedFunction` object. Lacking a specific handler, it would fall through to the default case, which immediately reported a fatal `invalid indirect of ...` error.

## 3. Solution Implemented

The fix was to make the evaluator more resilient to this specific scenario by treating the dereference of an unresolved function type as a valid symbolic operation.

-   **File Modified:** `symgo/evaluator/evaluator.go`
-   **Function Modified:** `evalStarExpr`

A new case was added to the main `switch` statement in `evalStarExpr`:

```go
// ... inside evalStarExpr, after checking for other types ...

// Handle dereferencing an unresolved function object.
if uf, ok := val.(*object.UnresolvedFunction); ok {
    return &object.SymbolicPlaceholder{
        Reason: fmt.Sprintf("instance of unresolved function %s.%s from dereference", uf.PkgPath, uf.FuncName),
    }
}

// ... rest of the function ...
```

This change ensures that when `symgo` tries to dereference a pointer to a function type it cannot fully resolve, it no longer crashes. Instead, it correctly produces a `*object.SymbolicPlaceholder`, representing a symbolic instance of that function. This allows the analysis to continue, significantly improving the robustness and usability of tools built on `symgo`.

This fix was verified by a new unit test (`TestEvalStarExpr_OnUnresolvedFunction`) that specifically reproduces the bug, and by confirming that the `make -C examples/find-orphans` command now completes without the "invalid indirect" errors in its output log.

---

## 2. Problem: `len` on a Direct Function Call Result

**Symptom:**

During the execution of `find-orphans`, the `symgo` engine would fail with an error indicating that the `len()` built-in function received an unsupported argument type.

```
level=ERROR msg="symbolic execution failed for entry point" function=github.com/podhmo/go-scan/examples/docgen.NewAnalyzer error="symgo runtime error: argument to `len` not supported, got RETURN_VALUE\n"
```

**Root Cause Analysis:**

1.  **Symbolic Execution Flow:** When `symgo` evaluates a function call, it doesn't immediately produce the final, concrete value. Instead, it often returns a special wrapper object, `*object.ReturnValue`, which contains the actual result.
2.  **`len()` Implementation:** The intrinsic implementation for the `len()` built-in function (`symgo/intrinsics/builtins.go:BuiltinLen`) was designed to operate on concrete types like `*object.String`, `*object.Slice`, `*object.Map`, etc.
3.  **The Mismatch:** The code being analyzed contained a pattern like `len(someFunc())`. `symgo` would first execute `someFunc()`, which yielded an `*object.ReturnValue`. This `ReturnValue` object was then passed directly to `BuiltinLen`. The `len` function did not have a case to handle this `ReturnValue` wrapper; it expected the underlying slice or map. This caused it to fall through to the default error case, halting analysis.

**Solution Implemented:**

The fix was to make the `BuiltinLen` function more intelligent by teaching it to unwrap the `*object.ReturnValue` object.

-   **File Modified:** `symgo/intrinsics/builtins.go`
-   **Function Modified:** `BuiltinLen`

A check was added at the beginning of the function:

```go
if ret, ok := args[0].(*object.ReturnValue); ok {
    // If the argument is a return value, unwrap it.
    args[0] = ret.Value
}
```

This ensures that if `len()` is called on the result of a function, it operates on the actual returned value (e.g., the slice) rather than the temporary wrapper, preventing the crash.

---

## 3. Problem: `new` on an Unresolved Function Type

**Symptom:**

When analyzing a large codebase, `symgo` would fail when encountering the `new()` built-in function applied to a function type from an external, unscanned package.

```
level=ERROR msg="symbolic execution failed for entry point" function=github.com/podhmo/go-scan/examples/find-orphans.main error="symgo runtime error: invalid argument for new: expected a type, got UNRESOLVED_FUNCTION\n"
```

**Root Cause Analysis:**

1.  **Unresolved Types:** When `symgo` encounters a type from a package outside its primary analysis scope (e.g., `net/http.HandlerFunc`), it creates a placeholder object, `*object.UnresolvedFunction`, to represent it. This is a correct and necessary part of shallow scanning.
2.  **`new()` Implementation:** The `BuiltinNew` function (`symgo/intrinsics/builtins.go`) is responsible for handling the `new()` built-in. Its purpose is to take a type object and return a pointer to a new instance of that type.
3.  **The Crash:** The `BuiltinNew` function had logic to handle `*object.Type` (for fully resolved types) and `*object.UnresolvedType` (for unresolved structs/interfaces). However, it was missing a case for `*object.UnresolvedFunction`. When code like `v := new(http.HandlerFunc)` was encountered, `BuiltinNew` received the `*object.UnresolvedFunction` placeholder, didn't know how to handle it, and fell through to the default case, which produced the "invalid argument for new" error.

**Solution Implemented:**

To improve robustness, the `BuiltinNew` function was updated to gracefully handle unresolved function types.

-   **File Modified:** `symgo/intrinsics/builtins.go`
-   **Function Modified:** `BuiltinNew`

A new `case` was added to the `switch` statement to detect `*object.UnresolvedFunction` arguments:

```go
case *object.UnresolvedFunction:
    // If we try to new an unresolved function type, it's valid.
    // We can't know the "zero value" but we can return a placeholder for the pointer.
    placeholder := &object.SymbolicPlaceholder{
        Reason: fmt.Sprintf("instance of unresolved function %s.%s", t.PkgPath, t.FuncName),
    }
    pointee = placeholder
```

This change allows `symgo` to continue its analysis when it encounters `new()` applied to function types from external packages, replacing a fatal error with a valid symbolic placeholder.

---

## 4. Problem: `len` on an Unresolved Function Placeholder

**Symptom:**

After fixing the previous issues, a new `len`-related error emerged during the `find-orphans` run.

```
level=ERROR msg="symbolic execution failed for entry point" function=github.com/podhmo/go-scan/examples/minigo.main error="symgo runtime error: argument to `len` not supported, got UNRESOLVED_FUNCTION\n"
```

**Root Cause Analysis:**

1.  **Placeholder Misclassification:** The analysis of `examples/minigo/main.go` involved the expression `len(os.Args)`. `os.Args` is a variable (a slice of strings) from an external, unscanned package. Due to limitations in how `symgo` creates placeholders for external package-level variables, it was incorrectly creating an `*object.UnresolvedFunction` object for `os.Args` instead of a more appropriate placeholder.
2.  **Brittle `len()` Intrinsic:** The `BuiltinLen` function, even after the `ReturnValue` fix, did not have a case to handle being called with an `*object.UnresolvedFunction`.
3.  **The Crash:** When `len(os.Args)` was evaluated, the `*object.UnresolvedFunction` placeholder was passed to `BuiltinLen`, which had no specific case for it and fell through to the default error, crashing the analysis. While the initial misclassification is a deeper issue, the immediate cause of the crash was the lack of robustness in the `len` intrinsic.

**Solution Implemented:**

The `BuiltinLen` function was further hardened to prevent this crash.

-   **File Modified:** `symgo/intrinsics/builtins.go`
-   **Function Modified:** `BuiltinLen`

A new `case` was added to the `switch` statement to handle this scenario gracefully:

```go
case *object.UnresolvedFunction:
    // This can happen if `len` is called on a variable from an unscanned
    // package that is mis-identified as a function. Instead of crashing,
    // return a symbolic placeholder for the length.
    return &object.SymbolicPlaceholder{Reason: "len on unresolved function"}
```

This ensures that even if type information is incomplete or incorrect for an external symbol, `len()` will not crash the analysis. It will instead return a symbolic value, allowing the analysis to continue.

---

## 5. Problem: `symgo` Erroneously Calls `nil` Function Pointers

**Symptom:**

When running analysis on codebases that include test files (`--include-tests`), `symgo` would frequently crash with a "not a function: NIL" error.

```
level=ERROR msg="not a function: NIL" in_func=nil in_func_pos=/app/examples/minigo/main_test.go:28:4
```

This error occurred in code patterns like the following, where a function is called with a `nil` function pointer:

```go
func TakesFunc(fn func()) {
	if fn != nil { // The bug occurred here
		fn()
	}
}

func main() {
	TakesFunc(nil)
}
```

**Root Cause Analysis:**

The problem was a composite failure in how the `symgo` evaluator handled `nil` comparisons and `if` statements.

1.  **`evalBinaryExpr` Failure:** The function responsible for evaluating binary expressions (`==`, `!=`, etc.) did not have specific logic for comparing a nillable type (like a function, interface, or pointer) against the `nil` literal. When it encountered `fn != nil`, instead of returning a concrete `*object.Boolean{Value: false}`, it returned a generic `*object.SymbolicPlaceholder`.

2.  **`evalIfStmt` Assumption:** The function for evaluating `if` statements did not correctly handle cases where the condition evaluated to a non-boolean object. It treated any non-`nil` object (including the `SymbolicPlaceholder` from the failed comparison) as `true`.

The combination of these two issues meant the `if fn != nil` check would always pass symbolically, causing the evaluator to attempt to execute the `fn()` call, which resulted in the "not a function: NIL" crash.

**Solution Implemented:**

A two-part fix was implemented in `symgo/evaluator/evaluator.go`:

1.  **Enhanced `evalBinaryExpr`:** The logic for `==` and `!=` was completely rewritten. It now explicitly checks if one side of the comparison is `nil` and the other side is a nillable type (function, pointer, slice, map, channel, or interface). In these cases, it now correctly returns a concrete `*object.Boolean` object (`object.TRUE` or `object.FALSE`).

2.  **Smarter `evalIfStmt`:** The `if` statement evaluator was updated to check if the condition's result is a concrete `*object.Boolean`. If it is, `evalIfStmt` now executes *only* the correct branch (`then` or `else`). If the condition is symbolic, it retains the previous behavior of exploring both paths for whole-program analysis.

This combined fix ensures that conditions like `fn != nil` are correctly evaluated to `false`, the `if` body is correctly skipped, and the evaluator no longer attempts to call a `nil` function. This was verified by adding a new unit test (`TestNilFunctionComparison`) and by confirming that the `make -C examples/find-orphans` command with `--include-tests` now completes without this error.