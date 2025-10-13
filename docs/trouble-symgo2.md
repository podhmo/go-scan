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

## Proposed Solution

The fix is to make the evaluator correctly handle this metacircular case. Inside `evalSelectorExpr`, the `case *object.Pointer:` block should be modified. The `switch` statement on the `pointee` (the dereferenced object) needs a new case: `case *object.Function:`.

This new case would specifically handle method calls on pointers to function objects. When the selector is `WithReceiver`, it would recognize this as a valid "meta-call" and return a new, callable `*object.Function`, allowing the analysis to proceed correctly.

## Code to Reproduce

A minimal Go code snippet to trigger this specific bug within a `symgo` test would look something like this:

```go
package mytest

import "github.com/podhmo/go-scan/symgo/object"

func F(fn *object.Function) {
	// This selector expression is what causes the failure during
	// symbolic execution. The evaluator incorrectly represents `fn`
	// as a pointer to an INSTANCE, not a pointer to a FUNCTION.
	_ = fn.WithReceiver(nil, 0)
}
```

When the `symgo` evaluator analyzes this function `F`, it will fail with the `undefined method: WithReceiver` error because it dereferences the pointer to `fn` and gets a symbolic `*object.Instance` instead of the expected `*object.Function`.