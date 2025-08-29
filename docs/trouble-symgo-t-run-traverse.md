# Trouble: `symgo` fails to traverse into `t.Run` subtests

This document details the investigation into a bug where `find-orphans` incorrectly flags a function as an orphan when its only usage is within a subtest created by `t.Run`.

## Problem Description

The `find-orphans` tool is designed to identify unused functions. When the `--include-tests` flag is used, functions used within tests should not be considered orphans. The initial user report suggested this was not working correctly.

A specific failure case was identified: a function called only from within a `t.Run(name, func(t *testing.T) { ... })` block was being flagged as an orphan.

A minimal test case was created in `examples/find-orphans/main_test.go` to reproduce this:

```go
// main_test.go
func TestFindOrphans_SubtestUsage(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/subtest-usage\ngo 1.21\n",
		"lib/lib.go": `
package lib
// This function should NOT be an orphan because it's used by a subtest.
func usedOnlyBySubtest() {}
`,
		"lib/lib_test.go": `
package lib
import "testing"
func TestSomething(t *testing.T) {
    t.Run("subtest", func(t *testing.T) {
        usedOnlyBySubtest()
    })
}
`,
	}
	// ... run find-orphans with --include-tests and --mode=lib ...
}
```

This test consistently fails, with `usedOnlyBySubtest` being incorrectly reported as an orphan.

## Investigation and Failed Attempts

The investigation revealed a two-part problem, and attempts to fix it failed, pointing to a deeper misunderstanding of the `symgo` evaluation process.

### Part 1: Test function arguments

The first identified issue was that the `analyzer` in `find-orphans` was starting the symbolic execution from test entry points (e.g., `TestSomething`) without providing any arguments.

```go
// examples/find-orphans/main.go
// The call was originally:
interp.Apply(ctx, ep, []object.Object{}, ep.Package)
```

This meant that inside the symbolic execution of `TestSomething`, the `t` parameter was unbound, causing the evaluation of `t.Run` to fail because `t` was an unresolved identifier.

**Attempted Fix 1:** Modify `examples/find-orphans/main.go` to supply a symbolic argument.

The code was changed to detect if an entry point was a test function and, if so, create a symbolic placeholder for its first argument (`*testing.T`).

```go
// examples/find-orphans/main.go (in the analysis loop)
args := []object.Object{}
if ep.Def != nil && isTestFunction(ep.Def) {
    if len(ep.Parameters.List) > 0 {
        // ... logic to resolve the type of the first parameter (*testing.T)
        // and create a symbolic placeholder with that type ...
        symbolicT := &object.SymbolicPlaceholder{...}
        args = append(args, symbolicT)
    }
}
interp.Apply(ctx, ep, args, ep.Package)
```

This change correctly creates a `Variable` named `t` in the function's environment, whose `Value` is a `SymbolicPlaceholder` correctly typed as `*testing.T`. This was verified by debug logs.

**Result:** This change alone was not sufficient. The test still failed.

### Part 2: Tracing into `t.Run`

The second part of the problem is that `symgo` does not automatically execute the body of a function literal that is passed as an argument to another function. When `t.Run` is called, the evaluator sees the `*object.Function` for the subtest, but doesn't evaluate its `Body`.

**Attempted Fix 2:** Add special handling for `t.Run` in the evaluator.

The `evalCallExpr` function in `symgo/evaluator/evaluator.go` was modified to recognize calls to `t.Run`. The idea was that if the call is `t.Run`, the evaluator should find the function literal argument and explicitly evaluate its body.

```go
// symgo/evaluator/evaluator.go (in evalCallExpr)
if sel, ok := n.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Run" {
    if fn, ok := function.(*object.Function); ok && fn.Def != nil && fn.Def.Receiver != nil {
        // Check if receiver is *testing.T
        recvType := fn.Def.Receiver.Type
        if recvType.IsPointer && recvType.Elem != nil && recvType.Elem.PkgName == "testing" && recvType.Elem.TypeName == "T" {
            // Find the function literal in the args and evaluate its body
            for _, arg := range args {
                if subtestFn, ok := arg.(*object.Function); ok && subtestFn.Body != nil {
                    e.Eval(ctx, subtestFn.Body, /* new env */, subtestFn.Package)
                }
            }
        }
    }
}
```

### Combined Result

The combination of both fixes was expected to solve the issue:
1.  `main.go` provides a typed, symbolic `t` variable.
2.  `evaluator.go` sees the call `t.Run`, identifies `t` as a `*testing.T`, and then evaluates the subtest body.

However, even with both changes in place, the test **still fails**.

## Conclusion and Next Steps

The failure of the combined approach indicates a fundamental misunderstanding of the `symgo` evaluation flow.
- It's possible the type information of the symbolic `t` is not being correctly propagated or checked.
- It's possible the `e.Eval(ctx, subtestFn.Body, ...)` call inside `evalCallExpr` does not affect the main analysis's `usageMap`. The `usageMap` is populated by a `defaultIntrinsic`, and perhaps this special-case evaluation bypasses that.
- There might be an issue with how environments are being created or enclosed.

Further, deeper debugging of the `symgo` evaluator is required. One would need to trace the entire evaluation of the `t.Run` call, from the `evalIdent` of `t`, through the `evalSelectorExpr`, and into the `applyFunction` call, to see exactly where the analysis chain is breaking.

## Follow-up Issue (Unresolved): Nested Function Calls as Arguments

A follow-up issue was reported where a function call nested inside another function call's argument list was not being detected by `find-orphans`.

### Problem Description

Consider the following code structure:

```go
// Simplified example
h = apitestlib.PackMiddleware(h, apitest.PackComponents(
    db.Db,
    apitest.FixedClock(fakeNow, "2023-02-02"),
))
```

In this scenario, the call to `apitest.FixedClock` is reportedly not being detected, causing `find-orphans` to flag it as unused.

### Analysis

The `symgo` evaluation flow for this expression should be:
1.  The evaluator starts processing the `PackMiddleware` call.
2.  To resolve the arguments for `PackMiddleware`, it must first evaluate the inner call to `PackComponents`.
3.  To resolve the arguments for `PackComponents`, it must evaluate the call to `FixedClock`.
4.  This evaluation of the `FixedClock` call expression should trigger the `defaultIntrinsic` used by `find-orphans` to mark the function as "used".

This is distinct from the original `t.Run` issue. The `t.Run` issue involved a *function literal* being passed as an argument, whose body was not being scanned. This new issue involves a standard *function call* whose result is passed as an argument. The existing evaluation logic is expected to handle this case.

### Status

**Unresolved.** The reason for the failure is currently unknown. The analysis of the evaluator's logic suggests the call should be detected. Without a minimal, reproducible test case, it is difficult to debug the subtle interaction that is causing this failure. This issue is documented here for future investigation.
