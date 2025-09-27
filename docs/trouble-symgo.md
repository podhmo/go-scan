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