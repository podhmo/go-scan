# Trouble: `docgen` Fails to Generate Response Schemas for Slices

This document details the root cause of a regression where the `docgen` tool fails to generate OpenAPI `responses` for handlers that return slice types.

## Symptom

After a series of refactorings to the `symgo` evaluator to support query parameter analysis, the `docgen` integration test began to fail. While query parameters and request bodies are correctly identified, the `responses` field for all endpoints in the generated OpenAPI specification is `nil`.

For example, a handler like this one no longer has its response schema generated:

```go
// listUsers handles the GET /users endpoint.
func listUsers(w http.ResponseWriter, r *http.Request) {
	users := []User{
		{ID: 1, Name: "Alice"},
		{ID: 2, Name: "Bob"},
	}
	w.Header().Set("Content-Type", "application/json")
	// The type of `users` is not correctly inferred here.
	_ = json.NewEncoder(w).Encode(users)
}
```

## Root Cause Analysis

The issue stems from a loss of type information within the `symgo` evaluator when handling composite literals for slices (e.g., `[]User{...}`).

1.  **`evalCompositeLit` Misinterpretation**: When the evaluator's `evalCompositeLit` function encounters an `*ast.ArrayType` (a slice literal), it correctly identifies the `FieldType` for `[]User`. However, to create a symbolic object, it then calls `fieldType.Resolve()`. The `Resolve()` method is designed to find the core definition of a type, so for `[]User`, it returns the `TypeInfo` for the element, `User`.

2.  **Loss of "Slice-ness"**: The evaluator then creates a generic `*object.Instance` and attaches the `TypeInfo` for `User`. At this moment, the crucial information that the original type was a slice is lost. The resulting symbolic object represents `User`, not `[]User`.

3.  **Incorrect Intrinsic Argument**: This incorrect object is assigned to the `users` variable. When `json.NewEncoder(w).Encode(users)` is called, the `Encode` intrinsic receives an `*object.Instance` that claims to be a `User`.

4.  **Schema Builder Failure**: The `Encode` intrinsic passes this `TypeInfo` for `User` to the `buildSchemaForType` function. The schema builder correctly generates a schema for a single `User` object. However, the test expects a schema for an *array* of `User` objects, leading to a test failure. My attempt to fix this in `schema.go` by checking `typeInfo.Underlying` was ineffective because the `TypeInfo` being passed was for a `struct`, which has no `Underlying` field.

The core of the problem is that the `symgo` object model, specifically `object.Instance`, is not expressive enough to distinguish between a value of type `T` and a value of type `[]T`.

## Proposed Tasks for Resolution

To properly fix this, several parts of the `symgo` engine need to be enhanced.

-   [ ] **Task 1: Enhance `symgo` Object Model**
    -   Introduce a new `object.Object` type, such as `object.Slice`, in `symgo/object/object.go`.
    -   This new type should be able to hold the `*scanner.FieldType` of the slice, which accurately represents its structure (e.g., `IsSlice: true` and `Elem: <FieldType for User>`).

-   [ ] **Task 2: Refactor `evalCompositeLit`**
    -   Modify `symgo/evaluator/evaluator.go` so that when `evalCompositeLit` encounters an `*ast.ArrayType`, it creates and returns an instance of the new `object.Slice` type, populated with the correct `FieldType`. It should no longer call `Resolve()` on the `FieldType`.

-   [ ] **Task 3: Update `Encode` Intrinsic and Schema Builder**
    -   Update the `(*encoding/json.Encoder).Encode` intrinsic in `docgen/analyzer.go` to handle the new `object.Slice` type.
    -   When it receives an `object.Slice`, it should extract the `FieldType` and pass that to the schema generation logic. The `buildSchemaFromFieldType` function is already equipped to handle slice `FieldType`s, so it should work correctly from there.
