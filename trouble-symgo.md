# Trouble: `symgo` hangs when analyzing recursive methods

This document describes an issue where the `symgo` symbolic execution engine enters an infinite loop when analyzing code containing certain types of recursive method calls.

## Symptoms

When running `symgo` on code containing recursive methods like `minigo/object.Environment.Get`, the process hangs and eventually times out. The logs are flooded with repeated warnings, originating from deep within the `symgo` evaluator:

```
level=WARN msg="expected multi-return value on RHS of assignment" \
  in_func=Get \
  in_func_pos=$HOME/ghq/github.com/podhmo/go-scan/minigo/object/object.go:1135:10 \
  exec_pos=$HOME/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator.go:2189
```

The key is the combination of the `exec_pos` pointing to the `symgo` evaluator and the `in_func_pos` pointing to the recursive method being analyzed. This indicates the evaluator itself is stuck in a loop, not the code being analyzed.

## Cause

The root cause is a flaw in `symgo`'s bounded recursion detection mechanism within `symgo/evaluator/evaluator.go` in the `applyFunction` method. The existing logic attempts to detect recursive calls by checking if the same function definition (`*scanner.FunctionInfo`) is already on the call stack.

For methods, it adds an additional check:

```go
if frame.Fn.Receiver != nil && frame.Fn.Receiver == f.Receiver {
    recursionCount++
}
```

This check compares the receiver `object.Object` by pointer identity. This is too strict for symbolic execution. When analyzing a method call like `e.outer.Get(name)`, the symbolic evaluation of `e.outer` can produce a *new* symbolic object for the receiver of the recursive call. Even though this new object represents the same conceptual entity, it has a different memory address.

As a result, `f.Receiver == frame.Fn.Receiver` evaluates to `false`, the `recursionCount` is not incremented, and the evaluator enters an infinite loop, repeatedly analyzing the same recursive call.

The fix is to make the recursion check more robust by not relying on pointer identity for the receiver. Instead, a combination of checking the function definition and the arguments passed to the function is a more reliable way to detect direct recursion.
