# Trouble: `docgen` Fails to Generate Response Schemas for Non-Slice Structs

This document details a regression where the `docgen` tool fails to generate OpenAPI `responses` for handlers that return single struct instances. This issue was discovered while fixing a related bug with slice responses ([docs/trouble-docgen.md](./trouble-docgen.md)).

## Symptom

While the fix for slice responses was successful, it appears to have broken the analysis of handlers returning single struct instances, specifically when the variable is declared with `var` rather than `:=`.

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

## Root Cause Hypothesis

The issue appears to be in the `symgo` evaluator's type information propagation logic.

1.  When a variable is declared with `var user User`, the `evalGenDecl` function correctly creates an `*object.Variable` with the `ResolvedTypeInfo` set to `User`. The `Value` of this variable is a `*object.SymbolicPlaceholder`.
2.  When this variable `user` is later used in `json.NewEncoder(...).Encode(user)`, the `evalIdent` function is supposed to unwrap the variable and propagate the `TypeInfo` from the `Variable` container to its `Value` (the `SymbolicPlaceholder`).
3.  For some reason, this `TypeInfo` appears to be `nil` by the time it reaches the `EncoderEncodePattern` intrinsic. The `arg.TypeInfo()` call returns `nil`, and no schema is generated.

The exact point of failure in the propagation logic has not been identified, despite several attempts. The logic in `evalIdent` seems correct, but the test results show otherwise, indicating a subtle bug in the evaluator's state management.

## Proposed Tasks for Resolution

- [ ] **Task 1: Debug TypeInfo Propagation**
    -   Add more detailed logging to the `symgo` evaluator, specifically tracking the `ResolvedTypeInfo` of `Variable` and `SymbolicPlaceholder` objects through the `evalAssignStmt`, `evalGenDecl`, and `evalIdent` functions.
    -   Step through the evaluation of the `createUser` handler to pinpoint where the `TypeInfo` is lost or becomes `nil`.

- [ ] **Task 2: Fix the Evaluator**
    -   Based on the debugging, implement a fix in the evaluator to ensure type information is correctly maintained and propagated from a variable's declaration to its use as an intrinsic argument.

- [ ] **Task 3: Re-enable Tests**
    -   Uncomment the response assertions in `examples/docgen/main_test.go` and verify that all tests pass.
