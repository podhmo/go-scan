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
    - **Goal**: Correctly model multi-return values from un-analyzable functions.
    - **Details**: When a call to an external or un-analyzable function is made, `symgo` should inspect the function's signature. If it returns multiple values, the evaluator should produce an `object.Tuple` containing the correct number of `SymbolicPlaceholder` objects. This will allow destructuring assignments (`v, err := ...`) to work correctly.

- [ ] **Task 4: Investigate and Refine Entry Point Analysis.**
    - **Goal**: Ensure that the analysis of `main` functions in the various example tools does not fail due to the above issues.
    - **Details**: The `find-orphans.out` log shows that analysis fails for almost every `main` function. Once the above tasks are complete, re-run the `e2e` test and address any remaining errors that cause the analysis of these primary entry points to fail. This will serve as a good acceptance test for the fixes.

---

## Reproduction Steps

1.  **Create a Makefile:**
    Create a file named `Makefile` in the `tools/find-orphans/` directory with the following content:
    ```makefile
    e2e:
	@echo "Running end-to-end test for find-orphans..."
	go run . --workspace-root ../.. ./... > find-orphans.out 2>&1
	@echo "Output written to tools/find-orphans/find-orphans.out"
    ```

2.  **Run the analysis:**
    Execute the make target from the repository root:
    ```sh
    make -C tools/find-orphans e2e
    ```

3.  **Inspect the output:**
    The results, including all logs and the final list of orphans, will be in the `tools/find-orphans/find-orphans.out` file.

## Appendix: Raw Log Snippets

<details>
<summary>Click to expand a snippet of the logs from find-orphans.out</summary>

```text
time=2025-09-03T02:30:22.458Z level=INFO msg="found main entry point(s), running in application mode" count=9

# Example of "invalid indirect" error
time=2025-09-03T02:30:22.458Z level=ERROR msg="invalid indirect of <Symbolic: result of external call to String> (type *object.SymbolicPlaceholder)" in_func=main in_func_pos=/app/examples/convert-define/main.go:48:1 pos=244709
time=2025-09-03T02:30:22.458Z level=ERROR msg="symbolic execution failed for entry point" function=github.com/podhmo/go-scan/examples/convert-define.main error="symgo runtime error: invalid indirect of <Symbolic: result of external call to String> (type *object.SymbolicPlaceholder)\n\t/app/examples/convert-define/main.go:63:5:\n\t\tif *defineFile == \"\" {\n\t/app/examples/convert-define/main.go:48:1:\tin main\n\t\tfunc main() {\n"

# Example of "could not scan package" and related warnings/errors
time=2025-09-03T02:30:22.480Z level=WARN msg="could not scan package, treating as external" in_func=discoverModules in_func_pos=/app/tools/find-orphans/main.go:80:1 package=golang.org/x/mod/modfile error="ScanPackageFromImportPath: could not find a module responsible for import path \"golang.org/x/mod/modfile\" in workspace"
time=2025-09-03T02:30:22.481Z level=WARN msg="expected multi-return value on RHS of assignment" in_func=discoverModules in_func_pos=/app/tools/find-orphans/main.go:80:1 got_type=SYMBOLIC_PLACEHOLDER
time=2025-09-03T02:30:22.481Z level=ERROR msg="unsupported assignment target: expected an identifier or selector, but got *ast.IndexExpr" in_func=discoverModules in_func_pos=/app/tools/find-orphans/main.go:80:1 pos=802529

# Example of "unsupported assignment target" leading to a failed entry point
time=2025-09-03T02:30:22.753Z level=ERROR msg="symbolic execution failed for entry point" function=github.com/podhmo/go-scan/examples/deps-walk.main error="symgo runtime error: unsupported assignment target: expected an identifier or selector, but got *ast.IndexExpr\n\t/app/tools/find-orphans/main.go:113:3:\n\t\texcludeMap[dir] = true\n\t/app/tools/find-orphans/main.go:80:1:\tin discoverModules\n\t\tfunc discoverModules(ctx context.Context, root string, excludeDirs []string) ([]string, error) {\n\t/app/tools/find-orphans/main.go:139:1:\tin run\n\t\tfunc run(ctx context.Context, all bool, includeTests bool, workspace string, verbose bool, asJSON bool, mode string, startPatterns []string, excludeDirs []string, scanPolicy symgo.ScanPolicyFunc, primaryAnalysisScope []string) error {\n\t/app/examples/deps-walk/main.go:49:1:\tin main\n\t\tfunc main() {\n"

time=2025-09-03T02:30:22.913Z level=INFO msg="symbolic execution complete"

-- Orphans --
github.com/podhmo/go-scan/examples/derivingbind/parser.String
  /app/examples/derivingbind/parser/parsers.go:11:1
...
```
</details>

---

## Appendix: Q&A on `unresolvedFunction` Implementation

This section clarifies the scope and behavior of the changes related to handling unresolved function calls.

#### Q1: How are functions that return `(T, bool)` interpreted by this change?

The implementation is generic and not specific to `(T, error)` returns. The core logic iterates through all return values defined in a function's signature. For a function returning `(MyType, bool)`, the system will correctly produce two symbolic placeholders, one with the type information for `MyType` and the other with the type information for the built-in `bool`. The special handling added was only to ensure the built-in `error` type was correctly resolved to its full interface definition; other built-in types like `bool` and standard types are handled by the general logic.

#### Q2: How are functions that return multiple values, including a cleanup function (e.g., `func()`), interpreted?

This scenario is also handled correctly. The logic is the same as above. If a function returns `(int, func())`, the system will generate two symbolic placeholders. The first will be typed as `int`, and the second will be correctly typed as a function (`*scanner.FieldType{ IsFunc: true, ... }`). This ensures that the type information for all return values, regardless of their kind (basic type, struct, interface, function, etc.), is preserved.

#### Q3: What is the precise scope of the `unresolvedFunction` change?

The new `object.UnresolvedFunction` logic is triggered whenever `symgo`'s `evalSelectorExpr` (which handles expressions like `pkg.Func`) fails to find the definition for `Func`. This can happen for two main reasons:
1.  The entire package `pkg` could not be scanned (e.g., it's not in the workspace, or a disk error occurred).
2.  The package `pkg` was scanned, but the specific function `Func` was not found in its list of declarations (e.g., it might be excluded by build tags).

In both cases, instead of returning a generic placeholder, the evaluator now returns a special `object.UnresolvedFunction` that remembers the package path and function name. When this object is later invoked, the `applyFunction` logic performs a "lazy" or "just-in-time" resolution: it re-attempts to scan the package and find the function's signature. If found, it generates the correct symbolic return values. This makes the analysis more robust against missing initial information.

#### Q4: Is the "Enhance Symbolic Function Return Values" task truly complete?

Yes. The original problem was that unresolved functions defaulted to a single, untyped return value, breaking the analysis of multi-value assignments. The implemented changes directly fix this by introducing a lazy resolution mechanism that correctly determines the number and type of return values at the call site. The related bug concerning the resolution of the `error` type was also fixed. The new test cases validate this specific functionality, confirming that the task's goal has been met.

#### Q5: Why does the `error` type need special handling in `builtins.go` while types like `bool` or `func()` do not?

This is because `error` is unique among Go's built-in types: it is the only one that is an **interface with methods**.

-   **`bool` and `int`** are primitive types. The symbolic engine only needs to know their name to treat them correctly.
-   **`func()`** is a structural type. The engine can understand its structure (parameters, return values) directly from the Go AST.
-   **`error`**, however, is defined as `type error interface { Error() string }`. For the symbolic engine to perform more advanced analysis (like type assertions or checking if a type satisfies the `error` interface), it needs to know not just the name "error", but also its method set—specifically, that it has a method `Error()` that returns a `string`.

The `go-scan` library does not automatically provide this detailed structural information for built-in types from the `universe` scope. It just knows their names. The test failures showed that when the evaluator created a symbolic placeholder for an `error`, the detailed `TypeInfo` for its interface structure was missing.

The solution was to create `symgo/evaluator/builtins.go` to manually construct a complete `scanner.TypeInfo` for the `error` interface. This hand-crafted `TypeInfo` is then used whenever the evaluator needs to represent the `error` type, ensuring the engine always has access to its full definition. This special handling is necessary to give the `error` type first-class status in the symbolic analysis, on par with user-defined interfaces.
