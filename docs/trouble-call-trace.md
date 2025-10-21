# Troubleshooting `call-trace` Interface Method Tracing

This document chronicles the debugging process for implementing interface method call tracing in the `examples/call-trace` tool. The task seemed straightforward initially but revealed several deep-seated complexities in the interaction between the `symgo` evaluator and the `call-trace` tool's analysis scope.

## The Goal

The objective was to extend the `call-trace` tool to trace function calls made through interfaces. For a given target function (e.g., a concrete method `(*MyType).MyMethod`), the tool should be able to find call stacks that arrive at this method via an interface that `*MyType` implements.

The initial proposed implementation was:
1.  **`symgo` Enhancement**: When `symgo` evaluates an interface method call, it should resolve all possible concrete implementations based on the known types of the interface variable (`PossibleTypes`). This list of concrete methods should be stored in a new field, `ConcreteImplementations`, on the `object.SymbolicPlaceholder` that represents the call.
2.  **`call-trace` Update**: The tool's `defaultIntrinsic` handler should be updated to inspect incoming `SymbolicPlaceholder` objects. If the `ConcreteImplementations` list is populated, it should iterate through it and check if any of the concrete methods match the target function.

## Step 1: Creating a Failing Test

Following Test-Driven Development (TDD) principles, the first step was to create a realistic test case that would fail with the current implementation but pass with the new feature.

A Domain-Driven Design (DDD)-style project structure was created under `testdata/interface_call_ddd`. This provided a multi-package scenario that mimics real-world code:
-   `domain`: Defines the `UserRepository` interface.
-   `infrastructure`: Provides a concrete implementation, `(*UserRepositoryImpl).Find`.
-   `usecase`: Depends on the `domain.UserRepository` interface and calls its `Find` method.
-   `myapp`: The main entry point, responsible for Dependency Injection (DI)—injecting the concrete `infrastructure` repository into the `usecase`.

The test case in `main_test.go` was configured to target `infrastructure.(*UserRepositoryImpl).Find`. The expectation was that `call-trace` would trace the call from `myapp.main` -> `usecase.(*UserUsecase).GetUserByID` -> `infrastructure.(*UserRepositoryImpl).Find`.

As expected, running this test failed. The tool reported "No calls to ... found." A `golden` file was created with this output to serve as the baseline.

## Step 2: The Debugging Odyssey

With a failing test in place, the implementation of the proposed solution began. This led to a long and complex debugging journey.

### Iteration 1: Initial Implementation and Failure

-   **`symgo` Changes**:
    1.  The `ConcreteImplementations []*scanner.FunctionInfo` field was added to `object.SymbolicPlaceholder`.
    2.  A helper, `findTypeInfoInAllPackages`, was added to `evaluator.go` to look up `*scanner.TypeInfo` from a fully qualified type name string.
    3.  `evaluator_eval_selector_expr.go` was modified. The logic for handling `staticType.Kind == scan.InterfaceKind` was updated to iterate over the `PossibleTypes` of the receiver variable, find the corresponding concrete methods, and attach them to the `ConcreteImplementations` field of the resulting `SymbolicPlaceholder`.
-   **`call-trace` Changes**:
    1.  The `defaultIntrinsic` handler in `main.go` was updated to check for `*object.SymbolicPlaceholder` and iterate through its `ConcreteImplementations`.

**Result**: The test still failed. The `golden` file was unchanged.

### Iteration 2: Investigating `PossibleTypes`

**Hypothesis**: The `PossibleTypes` map, which is crucial for resolving concrete types, was not being populated correctly.

1.  A `grep` for `PossibleTypes` pointed to `evaluator_eval_assign_stmt.go` as the primary location for updates.
2.  However, in the DDD test case, the assignment happens within a struct literal in `myapp/main.go`: `usecase := &usecase.UserUsecase{Repo: repo}`.
3.  This led to the realization that `evalCompositeLit` in `evaluator_eval_composite_lit.go` was responsible. The original implementation simply assigned the raw value to the struct field, without creating a `*object.Variable` to hold the `PossibleTypes` information.
4.  **Fix**: `evalCompositeLit` was modified. When initializing a struct field, it now checks if the field's static type is an interface. If so, it wraps the concrete value being assigned in a new `*object.Variable`, resolves the concrete type, and adds it to the new variable's `PossibleTypes` map.

**Result**: The test still failed.

### Iteration 3: Investigating `evalSelectorExpr`

**Hypothesis**: The logic in `evalSelectorExpr` was not correctly identifying the interface method call for expressions like `u.Repo.Find()`.

1.  The initial check for interface calls was guarded by `if ident, ok := n.X.(*ast.Ident)`. This only works for simple variables, not for field accesses like `u.Repo`.
2.  **Fix Attempt 1 (Failed)**: A major refactoring of `evalSelectorExpr` was attempted to evaluate the left-hand side (`n.X`) *before* any checks. This resulted in numerous syntax errors due to incorrect restructuring of the function's control flow. The file was reverted.
3.  **Fix Attempt 2 (Correct)**: A safer, incremental approach was taken. The original `if ident...` block was kept. A *new* block was added *after* it to handle other cases. This new logic first evaluates `n.X`. If the result is a `*object.Variable` whose static type is an interface, it applies the same `PossibleTypes` resolution logic.

**Result**: The test still failed. At this point, the `symgo` evaluator logic was believed to be largely correct. The focus shifted to the `call-trace` tool itself.

### Iteration 4: Investigating `call-trace`'s `analysisScope`

**Hypothesis**: `symgo` was working, but it wasn't being instructed to analyze the right packages. The `analysisScope` was likely incorrect, causing `symgo` to treat the call to `usecase.GetUserByID` as a symbolic, out-of-policy call, meaning its body (and the call to `Find`) was never evaluated.

1.  Debug logging was added to `call-trace/main.go` to print the computed `analysisScope`.
2.  **Confirmation**: The log revealed the scope only contained the `myapp` and `infrastructure` packages. It was missing `domain` and `usecase`.
3.  The cause was the scope discovery logic. It started from the target function's package (`infrastructure`) and only walked *up* the dependency chain (reverse dependencies). It found `myapp` (which imports `infrastructure`), but it did not trace `myapp`'s *forward* dependencies into `usecase`.
4.  **Fix**: The scope discovery logic was completely replaced. The new logic traverses both forward and reverse dependencies, starting from all packages that match the user's input patterns (e.g., `./...`). This ensures the entire dependency cone is included in the analysis.

**Result**: The test *still* failed. The output was still "No calls found."

### Iteration 5: The Contradiction and Final Discovery

This was the most confusing phase. Extensive logging was added to both `symgo` and `call-trace`.

1.  **`symgo` logs**: Confirmed that `evalCompositeLit` was setting `PossibleTypes` correctly, `evalSelectorExpr` was resolving the concrete method and populating `ConcreteImplementations`, and a `SymbolicPlaceholder` with this information was being returned.
2.  **`call-trace` logs**: Confirmed that the `defaultIntrinsic` was receiving this exact `SymbolicPlaceholder`, iterating the `ConcreteImplementations`, and that the `getFuncTargetName` was producing the correct string. A log message `!!! MATCH FOUND !!!` was printed, confirming that the `if calleeName == targetFunc` check was succeeding.
3.  **The Contradiction**: Despite the "MATCH FOUND" log, the test's final output, captured from the output buffer and logged by the test itself (`t.Logf`), was still "No calls found."

This pointed to a fundamental bug in the `defaultIntrinsic` closure itself.

**The Root Cause**: The signature of the `defaultIntrinsic` closure in `call-trace/main.go` was incorrect.
-   It was defined as `func(ctx context.Context, args ...object.Object) object.Object`.
-   The correct `symgo.IntrinsicFunc` type that `RegisterDefaultIntrinsic` expects is `func(ctx context.Context, eval *Interpreter, args []Object) Object`.
-   My incorrect signature in the `call-trace` `main.go` file caused the Go compiler to misinterpret the arguments when the wrapper inside `symgo` called it. The `*symgo.Interpreter` was being passed, but my function signature didn't account for it, leading to a silent runtime mismatch. The `args` slice was effectively shifted, and the `calleeObj` was incorrect.

The fix was to correct the signature in `main.go`'s `RegisterDefaultIntrinsic` call to match `symgo.IntrinsicFunc` and correctly handle the `interpreter` and `args` parameters.

## Conclusion

The path to a solution was obstructed by several distinct bugs that needed to be fixed in sequence:
1.  Incorrect `PossibleTypes` population in `evalCompositeLit`.
2.  Incomplete interface call detection in `evalSelectorExpr`.
3.  Insufficient dependency traversal in `call-trace`'s `analysisScope` logic.
4.  A subtle but critical bug in the signature of the `defaultIntrinsic` closure that prevented results from being recorded correctly.

The prolonged nature of the debugging was caused by the interaction of these bugs and a series of incorrect assumptions during the investigation, culminating in the final realization of the incorrect function signature.
