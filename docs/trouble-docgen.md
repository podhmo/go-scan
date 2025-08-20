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

## Part 3: `symgo` Fails to Resolve Methods on External Interfaces (In Progress)

This section details an issue where the `symgo` evaluator correctly resolves method calls on locally-defined interfaces but fails for interfaces imported from external packages, such as `net/http.ResponseWriter`.

### Symptom

A new test case was added to `docgen` to analyze a handler wrapped in `http.TimeoutHandler`. This required the evaluator to resolve method calls on `http.ResponseWriter`, which is an interface. The analysis fails because the intrinsic for `(net/http.ResponseWriter).Write` is not triggered.

To isolate the issue, two minimal test cases were created in `symgo/evaluator`:
1.  `TestEval_InterfaceMethodCall`: Uses a locally-defined `Writer` interface.
2.  `TestEval_ExternalInterfaceMethodCall`: Uses a `Writer` interface defined in a separate package within the test module.

Initially, both tests failed.

### Root Cause Analysis

The investigation revealed two distinct bugs:

1.  **Bug 1: Incorrect Parameter Handling (Fixed)**: The `symgo` evaluator's `extendFunctionEnv` function was not correctly creating `*object.Variable`s for function parameters. It was binding the raw argument value (e.g., `nil`) directly to the parameter name, causing the variable's type information (e.g., `Writer` interface) to be lost.
    -   **Fix**: The `extendFunctionEnv` function was modified to resolve the parameter's type from the function declaration and create a proper `*object.Variable`, associating the type with the argument value.

2.  **Bug 2: Missing Module Resolver in `scantest` (Identified)**: After fixing Bug 1, the `TestEval_ExternalInterfaceMethodCall` still failed with a `package directory ... is outside the module root` error. The root cause was discovered to be the `scantest.Run` helper function, which does not configure its `go-scan` scanner with `goscan.WithGoModuleResolver()` by default. This prevents it from finding and resolving types from any package other than the one being directly scanned.

### Current Status & Next Steps

The `symgo` evaluator itself is now believed to be correct. The remaining problem lies entirely within the test setup for both the `symgo` tests and the `docgen` integration test.

- [ ] **Task 1: Fix `symgo` tests**:
    -   Modify `TestEval_InterfaceMethodCall` to pass `scantest.WithModuleRoot(dir)` to `scantest.Run`.
    -   Modify `TestEval_ExternalInterfaceMethodCall` to create a scanner with `goscan.WithGoModuleResolver()` and pass it to `scantest.Run` using `scantest.WithScanner()`, in addition to passing `scantest.WithModuleRoot(dir)`.

- [ ] **Task 2: Verify `docgen` Test**: Once the `symgo` tests are passing, the `docgen` test should also pass without further changes, as its scanner is already correctly configured. Run the full `make test` suite to confirm.

---

## Part 4: `minigo` vs. `symgo` Import Handling Comparison

To understand the `symgo` issue, an investigation was conducted into `minigo`'s import and package loading mechanism.

### `minigo` Analysis

-   **Scanner Configuration**: `minigo.NewInterpreter` *always* adds `goscan.WithGoModuleResolver()` when creating its internal `go-scan` scanner. This makes it robust for handling external packages by default.
-   **Import Handling**: It uses a `FileScope` object to track `import` aliases for each file.
-   **Lazy Loading**: When a symbol from an un-scanned package is accessed, `minigo`'s evaluator calls `scanner.FindSymbolInPackage()`. The module-aware scanner then locates and parses the required package on-demand. The results are cached in the `minigo.Interpreter`.

### `symgo` Analysis

-   **Scanner Configuration**: `symgo.NewInterpreter` relies on the *caller* to provide a correctly configured scanner. This provides flexibility but led to the issues in the `symgo` and `docgen` tests, where the test setup (`scantest`) did not provide a module-aware scanner by default.
-   **Import Handling**: `symgo`'s evaluator uses a similar lazy-loading mechanism, calling `scanner.ScanPackageByImport()` when an external symbol is accessed.
-   **Conclusion**: The core logic in `symgo` is sound, but it is more sensitive to the configuration of the scanner it is given. The failures were not in the evaluator's logic itself, but in the test harness's setup of the scanner.
