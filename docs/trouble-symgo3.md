# Trouble: `symgo` Fails to Resolve Local Type Definitions

-   **Last Updated:** 2025-09-18
-   **Status:** In Progress

## Summary

When running the `find-orphans` tool in library mode (`--mode lib`), the analysis fails with an `identifier not found` error. The error occurs when the `symgo` symbolic execution engine attempts to analyze a function that contains a local type definition (e.g., `type Alias MyType`). The engine does not correctly register this local type in its environment for the function's scope, leading to a failure when the type alias is subsequently used.

## The Problem

The error was identified when analyzing the `UnmarshalJSON` methods in `examples/derivingjson/integrationtest/integrationtest_deriving.go`. These methods use a common pattern to avoid infinite recursion:

```go
func (s *APIResponse) UnmarshalJSON(data []byte) error {
	// Define an alias type to prevent infinite recursion with UnmarshalJSON.
	type Alias APIResponse // This is the local type definition
	aux := &struct {
		// ...
		*Alias // The use of 'Alias' here fails
	}{
		Alias: (*Alias)(s), // And the use here also fails
	}
    // ...
}
```

The `symgo` evaluator produced the following error log:

```
level=ERROR msg="identifier not found: Alias" in_func=UnmarshalJSON ...
```

Investigation of the evaluator's code in `symgo/evaluator/evaluator.go` revealed that the `evalGenDecl` function, which is responsible for handling declarations, only had logic for `var` declarations (`token.VAR`). It completely ignored `type` declarations (`token.TYPE`) within function bodies.

As a result, when the evaluator processed the `type Alias APIResponse` declaration, it did nothing. The `Alias` type was never added to the environment for the `UnmarshalJSON` function's scope. When the code later tried to use `Alias`, the evaluator could not find it and reported the error.

## The Solution

The fix involves modifying the `evalGenDecl` function in `symgo/evaluator/evaluator.go` to correctly handle `token.TYPE` declarations. The new logic mirrors the existing handling for top-level type declarations:

1.  When a `GenDecl` with `tok == token.TYPE` is encountered within a function body, the evaluator iterates through its `TypeSpec`s.
2.  For each `TypeSpec`, it finds the corresponding `scan.TypeInfo` that was created by the initial scanning phase. This ensures that the alias is resolved to the correct underlying type.
3.  An `object.Type` is created from the `TypeInfo`.
4.  This new type object is registered in the **current environment**, making it available for the rest of the function's scope.

This ensures that local type aliases are correctly resolved, allowing the symbolic execution to proceed without error. A regression test was added to specifically cover local type definitions and prevent future breakages.

---

## Update: Failed Implementation Attempt

An attempt was made to fix this issue by modifying `evalGenDecl` to handle `token.TYPE` and `evalCompositeLit` to use the evaluator's environment for type resolution.

**The Approach:**

1.  **Modify `evalGenDecl`:** The function was updated to recognize `case token.TYPE` and, for each local type definition, create a new `object.Type` and register it in the current function's environment.
2.  **Modify `evalCompositeLit`:** The function was changed to first call `e.Eval()` on the type expression of a composite literal. The idea was that this would resolve local types from the environment. If `e.Eval()` did not return a valid type object, the logic would fall back to the original method of using `e.scanner.TypeInfoFromExpr()`, which resolves top-level types.

**The Failure:**

This approach introduced a significant number of regressions and a panic.

-   **Regressions:** Nearly all tests involving the resolution of top-level struct types began failing with `identifier not found` errors. The root cause was a bug in the fallback logic. When `e.Eval()` was called on a top-level type identifier (e.g., `S` in `S{}`), it correctly failed to find it in the *current* (local) environment and returned an error object. However, the initial implementation of the new logic immediately propagated this error instead of catching it and proceeding to the fallback (the scanner-based lookup).
-   **Panic:** A panic (`nil pointer dereference`) occurred in `TestEval_FunctionInCompositeLiteral`. This was also traced back to the faulty logic in `evalCompositeLit`, where an error was not handled correctly, leading to subsequent operations on a `nil` value.

**Conclusion:**

While the core idea of handling `token.TYPE` in `evalGenDecl` is correct, the modification to `evalCompositeLit` was flawed. A correct implementation must handle the `e.Eval()` call's failure gracefully and ensure the fallback to the scanner-based lookup for top-level types is always executed when a type is not found in the local environment. The repeated failure to apply a correct patch for this indicates the complexity of the change and the need for a more careful approach, which is documented in `docs/cont-symgo-local-alias.md`.
