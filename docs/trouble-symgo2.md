# Troubleshooting symgo: undefined method WithReceiver

This document tracks the investigation into the `undefined method or field: WithReceiver for pointer type INSTANCE` error.

## Error Reproduction

The error is triggered by running the `find-orphans` end-to-end test:

```bash
make -C examples/find-orphans
```

The output log contains the following error:
```
time=2025-10-13T11:22:26.440Z level=ERROR msg="undefined method or field: WithReceiver for pointer type INSTANCE" in_func=findDirectMethodOnType in_func_pos=/app/symgo/evaluator/accessor.go:126:20 exec_pos=/app/symgo/evaluator/evaluator_eval_selector_expr.go:520 pos=/app/symgo/evaluator/accessor.go:191:13
```

## Debugging

I modified `symgo/evaluator/evaluator_log.go` to panic on error and dump the symbolic call stack.

### Panic Output

```
time=2025-10-13T11:23:37.060Z level=ERROR msg="undefined method or field: WithReceiver for pointer type INSTANCE" in_func=findDirectMethodOnType in_func_pos=/app/symgo/evaluator/accessor.go:126:20 exec_pos=/app/symgo/evaluator/evaluator_eval_selector_expr.go:520 pos=/app/symgo/evaluator/accessor.go:191:13
panic: terminating due to error: undefined method or field: WithReceiver for pointer type INSTANCE

goroutine 1 [running]:
github.com/podhmo/go-scan/symgo/evaluator.(*Evaluator).logcWithCallerDepth(0xc000866000, {0x6a0ee0, 0x80f420}, 0x8, 0x2, {0xc003f6e050, 0x41}, {0xc003f94118, 0x2, 0x2})
	/app/symgo/evaluator/evaluator_log.go:57 +0x6e5
github.com/podhmo/go-scan/symgo/evaluator.(*Evaluator).newError(0xc000866000, {0x6a0ee0, 0x80f420}, 0x38e999, {0x657910, 0x31}, {0xc003f94b18, 0x2, 0x2})
	/app/symgo/evaluator/evaluator_new_error.go:18 +0x245
github.com/podhmo/go-scan/symgo/evaluator.(*Evaluator).evalSelectorExpr(0xc000866000, {0x6a0ee0, 0x80f420}, 0xc0011415a8, 0xc003f4f150, 0xc0013ab700)
	/app/symgo/evaluator/evaluator_eval_selector_expr.go:520 +0x2813
... (omitted for brevity) ...
```

### Analysis

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

### Proposed Solution

The fix is to make the evaluator correctly handle this metacircular case. Inside `evalSelectorExpr`, the `case *object.Pointer:` block should be modified. The `switch` statement on the `pointee` (the dereferenced object) needs a new case: `case *object.Function:`.

This new case would specifically handle method calls on pointers to function objects. When the selector is `WithReceiver`, it would recognize this as a valid "meta-call" and return a new, callable `*object.Function`, allowing the analysis to proceed correctly.