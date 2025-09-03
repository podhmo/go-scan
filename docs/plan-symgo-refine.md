# `symgo` Refinement Plan based on E2E Analysis

This document outlines a plan to improve the `symgo` symbolic execution engine based on the findings from running the `find-orphans` tool on its own repository. The analysis revealed several limitations and potential bugs in `symgo`.

## Summary of Findings

An end-to-end test of `find-orphans` was performed by running it across the entire `go-scan` workspace. While the tool completed its run and produced a list of potential orphans, the log output (`find-orphans.out`) was populated with a significant number of warnings and errors. These issues indicate that the symbolic execution is failing for many of the tool's main entry points, which means the analysis is incomplete and the results are likely unreliable (containing both false positives and false negatives).

The logged errors can be grouped into several categories, revealing core limitations in the `symgo` evaluator.

## Error and Warning Analysis

### 1. Unsupported Assignment to Index Expressions

- **Error Message**: `unsupported assignment target: expected an identifier or selector, but got *ast.IndexExpr`
- **Example Code**: `excludeMap[dir] = true`
- **Analysis**: The `symgo` evaluator currently cannot handle assignment statements where the left-hand side is a map or slice index expression (e.g., `m[k] = v`, `s[i] = v`). This is a major feature gap, as it's a very common pattern in Go. When the evaluator encounters this, it stops the analysis for that function path, leading to incomplete call graph traversal.

### 2. Pointer Dereference on Symbolic Placeholders

- **Error Message**: `invalid indirect of ... (type *object.SymbolicPlaceholder)`
- **Example Context**: This error occurs frequently when `symgo` encounters a pointer dereference (`*p`) where `p` is a variable it could not resolve to a concrete value. This often happens with function parameters in entry points or with variables returned from functions that cannot be analyzed (e.g., from external packages).
- **Analysis**: `symgo` lacks a robust mechanism for handling operations on symbolic pointers. When it doesn't know the concrete object a pointer refers to, it should ideally be able to proceed symbolically (e.g., by creating a new symbolic value to represent the result of the dereference). Instead, it halts with an "invalid indirect" error.

### 3. Incorrect Handling of Multi-Return Functions

- **Warning Message**: `expected multi-return value on RHS of assignment`
- **Example Context**: This occurs during a destructuring assignment like `val, err := someFunc()` when `someFunc` could not be fully analyzed.
- **Analysis**: When `symgo` cannot analyze a function call, it returns a single `SymbolicPlaceholder`. It does not correctly model that the function was expected to return multiple values. The correct behavior would be to return a tuple of `SymbolicPlaceholder` objects, allowing the destructuring assignment to proceed. This is closely related to the issue of handling external packages.

### 4. Incomplete Analysis of External Packages

- **Warning Message**: `could not scan package, treating as external`
- **Analysis**: This warning itself is not a bug; it correctly identifies a package that is outside the analysis scope. However, it often precedes the "expected multi-return" warning. This indicates that `symgo`'s strategy for handling unscannable code is not robust. While it correctly creates a placeholder for the function call, the placeholder is not a sufficiently accurate representation (as seen in point 3).

## Proposed Task List for `symgo` Improvement

To make `symgo` a more robust and reliable engine for tools like `find-orphans`, the following tasks should be prioritized.

- [ ] **Task 1: Implement Map and Slice Assignments.**
    - **Goal**: Add support for `*ast.IndexExpr` on the left-hand side of `*ast.AssignStmt`.
    - **Details**: The evaluator needs to be able to handle `m[k] = v` and `s[i] = v`. For maps, this involves updating the symbolic map object. For slices, it involves updating the element at the given index.

- [ ] **Task 2: Improve Symbolic Pointer Handling.**
    - **Goal**: Prevent "invalid indirect" errors by allowing dereferencing of symbolic pointers.
    - **Details**: When evaluating `*p` where `p` is a symbolic pointer, the evaluator should not error. Instead, it should return a new `SymbolicPlaceholder` representing the value pointed to. This new placeholder should be associated with the pointer's type, allowing subsequent field or method access to be resolved symbolically.

- [ ] **Task 3: Enhance Symbolic Function Return Values.**
    - **Goal**: Correctly model multi-return values from un-analyzed functions.
    - **Details**: When a call to an external or un-analyzable function is made, `symgo` should inspect the function's signature. If it returns multiple values, the evaluator should produce an `object.Tuple` containing the correct number of `SymbolicPlaceholder` objects. This will allow destructuring assignments (`v, err := ...`) to work correctly.

- [ ] **Task 4: Investigate and Refine Entry Point Analysis.**
    - **Goal**: Ensure that the analysis of `main` functions in the various example tools does not fail due to the above issues.
    - **Details**: The `find-orphans.out` log shows that analysis fails for almost every `main` function. Once the above tasks are complete, re-run the `e2e` test and address any remaining errors that cause the analysis of these primary entry points to fail. This will serve as a good acceptance test for the fixes.
