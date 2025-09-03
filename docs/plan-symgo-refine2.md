# `symgo` Refinement Plan 2: Addressing Infinite Recursion and Timeout

This document outlines a new plan to improve the `symgo` symbolic execution engine. It is based on a re-run of the `find-orphans` end-to-end test, as specified in `docs/plan-symgo-refine.md`. The analysis revealed a critical timeout issue caused by infinite recursion, as well as other significant errors that need to be addressed.

## Summary of Findings

The `find-orphans` e2e test was executed with a 30-second timeout. The process was terminated by the timeout, confirming the user's suspicion that the analysis was hanging. The generated log file, `find-orphans.out`, was incomplete but contained enough information to diagnose several critical issues. The previous fixes have resolved some problems, but new, more severe issues have been uncovered.

## Error and Warning Analysis

### 1. Critical: Infinite Recursion and Timeout

- **Log Messages**:
    - `level=WARN msg="infinite recursion detected, aborting" in_func=TypeInfoFromExpr`
    - `level=ERROR msg="infinite recursion detected: TypeInfoFromExpr"`
- **Analysis**: The log is flooded with these messages, all originating from `scanner/scanner.go` in the `TypeInfoFromExpr` function. This is the most critical issue and the undeniable cause of the timeout. The scanner is entering an unbounded recursive loop while trying to resolve type information, which prevents the symbolic execution from making any meaningful progress. This must be fixed before any other analysis can be reliably performed.

### 2. Invalid Dereference of Unresolved Functions

- **Error Message**: `invalid indirect of <Unresolved Function: ...> (type *object.UnresolvedFunction)`
- **Example Context**: This error occurs for standard library types like `log/slog.Logger`, `go/token.FileSet`, and `go/ast.File`.
- **Analysis**: This is a new and severe bug. The `symgo` engine appears to be incorrectly identifying a type as an `UnresolvedFunction` object. Subsequently, when the code attempts to use this type (e.g., as a pointer to a struct), the engine tries to perform an indirect memory access (`*p`) on the function object, which is an invalid operation. This indicates a fundamental flaw in the type resolution or object representation logic for external or built-in types.

### 3. Cascading "Identifier Not Found" Errors

- **Error Messages**:
    - `identifier not found: PackageInfo`
    - `identifier not found: elemType`
    - `identifier not found: genericType`
- **Analysis**: These errors appear frequently within the same function (`TypeInfoFromExpr`) that is suffering from infinite recursion. It is highly likely that these are a direct symptom of the recursion bug. The recursive calls are likely failing to maintain or propagate the correct scope, leading to a state where expected variables are not defined.

### 4. Persistent Multi-Return Value Warnings

- **Warning Message**: `expected multi-return value on RHS of assignment`
- **Analysis**: This warning, noted in the original plan, is still present. While the previous fix may have addressed some cases, it is not comprehensive. This suggests that the symbolic representation of un-analyzable function calls is still not robust enough to handle all multi-value assignment scenarios.

## Proposed Task List for `symgo` Improvement

- [ ] **Task 1: Fix Infinite Recursion in `scanner.TypeInfoFromExpr`.**
    - **Goal**: Identify and fix the cause of the unbounded recursion in `TypeInfoFromExpr`.
    - **Details**: This requires a deep dive into the type resolution logic within the scanner. The fix will likely involve adding a mechanism to track visited nodes during the recursive traversal to prevent re-entering the same analysis loop. This is the highest priority task.
    - **Acceptance Test**: The `find-orphans` e2e test should run to completion without timing out.

- [ ] **Task 2: Correctly Resolve and Handle External Types.**
    - **Goal**: Prevent the "invalid indirect" error by ensuring external and built-in types are resolved as type objects, not function objects.
    - **Details**: Investigate how types from packages like `log/slog` are being resolved. The evaluator should create a symbolic type placeholder, not an `UnresolvedFunction` object. This may require changes to the `scanner` or the `symgo` evaluator's handling of package lookups.

- [ ] **Task 3: Add a Debug Timeout Option to `find-orphans`.**
    - **Goal**: Make the tool easier to debug by adding a CLI flag for a timeout.
    - **Details**: The user suggested a 30s timeout is sufficient. Implement a `--timeout` flag (e.g., `--timeout 30s`) in `find-orphans` that uses a `context.WithTimeout` to gracefully terminate the analysis. This will make debugging long-running or hanging analyses much more manageable.

- [ ] **Task 4: Re-evaluate Entry Point Analysis.**
    - **Goal**: Once the critical bugs are fixed, re-run the e2e test and address any remaining errors that cause the analysis of `main` entry points to fail.
    - **Details**: This task is carried over from the previous plan. With the timeout and recursion issues resolved, it will be possible to get a complete and accurate log, which can be used to identify and fix any further bugs preventing a full analysis.

## Reproduction Steps

The reproduction steps remain the same.

1.  **Ensure Makefile exists:** The file `examples/find-orphans/Makefile` should contain:
    ```makefile
    e2e:
	@echo "Running end-to-end test for find-orphans..."
	go run . --workspace-root ../.. ./... > find-orphans.out 2>&1
	@echo "Output written to examples/find-orphans/find-orphans.out"
    ```

2.  **Run the analysis:**
    Execute the make target from the repository root. A `timeout` is recommended to prevent a hung process.
    ```sh
    timeout 60s make -C examples/find-orphans e2e
    ```

3.  **Inspect the output:**
    The results, including logs, will be in `examples/find-orphans/find-orphans.out`.
