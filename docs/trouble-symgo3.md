# Trouble: `symgo` Fails to Resolve Local Type Definitions

-   **Last Updated:** 2025-09-18
-   **Status:** Resolved (Fix in progress)

## Summary

When running the `find-orphans` tool in library mode (`--mode lib`), the analysis can fail with an `identifier not found` error. The error occurs when the `symgo` symbolic execution engine attempts to analyze a function that contains a local type definition (e.g., `type Alias MyType`).

## The Problem

The error was identified when analyzing code that uses a common pattern to avoid `json.Unmarshal` recursion:

```go
func (s *APIResponse) UnmarshalJSON(data []byte) error {
	type Alias APIResponse // This is the local type definition
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(s),
	}
    // ...
}
```

The core issue was a two-part bug spanning both the `scanner` and the `evaluator`.

1.  **Scanner Bug:** The scanner did not inspect the bodies of functions for declarations. It only processed top-level declarations. As a result, the `TypeInfo` for `Alias` was never created, and the evaluator had no information about it.

2.  **Evaluator Bug:** Even if the scanner *did* provide the information, the `evalCompositeLit` function had no logic to look up types in the current function's local environment. It exclusively used the global package scanner, which would never find a locally-scoped type like `Alias`.

## The Solution

The fix required addressing both bugs in sequence.

### Part 1: Fixing the Scanner (Completed)

The `scanner/scanner.go` file was modified to correctly parse local types.

-   **`parseFuncDecl` was updated** to use `ast.Inspect` to walk the body of every function it parses.
-   Inside the walk, it now **detects `ast.GenDecl` nodes with `token.TYPE`**.
-   When a local type is found, it's processed using the existing `parseTypeSpec` logic, and the resulting `TypeInfo` is added to the `PackageInfo.Types` slice.
-   A crucial addition was made: after parsing a local alias, the scanner **immediately attempts to link the alias's underlying type to its definition** within the same package by looking it up in the `PackageInfo`'s types and setting the `Underlying.Definition` field.

This ensures that the scanner produces a complete and correct `PackageInfo` where local aliases are not only present but also correctly linked to the `TypeInfo` of their concrete underlying types. This was verified with targeted unit tests.

### Part 2: Fixing the Evaluator (In Progress)

With the scanner providing the correct data, the `evaluator` can now be fixed. The `evalCompositeLit` function in `symgo/evaluator/evaluator.go` must be modified to use this data.

-   **The logic will be changed to perform a dual lookup:**
    1.  First, it will call `e.Eval()` on the composite literal's type expression (e.g., `Alias` in `Alias{}`). This uses the evaluator's environment stack and will find the `object.Type` for the local alias.
    2.  If found, it will use the `ResolvedType.Underlying.Definition` field (which is now correctly populated by the fixed scanner) to get the `TypeInfo` for the concrete type.
    3.  If `e.Eval()` fails with an "identifier not found" error, it means the type is not local. The function will then **fall back** to its original behavior of using `e.scanner.TypeInfoFromExpr()` to resolve it as a package-level type.

This two-step process (fix scanner, then fix evaluator) correctly addresses the root cause and will allow `symgo` to handle local type aliases.
