# `symgo` Evaluator Panics on `panic(nil)`

**Date**: 2025-09-19
**Status**: Resolved

## Problem

When running symbolic analysis on Go code that contains a `panic(nil)` call, the `symgo` evaluator itself panics and crashes. This was discovered while running the `find-orphans` tool in library mode (`--mode lib`) on a large workspace, which triggered the analysis of a function containing this pattern.

The error log shows the evaluator failing during the symbolic execution of a specific function (`github.com/podhmo/go-scan/minigo.New` in this case):

```
level=ERROR ... msg="symbolic execution failed for entry point" function=github.com/podhmo/go-scan/minigo.New error="symgo runtime error: panic: nil\n"
```

Drilling down into the debug logs reveals that the evaluator is attempting to apply the intrinsic `panic` function with a `nil` argument, which causes a `nil pointer dereference` within the evaluator's own runtime.

## Root Cause

The root cause of this issue lies in the implementation of the `panic` intrinsic within the `symgo` evaluator. The code did not correctly handle the case where the argument to `panic` evaluates to `nil`.

When `symgo` encounters a `panic(expr)` call, it first evaluates `expr`. In this case, `expr` was `nil`. The intrinsic handler for `panic` received this `nil` value but did not have a proper guard. It attempted to access properties of the argument assuming it was a valid `object.Object`, leading to a `nil pointer dereference` inside the evaluator.

The expected behavior is for the evaluator to treat `panic(nil)` as a valid, albeit unusual, control flow event. It should wrap the `nil` value in a `symgo.PanicError` object and return it, allowing the symbolic execution to continue unwinding the stack, rather than crashing the analysis process.

## Solution

The fix involves modifying the `panic` intrinsic handler in `symgo/evaluator/evaluator.go`. A check is added at the beginning of the handler to test if the argument is `nil`. If it is, the handler now creates and returns a `*object.PanicError` containing the symbolic `object.NIL` constant.

This ensures that `panic(nil)` is handled gracefully as a symbolic event, allowing the evaluator to correctly model the program's behavior without crashing. A regression test was added to specifically cover the `panic(nil)` case.
