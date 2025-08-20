# Trouble: `docgen` Schema Generation Issues

This document details issues related to OpenAPI schema generation in the `docgen` tool.

---

## Part 1: `docgen` Fails to Generate Response Schemas for Slices (Fixed)

This section details the root cause of a regression where the `docgen` tool fails to generate OpenAPI `responses` for handlers that return slice types. **This issue is now considered fixed.**

### Symptom

The `docgen` integration test began to fail, with the `responses` field for slice-returning endpoints being `nil`.

For example, a handler like this one no longer had its response schema generated:

```go
// listUsers handles the GET /users endpoint.
func listUsers(w http.ResponseWriter, r *http.Request) {
	users := []User{...}
	_ = json.NewEncoder(w).Encode(users)
}
```

### Root Cause Analysis

The issue stemmed from a loss of type information within the `symgo` evaluator when handling composite literals for slices (e.g., `[]User{...}`). The evaluator's `evalCompositeLit` function was incorrectly resolving the type of a slice literal to its element type (`User`), losing the "slice-ness" of the value.

### Resolution

The issue was resolved by implementing the following tasks:

-   [x] **Task 1: Enhance `symgo` Object Model**: Introduced a new `object.Slice` type in `symgo/object/object.go` to hold the `*scanner.FieldType` of the slice, accurately representing its structure.

-   [x] **Task 2: Refactor `evalCompositeLit`**: Modified `symgo/evaluator/evaluator.go` so that `evalCompositeLit` creates and returns an `object.Slice` for slice literals, preserving the full type information.

-   [x] **Task 3: Update `Encode` Intrinsic**: Updated the `(*encoding/json.Encoder).Encode` intrinsic pattern in `examples/docgen/patterns/patterns.go` to handle the new `object.Slice` type and pass the correct `FieldType` to the schema builder.

---

## Part 2: `docgen` Fails to Generate Response Schemas for Non-Slice Structs (New Regression)

This section details a new regression discovered while fixing the slice issue. The tool now fails to generate OpenAPI `responses` for handlers that return single struct instances.

### Symptom

While the fix for slice responses was successful, it broke the analysis of handlers returning single struct instances, specifically when the variable is declared with `var` rather than `:=`.

The following handlers in `sampleapi` now fail to have their response schemas generated:
- `getUser()`
- `createUser()`

```go
// createUser handles the POST /users endpoint.
func createUser(w http.ResponseWriter, r *http.Request) {
	var user User // The variable is declared here.
	_ = json.NewDecoder(r.Body).Decode(&user)
	user.ID = 3
	// The type of `user` seems to be lost when it reaches the `Encode` intrinsic.
	_ = json.NewEncoder(w).Encode(user)
}
```

The `TestDocgen` integration test now has the response assertions for these handlers commented out to allow the main refactoring work to be merged.

### Root Cause Hypothesis

The issue appears to be in the `symgo` evaluator's type information propagation logic.

1.  When a variable is declared with `var user User`, the `evalGenDecl` function correctly creates an `*object.Variable` with the `ResolvedTypeInfo` set to `User`. The `Value` of this variable is a `*object.SymbolicPlaceholder`.
2.  When this variable `user` is later used in `json.NewEncoder(...).Encode(user)`, the `evalIdent` function is supposed to unwrap the variable and propagate the `TypeInfo` from the `Variable` container to its `Value` (the `SymbolicPlaceholder`).
3.  For some reason, this `TypeInfo` appears to be `nil` by the time it reaches the `EncoderEncodePattern` intrinsic. The `arg.TypeInfo()` call returns `nil`, and no schema is generated.

The exact point of failure in the propagation logic has not been identified, despite several attempts.

### Proposed Tasks for Resolution

- [ ] **Task 1: Debug TypeInfo Propagation**
    -   Add more detailed logging to the `symgo` evaluator, specifically tracking the `ResolvedTypeInfo` of `Variable` and `SymbolicPlaceholder` objects through the `evalAssignStmt`, `evalGenDecl`, and `evalIdent` functions.
    -   Step through the evaluation of the `createUser` handler to pinpoint where the `TypeInfo` is lost or becomes `nil`.

- [ ] **Task 2: Fix the Evaluator**
    -   Based on the debugging, implement a fix in the evaluator to ensure type information is correctly maintained and propagated from a variable's declaration to its use as an intrinsic argument.

- [ ] **Task 3: Re-enable Tests**
    -   Uncomment the response assertions in `examples/docgen/main_test.go` and verify that all tests pass.

---

## Part 3: `symgo` Fails to Resolve Methods on External Interfaces

This section details an issue where the `symgo` evaluator correctly resolves method calls on locally-defined interfaces but fails for interfaces imported from external packages, such as `net/http.ResponseWriter`.

### Symptom

A new test case was added to `docgen` to analyze a handler wrapped in `http.TimeoutHandler`. This required the evaluator to resolve method calls on `http.ResponseWriter`, which is an interface.

The analysis fails because the intrinsic for `(net/http.ResponseWriter).Write` is not triggered. This leads to a test failure where the response for the endpoint is `nil`.

To isolate the issue, a minimal test case (`TestEval_InterfaceMethodCall`) was created in `symgo/evaluator`. This test uses a **locally-defined** interface:
```go
package main
type Writer interface {
	Write(p []byte) (n int, err error)
}
// ...
```
The evaluator **successfully** resolves `w.Write()` and calls the registered intrinsic for `(main.Writer).Write`.

This proves that the core logic for handling method calls on interface-typed variables is working, but it fails specifically for external interfaces like `http.ResponseWriter`.

### Root Cause Analysis

The root cause is not yet fully understood, but the evidence points to a subtle difference in how `go-scan` resolves `TypeInfo` for local versus external packages, or how `symgo` interprets that `TypeInfo`.

- The fix in `extendFunctionEnv` correctly creates a `*object.Variable` for function parameters, associating the argument value with the parameter's type.
- Debugging logs show that when `evalSelectorExpr` evaluates `w.Write`, it correctly identifies the type of `w` as `net/http.ResponseWriter` and constructs the correct lookup key: `(net/http.ResponseWriter).Write`.
- Despite the correct key being used, the intrinsic registered with that key in `docgen`'s `patterns.go` is not found or not called.

The discrepancy between the passing `symgo` test (local interface) and the failing `docgen` test (external interface) is the primary clue. The problem likely lies in the details of the `*scanner.TypeInfo` object for `http.ResponseWriter` or in the intrinsic registration/lookup mechanism when dealing with types from packages outside the immediate scan target.

### Proposed Tasks for Resolution

- [ ] **Task 1: Deeper Inspection of `TypeInfo`**: Add debug logging to print the full `*scanner.TypeInfo` struct for both the local `Writer` interface and the external `http.ResponseWriter` interface to spot any structural differences that might affect method resolution.
- [ ] **Task 2: Simplify `docgen` Test**: Temporarily modify the `docgen` test to use a local interface instead of `http.ResponseWriter` to confirm that the `docgen` setup works for local interfaces, further isolating the problem to external package handling.
- [ ] **Task 3: Fix `symgo` or `docgen`**: Based on the findings, implement a fix. The fix might be in `symgo/evaluator`'s `evalSelectorExpr` if it handles external `TypeInfo` incorrectly, or in `docgen`'s `Analyzer` if it needs to prepare or register intrinsics for external types in a different way.
