# Trouble: `docgen` Fails to Detect Responses After `symgo` Update

- **Date**: 2025-08-29
- **Status**: Identified

## Symptom

After a change in the `symgo` evaluator to correctly process `if` statement conditions (`if n.Cond`), the `docgen` example tool regressed in its functionality.

Specifically, for handlers that contain `if` statements to check for query parameters and return early, the generated OpenAPI specification is missing the `responses` section entirely. It correctly identifies the new `parameters` from the `if` condition, but fails to analyze the rest of the function to find the responses.

## Root Cause Analysis

The root cause is in how the `symgo` evaluator's `evalIfStmt` function handles control flow, and how the `docgen` analyzer consumes its results.

1.  **Previous Behavior**: Before the fix, `evalIfStmt` did *not* evaluate the `Cond` expression. The `docgen` analyzer would proceed to analyze the function body, finding `return` statements or calls to helper functions like `helpers.RenderJSON` or `helpers.RenderError`, and correctly generating the `responses` section.
2.  **Current Behavior**: After the fix, `evalIfStmt` now evaluates `n.Cond`. It then proceeds to evaluate both the `then` (`Body`) and `else` (`Else`) branches in separate, enclosed environments. Crucially, the function returns a generic `*object.SymbolicPlaceholder` after evaluating the branches.

It appears the `docgen` analyzer's logic, which walks the AST using the `symgo` interpreter, does not correctly handle the state after `evalIfStmt` returns. It seems to stop or lose the necessary context after the `if` block, preventing it from discovering the `return` statements that define the HTTP responses.

For example, in `GetUserHandler` from `examples/docgen/testdata/new-features/main/api.go`:

```go
func GetUserHandler(w http.ResponseWriter, r *http.Request) {
	// This branch is now correctly analyzed for the "error" query parameter.
	if r.URL.Query().Get("error") == "true" {
		helpers.RenderError(w, r, http.StatusNotFound, &helpers.ErrorResponse{Error: "User not found"})
		return
	}

	// The analyzer no longer reaches this part of the code.
	user := User{ID: "123", Name: "John Doe"}
	helpers.RenderJSON(w, http.StatusOK, user)
}
```

The symbolic execution seems to "end" after the `if` block, so `helpers.RenderJSON` is never seen. The analyzer correctly finds the `error` query parameter in the `if` condition, but then fails to analyze the happy-path response.

## Resolution Plan

The `symgo` evaluator needs to be fixed to correctly model the control flow of an `if` statement without losing the execution path. Both branches (`then` and `else`) should be symbolically executed, and the state from both paths should be managed in a way that allows analysis to continue after the `if` block.

A potential fix might involve `evalIfStmt` returning a special object that represents the merged state of both branches, or ensuring that the environment after the `if` statement reflects the outcomes of both possible paths. The current implementation, which returns a simple placeholder, is insufficient for tools like `docgen` that need to understand the complete control flow.
