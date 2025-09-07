# `symgo` Refinement: Addressing State Management and Type Propagation Regressions

This document outlines the investigation and resolution of a series of regressions in the `symgo` symbolic execution engine, which were introduced after implementing a lazy evaluation mechanism for package-level variables. The regressions manifest as infinite recursion errors, incorrect "orphan" function reporting, and improper type propagation.

## 1. Core Problem: Infinite Recursion in `TestCrossPackageUnexportedResolution`

The most critical regression is an infinite recursion failure in `TestCrossPackageUnexportedResolution`. This test was specifically designed to validate state management of a package-level variable across cross-package recursive calls.

### Symptom

The test, which uses a `count` variable to terminate recursion, now fails with `infinite recursion detected: getSecretMessage`. This indicates that the `count++` operation within the recursive function is not persisting across calls. The `count` variable's state is effectively reset on each recursive invocation, causing the termination condition (`count > 0`) to never be met.

### Root Cause Analysis

The failure stems from a subtle issue in `evalIncDecStmt`. The implementation is as follows:

1.  `e.Eval(ctx, n.X, ...)` is called for the identifier (e.g., `count`).
2.  This resolves through `evalIdent` and `evalVariable` to the underlying `*object.Integer` stored in the variable's `Value` field.
3.  `evalIncDecStmt` then increments the `Value` field of this returned `*object.Integer` object.

While this appears correct (as it modifies a pointer), it seems this modification is not consistently reflected in the environment seen by subsequent recursive calls. This suggests a flaw in how environments are created or how variables are resolved, where a stale or copied version of the state is being used.

The most robust fix is to avoid modifying the returned value object indirectly. Instead, `evalIncDecStmt` should explicitly fetch the `*object.Variable` container from the environment and then update the `Value` it points to. This ensures the change is made directly to the canonical state holder in the environment.

## 2. Related Symptom: `find-orphans` Failures

Tests like `TestFindOrphans_ShallowScan_UnresolvedInterfaceMethodCall` and `TestFindOrphans_interface` are failing. They incorrectly report used methods as orphans.

### Symptom

-   `'CallMe' was reported as an orphan, but it should be considered used.`
-   `find-orphans mismatch... + "(example.com/find-orphans-test/speaker.*Cat).Speak"`

### Root Cause Analysis

These failures are classic signs that `symgo`'s call graph analysis is incomplete. Specifically, it's failing to trace method calls made through interfaces. When `main` calls a function that takes an interface, and that function calls a method on the interface, `symgo` isn't connecting the call to the concrete implementation (`CallMe` or `Speak`). This is likely because the engine loses track of the concrete type that was assigned to the interface variable. This is a type propagation issue, which may be exacerbated by the new lazy variable evaluation.

## 3. Related Symptom: Widespread Type Propagation Failures in `symgo/evaluator`

A large number of tests in `symgo/evaluator` are failing with similar patterns.

### Symptoms

-   `expected return value to be a *symgo.String, but got *object.SymbolicPlaceholder`
-   `V is not an integer, got *object.SymbolicPlaceholder`
-   `mismatch in inspected types (-want +got)`
-   `did not capture an interface method call, UnderlyingMethod was nil`

### Root Cause Analysis

These errors all point to a systemic problem where the evaluator loses or fails to determine the specific type of an object during execution, falling back to a generic `SymbolicPlaceholder`. This happens in various scenarios: generics, closures, type switches, and interface method calls.

The issue is likely connected to the lazy evaluation mechanism. As the `trouble-symgo-nested-scope.md` document noted, lazy evaluation is not being triggered in all necessary contexts. When a variable holding a concrete value (e.g., an integer) is passed to a function, its lazy initializer might not be evaluated, causing the function to receive a placeholder instead of the actual integer.

## Action Plan

The highest priority is to fix the state management bug causing the infinite recursion. This is a fundamental correctness issue.

1.  **Modify `evalIncDecStmt`**: Change the implementation to fetch the `object.Variable` from the environment and update its `Value` field directly. This will ensure state changes are atomic and correctly persisted in the environment.
2.  **Re-evaluate Other Failures**: After fixing the recursion bug, re-run all tests. It is possible that fixing the state management will also resolve some of the type propagation issues.
3.  **Address Type Propagation**: For the remaining failures, systematically enhance the evaluator to ensure `evalVariable` is called whenever a variable's value is needed, not just in `evalIdent`. This may involve adding explicit calls to `forceEval` in places like `evalCallExpr` before passing arguments to a function.
4.  **Fix Interface Resolution**: With state and types being more reliably propagated, the final step will be to ensure interface method calls are correctly resolved to their concrete implementations, which should fix the `find-orphans` failures.
