# Trouble Report: `symgo` TypeInfo Propagation Issue in `docgen`

## Goal
The objective is to implement request/response body analysis for the `docgen` tool. This involves using the `symgo` symbolic execution engine to analyze the body of HTTP handler functions, detect calls to `json.Decode` and `json.Encode`, and generate OpenAPI schemas based on the types of the variables used in those calls.

## Approach Taken
The initial approach of using a simple AST visitor was abandoned based on user feedback to stick with the `symgo` engine. The chosen path involves enhancing `symgo` to understand Go types from `go-scan` during its evaluation.

The implementation involved several major refactoring steps:
1.  **Modified `symgo/object`**: The `object.Object` interface was extended with a `TypeInfo() *scanner.TypeInfo` method. A `BaseObject` struct was added to provide a default implementation and a field (`ResolvedTypeInfo`) to store the type information. New object types like `Variable` and `Pointer` were also introduced.
2.  **Exposed `scanner` Utilities**: Key functions from the internal `scanner` package, `TypeInfoFromExpr` and `BuildImportLookup`, were made public on the `goscan.Scanner` so they could be accessed by the `symgo` evaluator.
3.  **Enhanced `symgo/evaluator`**:
    *   The `Eval` function signature was changed from `Eval(node, env)` to `Eval(node, env, pkg *scanner.PackageInfo)` to provide the necessary package context for type resolution.
    *   Logic was added to `evalGenDecl` to handle `var` declarations. This logic uses the new scanner utilities to resolve the `scanner.TypeInfo` of a declared variable.
    *   The resolved `TypeInfo` is stored on the `object.Variable` that is created and placed in the environment.
    *   Logic was added to `evalUnaryExpr` to handle the `&` operator. When it creates a `*object.Pointer`, it retrieves the `TypeInfo` from the variable being pointed to and attaches it to the pointer object.
    *   Logic was added to `evalSelectorExpr` to handle field access on a `*object.Variable`, allowing the evaluator to trace expressions like `r.Body`.
4.  **Refactored `docgen`**:
    *   `analyzer.go` was updated to use a temporary, scoped intrinsic registry (`PushIntrinsics`/`PopIntrinsics`) on the main interpreter instance rather than creating a new interpreter for each handler.
    *   The intrinsics for `json.Decode` and `json.Encode` were implemented to receive the `symgo` objects, call the `TypeInfo()` method, and then generate a schema.

## The Problem: A Contradiction in State

Despite extensive refactoring and debugging, the implementation is failing. The test fails because the generated OpenAPI specification does not contain the request or response body schemas.

The core of the problem is a contradiction between the state I can observe in the evaluator and the state received by the intrinsic function.

**What the logs confirm:**
1.  When the evaluator processes `var user User` in a handler, the `evalGenDecl` function successfully resolves the type and logs: `evalGenDecl: resolved type for var","var":"user","type":"User"`. This confirms the `*scanner.TypeInfo` for `User` is found.
2.  When the evaluator processes `&user`, the `evalUnaryExpr` function is called. It evaluates `user`, which correctly returns the `*object.Variable` created in the previous step. The log confirms that it successfully retrieves the `TypeInfo` from this variable and attaches it to the new `*object.Pointer`: `evalUnaryExpr: attaching type to pointer","type":"User"`.
3.  The `json.Decode` intrinsic function *is* called. This is confirmed by a `DECODE INTRINSIC CALLED` log message.

**The Contradiction:**
Inside the `Decode` intrinsic, the very first thing I do is check the `TypeInfo` on the `*object.Pointer` argument. **It is always `nil`**.

This is where I am stuck. The evaluator appears to be correctly setting the `ResolvedTypeInfo` field on the pointer object it creates. The `applyFunction` logic passes the arguments to the intrinsic. But when the intrinsic receives the argument, the `ResolvedTypeInfo` field is `nil`.

I have tried adding extensive logging at every step of the process. The data seems to be correct right up until the point it crosses the boundary into the intrinsic function. I am missing a fundamental concept about how the `symgo` evaluator passes arguments or manages object state.

---
## Update: 2025-08-19

Based on user feedback, I pivoted to writing an isolated test case for the `symgo` engine to debug the `TypeInfo` propagation issue directly.

### What I Did
1.  **Created an Isolated Test:** I created `symgo/evaluator/integration_test.go` with a single test, `TestTypeInfoPropagation`, designed to check if a `TypeInfo` attached to a variable is correctly passed to an intrinsic function.
2.  **Fixed Test Infrastructure:** The existing `symgo` tests were broken by my previous changes. I spent considerable time fixing compilation errors and test setup issues, primarily related to incorrect package imports and incorrect use of the `scantest` test harness. This involved refactoring all `symgo` tests to correctly use temporary directories and `goscan.New(goscan.WithWorkDir(...))`.
3.  **Attempted to Fix the Evaluator:** I identified that the evaluator was not handling calls to non-packaged, global-scope functions (like the `inspect_type` intrinsic in my test). I attempted to fix this by modifying `evalIdent` to check the intrinsic registry.

### Accident Encountered
The tests still fail. The `TestTypeInfoPropagation` test fails with the message `intrinsic was not called`. This indicates that my fix to `evalIdent` was insufficient and the evaluator is still not resolving the function call correctly. Other tests for `symgo` also fail for similar reasons related to incorrect evaluation flow within function bodies.

### Miscalculation
My primary miscalculation was underestimating the complexity of the `symgo` evaluator and the importance of its execution model. My approach of simply evaluating a function's body (`*ast.BlockStmt`) in a new environment was flawed. I now understand that this bypasses the evaluator's own function application logic (`applyFunction`), which is responsible for correctly setting up the function's scope, including parameters.

The core issue is that my tests (and the `docgen` analyzer) are not correctly simulating a *call* to the function being analyzed. They are trying to evaluate its body as a standalone block, causing the environment and function resolution to fail.

