# symgo: Regressions from Lazy Evaluation Implementation

This document outlines the various test failures that appear to be regressions caused by the introduction of a lazy evaluation mechanism for variables in the `symgo` engine. The failures are widespread across the `symgo`, `symgo/evaluator`, and `examples/find-orphans` packages.

The previous work is in `docs/trouble-symgo-nested-scope.md`. The goal was to fix "identifier not found" for unexported symbols by evaluating package-level variables lazily. However, this change has caused numerous regressions.

## Failure Categories

The test failures can be grouped into several distinct categories, likely stemming from one or two root causes.

### 1. Overly Lazy Evaluation / Values Not Being Forced

The most common failure mode is that variables are not being evaluated when they should be. The evaluator returns a symbolic placeholder (`<Symbolic: zero value for uninitialized variable>`) instead of a concrete value when a variable is used in contexts like function arguments, return statements, or binary expressions.

**Affected Tests:**
- `symgo.TestFeature_SprintfIntrinsic`: Fails to evaluate a variable passed to `Sprintf`.
- `symgo.TestSymgo_AnonymousTypes`: Fails to resolve a field access.
- `symgo/evaluator.TestEvalIncDecStmt`: `i++` results in a placeholder, not an integer.
- `symgo/evaluator.TestVariadicFunction`: Variadic arguments are not evaluated.
- `symgo/evaluator.TestEvalFunctionApplication`: Fails to return a concrete integer.
- `symgo/evaluator.TestEvalClosures`: Fails to return a concrete integer from a closure.
- `symgo/evaluator.TestGenericFunctionCall`: Fails to evaluate generic function return values.
- And many others.

**Hypothesis:** The mechanism to "force" the evaluation of a lazy variable (`object.Variable`) is either missing or not being triggered correctly in `evalCallExpr`, `evalBinaryExpr`, and other expression evaluation functions.

### 2. Method Dispatch Failure on Pointer Receivers

A critical error, `not a function: POINTER`, occurs when attempting to call a method on a pointer receiver.

**Affected Tests:**
- `examples/find-orphans.TestFindOrphans_interface`
- `symgo/evaluator.TestEval_InterfaceMethodCall_OnConcreteType`
- `symgo/evaluator.TestEval_InterfaceMethodCall_AcrossControlFlow`
- `symgo/integration_test.TestStateTracking_GlobalVarWithMethodCall`

**Hypothesis:** The `evalCallExpr` or its underlying dispatch logic incorrectly handles method calls on pointer types. Instead of resolving the method on the pointer's underlying type, it seems to be attempting to "execute" the pointer itself as a function, leading to the `not a function: POINTER` error.

### 3. Inconsistent Recursion Detection

The recursion detection mechanism is behaving inconsistently.

- **`symgo.TestCrossPackageUnexportedResolution`**: Fails with an `infinite recursion detected` error, suggesting a false positive. This seems to be a direct regression from the lazy-loading fix for unexported variables.
- **`symgo/evaluator.TestRecursion_method`**: Fails because it *does not* detect an obvious infinite recursion, suggesting a false negative.

**Hypothesis:** The logic for tracking the call stack and identifying recursive calls is flawed. It may not be correctly handling the new lazy variable evaluation or has other logic bugs.

### 4. Incorrect Orphan Detection in `find-orphans`

Multiple tests for `find-orphans` are failing because functions and methods that are clearly in use are being reported as orphans.

**Affected Tests:**
- `examples/find-orphans.TestFindOrphans_ShallowScan_UnresolvedInterfaceMethodCall`
- `examples/find-orphans.TestFindOrphans`
- `examples/find-orphans.TestFindOrphans_json`

**Hypothesis:** This is a direct symptom of categories #1 and #2. If method calls are not being traced correctly by the `symgo` engine, the `find-orphans` tool will naturally fail to build an accurate call graph, leading to false positives.

### 5. Type Resolution Failures

Some tests fail because they cannot resolve certain types, particularly `any`.

**Affected Tests:**
- `symgo/evaluator.TestEvaluator_ShallowScan_TypeSwitch`: `identifier not found: any`
- `symgo/evaluator.TestShallowScan_StarAndIndexExpr`: `identifier not found: any`

**Hypothesis:** There might be an issue with how the `universe` scope (which contains built-in types like `any`) is being handled, or how type information is propagated during shallow scanning.

## Plan for Resolution

The highest priority is to fix the overly lazy evaluation, as this is the most widespread issue and likely the root cause of many other failures.

1.  **Fix Lazy Evaluation Forcing**:
    -   Review the implementation of `evalVariable` and where it's called.
    -   Create a new helper function, `forceEval(object.Object)`, that checks if an object is a variable and, if so, evaluates it, returning the result. This function should be used ubiquitously before using an object's value.
    -   Integrate `forceEval` into `evalCallExpr` (for function arguments), `evalBinaryExpr`, `evalUnaryExpr`, and any other place where a concrete value is required.
2.  **Fix Pointer Receiver Method Dispatch**:
    -   Debug `evalCallExpr` and `evalSelectorExpr`.
    -   When the receiver of a method call is a pointer, ensure the logic correctly looks up the method on the pointed-to type rather than treating the pointer as a callable function. This likely involves dereferencing the pointer type before method lookup.
3.  **Address Recursion Bugs**:
    -   Once the evaluation and dispatch logic is more stable, re-examine the two failing recursion tests to diagnose the contradictory behavior.
4.  **Verify `find-orphans`**:
    -   After the above fixes are in place, re-run the `find-orphans` tests. It's expected that most of them will pass without further changes.
5.  **Fix Type Resolution**:
    -   Investigate the `identifier not found: any` errors. This might require ensuring the `universe` scope is correctly accessible in all evaluation contexts.

## Feedback (Post-Analysis)

An independent analysis of the `symgo` evaluator was conducted after the issues in this document were presumably fixed. The key findings are:

1.  **Issues Appear Resolved**: The failure categories described above (overly lazy evaluation, pointer dispatch errors, inconsistent recursion) were not observed in the current implementation. The test suite now demonstrates correct behavior in these areas, such as proper method calls on pointer receivers, reliable recursion detection, and correct evaluation of variables in various expression contexts.

2.  **Design is Sound**: The investigation concluded that the lazy evaluation model, combined with the "force evaluation" mechanism and special handling for interfaces, is a robust and effective design for the engine's static analysis goals. The fixes implemented based on the plan in this document have resulted in a sound architecture.

No further conceptual refinements to the core evaluation model are recommended based on the analysis.
