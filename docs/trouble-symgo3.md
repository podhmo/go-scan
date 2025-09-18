# Trouble: `symgo` Fails to Resolve Local Type Definitions

-   **Last Updated:** 2025-09-18
-   **Status:** Investigation Halted

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

Initial investigation of the evaluator's code in `symgo/evaluator/evaluator.go` revealed that the `evalGenDecl` function, which is responsible for handling declarations, only had logic for `var` declarations (`token.VAR`). It completely ignored `type` declarations (`token.TYPE`) within function bodies.

## Implementation Attempts and Analysis

An attempt was made to fix this issue by modifying `evalGenDecl` to handle `token.TYPE` and `evalCompositeLit` to use the evaluator's local environment for type resolution. This approach has so far been unsuccessful, but has narrowed down the problem considerably.

### Step 1: Modify `evalGenDecl` (Success)

The `evalGenDecl` function was successfully modified to recognize `case token.TYPE` within a function's scope. This change correctly identifies local type specifications and adds a corresponding `object.Type` to the current environment. This part of the fix is considered correct and is a necessary first step.

### Step 2: Modify `evalCompositeLit` (Multiple Failures)

The core difficulty lies in `evalCompositeLit`. When this function encounters a composite literal using a local alias (e.g., `Alias{}`), it must resolve `Alias` to its underlying concrete type (e.g., `S`). Several attempts were made to achieve this.

#### Attempt 1: Naive Local Resolution + Fallback

-   **Approach:** Modify `evalCompositeLit` to first call `e.Eval()` to look up the type in the local environment. If this fails, fall back to the global `e.scanner`.
-   **Result:** This caused widespread regressions. The fallback logic was flawed, causing lookups for all global types to fail. It also introduced a panic due to incorrect error handling.

#### Attempt 2: Fixing the Regressions

-   **Approach:** The regressions from Attempt 1 were fixed by correcting the fallback logic and adding nil checks to prevent the panic.
-   **Result:** This was a major step forward. All tests began passing *except* for the target test, `TestEval_LocalTypeDefinition`. This isolated the bug perfectly. The test now failed with `expected pointer to point to an instance, but got *object.SymbolicPlaceholder`. This indicates the alias `Alias` was being resolved, but its underlying type `S` was not being resolved to a concrete `TypeInfo`, resulting in a placeholder.

#### Attempt 3: Aiding the Resolver

-   **Hypothesis:** The `Underlying` `FieldType` for `S` was missing context (like `CurrentPkg`) needed by the resolver.
-   **Approach:** Manually add context to the `FieldType` before calling `e.resolver.ResolveType()`.
-   **Result:** Failure. The test still produced a `SymbolicPlaceholder`.

#### Attempt 4: Using Pre-Resolved Definitions

-   **Hypothesis:** The scanner might have already linked the underlying type's `TypeInfo` in the `FieldType.Definition` field.
-   **Approach:** Modify the logic to use `fieldType.Definition` directly, bypassing the resolver.
-   **Result:** Failure. The test still produced a `SymbolicPlaceholder`, implying `Definition` was `nil`.

### Current Status and Root Cause Analysis

-   **The bug is isolated:** The problem is now confirmed to be solely within `evalCompositeLit`'s handling of the `Underlying` `FieldType` of a local alias.
-   **The root cause:** When `evalCompositeLit` gets the `FieldType` for the underlying type `S`, that `FieldType` is "disconnected" from the `TypeInfo` for `S` that exists within the `PackageInfo`'s `Types` list. The `resolver.ResolveType()` call fails because it cannot bridge this gap based on the information in the `FieldType` alone.
-   **Reading the scanner code (`scanner/scanner.go`) confirms** that the `FieldType.Definition` field is not populated by the scanner during the initial parse; it is intended to be populated on-demand by the `Resolve()` method. The failure of the `Resolve()` method is therefore the key issue.

### Conclusion and Next Steps

The investigation has hit a wall. The fundamental mechanism for resolving an alias's `Underlying` `FieldType` to its `TypeInfo` within the evaluator is not understood. Further attempts to patch `evalCompositeLit` without this knowledge will likely be ineffective.

The next logical step is not to attempt another fix, but to **investigate the construction of the `PackageInfo` object in the scanner**. A deeper understanding of how `pkg.Types` is populated and how the `Lookup` method is intended to work is required to understand why the `resolver` is failing and to devise a correct solution.
