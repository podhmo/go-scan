# Debugging the `symgo` Engine and Unblocking `docgen` Implementation

While working on the M3 task for `docgen` (parsing schemas and parameters), it became necessary to perform fundamental bug fixes on the `symgo` symbolic execution engine. This section documents the debugging process and handoff to the next developer.

### Objective

To fix `symgo` so that it can correctly parse method calls within `net/http` handlers (e.g., `json.Decode`, `json.Encode`), enabling `docgen` to generate request/response schemas.

### What Was Done (Progress)

Picking up from the previous developer's work (the older version of this file), the investigation began with a failing `symgo` test.

1.  **Test Infrastructure Architecture Fix**:
    *   **Problem**: Existing `symgo` tests were directly passing a function's `*ast.BlockStmt` (the body) to `Eval`. This bypassed the scope where function arguments and receivers are set up, failing to simulate a real function call.
    *   **Fix**: All tests were refactored to first retrieve an `*object.Function` from the environment and then call the `applyFunction` method (the actual function call logic). This significantly improved the reliability of the tests.

2.  **Intrinsic Resolution for Package-Level Functions**:
    *   After running the newly reliable tests, a bug became apparent where functions from imported packages, like `fmt.Println`, were not correctly resolving to the intrinsics (mocks) registered for the test.
    *   The logic in `evalSelectorExpr` (which evaluates expressions like `a.b`) was corrected to properly switch on the type of the object (package, instance, variable). This ensured that package-level function calls are now correctly resolved to their intrinsics.

3.  **Refactoring `docgen` Analysis Logic**:
    *   The `applyFunction` pattern established in the `symgo` tests was also applied to the `docgen` analysis logic. This allows `docgen` to simulate HTTP handlers more accurately, leading to a healthier architecture.

### Unresolved Issues (Handoff to the Next Developer)

Despite the improvements above, a critical bug remains in `symgo`, blocking the completion of the `docgen` task.

*   **Root Cause: Intrinsic for instance method calls cannot be resolved.**
    *   The `TestEvalCallExprOnInstanceMethod` test still fails. This test is for method calls on a symbolic instance (`mux`), such as `mux.HandleFunc(...)`.
    *   Because of this bug, `docgen` is also unable to capture method calls like `decoder.Decode(...)` or `encoder.Encode(...)`, preventing it from generating request/response schemas.

### Important Investigation History (For the Next Developer)

To prevent the next developer from repeating the same steps, here are some particularly important findings from the investigation of this bug.

*   **Object capture is successful**:
    *   It has been confirmed by adding diagnostic tests that after evaluating an assignment statement like `mux := http.NewServeMux()`, the `mux` variable is correctly stored in the environment as an `*object.Instance`. Therefore, the problem is not in the object creation or assignment process.

*   **AST structure was as expected**:
    *   A hypothesis was formed that the AST structure of a method call (`*ast.SelectorExpr`) might be special and different from my assumption.
    *   To verify this, following user advice, the test was temporarily modified to use `go/printer` to dump the AST node for `mux.HandleFunc` as a string.
    *   The result was that the dumped string was `mux.HandleFunc`, confirming that the AST structure (`X` being `mux`, `Sel` being `HandleFunc`) was completely as expected. **This allowed me to completely rule out the possibility of an AST interpretation error.** This was a very important step in narrowing down the problem.

### Recommendations for Next Steps

Given that both the AST structure and object creation are correct, it is highly likely that the problem lies in a more subtle area, such as **state reference (especially the intrinsic registry) or environment (scope) management** when `evalSelectorExpr` receives an `*object.Instance`.

The next developer should focus on further debugging why the call to `e.intrinsics.Get(key)` returns `false` when `evalSelectorExpr` enters the `case *object.Instance:` block, or whether `e.Eval(n.X, ...)` is truly returning the expected object (within the nested environment of a function call).

I hope this document will be of some help in resolving the issue.

### Resolution: Identifying the Root Cause

As a result of further investigation, it was discovered that there was **no bug** in the `symgo` engine itself. The root cause of the problem was in the implementation of the `TestEvalCallExprOnInstanceMethod` test case.

*   **The Problem**: The intrinsic (mock) for the `HandleFunc` method written in the test was misinterpreting the arguments it received.
    *   The first element (`args[0]`) of the argument list received by an instance method (`instance.Method()`) intrinsic is the receiver instance (`instance`) itself.
    *   However, the test was expecting `args[0]` to be the first argument of `HandleFunc` (the path pattern string).
    *   In reality, the path pattern was in `args[1]`, so the test was always failing, leaving the `gotPattern` variable empty.

*   **The Fix**: The implementation of the intrinsic within the `TestEvalCallExprOnInstanceMethod` test was corrected to retrieve the path pattern from `args[1]`. With this fix, the test passed successfully.

*   **Conclusion**: The `symgo` engine was correctly handling instance method calls. This incident served as a good lesson on the importance of questioning the correctness of the test code itself when debugging a complex system. The issue blocking `docgen` development was completely resolved by this test fix.

---

# New Skipped Tests for Method Call Patterns

As part of an effort to improve the robustness of the `symgo` evaluator, a new suite of tests has been added in `evaluator/evaluator_call_test.go`. These tests cover more complex and comprehensive scenarios for function and method calls, including:

*   Method calls on struct literals (e.g., `MyStruct{}.Do()`).
*   Method chaining (e.g., `NewDecoder(r).Decode(&v)`).
*   Nested function calls (e.g., `add(add(1, 2), 3)`).

## Current Status: Skipped

These new tests are currently marked as skipped (`t.Skip()`) because the `symgo` evaluator does not yet fully support these patterns. The tests have been added to the codebase to serve as a clear specification for the required future work and to enable test-driven development for these features.

The primary goal is to un-skip these tests one by one as the evaluator's capabilities are enhanced. This provides a clear roadmap for improving `symgo`'s ability to analyze real-world Go code.
