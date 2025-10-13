# `symgo`: Metacircular analysis fails for method calls on `*object.Function`

This document tracks the investigation into the `undefined method or field: WithReceiver for pointer type INSTANCE` error.

## Error Reproduction

The error is triggered by running the `find-orphans` end-to-end test, which causes `symgo` to analyze its own source code.

```bash
make -C examples/find-orphans
```

The output log contains the following error:
```
level=ERROR msg="undefined method or field: WithReceiver for pointer type INSTANCE" in_func=findDirectMethodOnType in_func_pos=/app/symgo/evaluator/accessor.go:126:20 exec_pos=/app/symgo/evaluator/evaluator_eval_selector_expr.go:520 pos=/app/symgo/evaluator/accessor.go:191:13
```

## Root Cause Analysis

The runtime stack trace shows the panic originates in `evalSelectorExpr`. The key log entries are:
- `in_func=findDirectMethodOnType`: The evaluator was *analyzing* this function.
- `pos=/app/symgo/evaluator/accessor.go:191:13`: The specific AST node being analyzed was `boundFn := baseFn.WithReceiver(receiver, receiverPos)`.

The error message `undefined method or field: WithReceiver for pointer type INSTANCE` reveals the root cause. This is a metacircular analysis problem.

1.  The `symgo` evaluator is analyzing its own source code, specifically `accessor.go`.
2.  It encounters the selector expression `baseFn.WithReceiver`.
3.  To evaluate this, it first evaluates the left-hand side, `baseFn`.
4.  Due to a flaw in how the evaluator represents its own internal types, the symbolic object for the `baseFn` variable (which is an `*object.Function` at runtime) is incorrectly created as an `*object.Pointer` pointing to an `*object.Instance` (where `TypeName` is "Function"), instead of an `*object.Pointer` pointing to an `*object.Function`.
5.  The `evalSelectorExpr` function then handles the `*object.Pointer`. It dereferences it, gets the `*object.Instance`, and tries to find the method `WithReceiver` on it.
6.  This fails because the `*object.Instance` type does not have a `WithReceiver` method.

The fundamental issue is the incorrect symbolic representation of `*object.Function` during self-analysis.

## Resolution

The issue was resolved by implementing the proposed solution. A new `case *object.Function:` was added to the `switch` statement within the `case *object.Pointer:` block in `symgo/evaluator/evaluator_eval_selector_expr.go`.

This new case handles method calls on a pointer to a function object. Specifically, when the selector is `WithReceiver`, the evaluator now recognizes this as a valid "meta-call." Instead of attempting to find the method on an `*object.Instance`, it clones the underlying `*object.Function`, binds the receiver to it, and returns the new function object. This allows the symbolic analysis to proceed without error.

A regression test, `TestMetaCircularAnalysis_MethodCallOnFunctionPointer`, was added to the `symgo/evaluator` package (as an external `evaluator_test` package to avoid import cycles). This test forces the evaluator to analyze its own code, specifically targeting the code path that triggered the bug, ensuring that the fix remains effective and preventing future regressions.