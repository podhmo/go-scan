# Troubleshooting: `symgo` Fails to Model Map Assignment, Causing Infinite Loop

This document details the investigation of an issue where the `find-orphans` tool, which is powered by the `symgo` engine, fails to analyze the `examples/convert` project.

The initial hypothesis that `symgo`'s recursion detector was overly aggressive was incorrect. A deeper analysis, prompted by user feedback, revealed the true root cause: **a bug in the `symgo` evaluator that prevents it from modeling state changes from map assignments.** This causes the analyzed code to enter a genuine infinite loop, which `symgo`'s recursion detector then correctly identifies and halts.

## 1. The Problem

When running `find-orphans` on `examples/convert`, the process hangs. Debug logs show a very deep call stack that alternates between two functions in `examples/convert/parser/parser.go` at the exact same line numbers:

- `processPackage` (called from `resolveType` at `parser.go:270:13`)
- `resolveType` (called from `processPackage` at `parser.go:148:27`)

The repetitive nature of the stack trace is the key symptom, indicating the analysis is not progressing and is stuck in a non-productive loop.

```
 stack.5.func=processPackage
 stack.5.pos=.../parser.go:270:13
 stack.6.func=resolveType
 stack.6.pos=.../parser.go:148:27
 stack.7.func=processPackage
 stack.7.pos=.../parser.go:270:13
 ... (repeats for hundreds of frames)
```

## 2. Investigation

### Step 1: `symgo`'s Symbolic Execution Model

A review of `docs/analysis-symgo-implementation.md` and `symgo/evaluator/evaluator.go` confirms that `symgo` is a symbolic tracer, not a standard interpreter. When it encounters an `if` statement, it evaluates the condition expression (to trace calls) but then proceeds to evaluate **all** possible branches (`then` and `else`). It does not use the condition's result to decide which single path to take. This is the correct, intended behavior.

### Step 2: The Guard Condition in `parser.go`

The `parser.go` code is designed to be recursive to discover type dependencies. It contains a crucial guard condition to prevent processing the same package multiple times, which relies on a map to track state:

```go
// examples/convert/parser/parser.go:41
func processPackage(...) error {
	// THE GUARD: This check should prevent infinite loops.
	if pkgInfo == nil || info.ProcessedPackages[pkgInfo.ImportPath] {
		return nil
	}
	// THE STATE CHANGE: This update should make the guard effective.
	info.ProcessedPackages[pkgInfo.ImportPath] = true
	// ...
}
```

For the program to enter an infinite loop, the state change on line 45 must not be taking effect during symbolic execution, causing the guard on line 42 to always fail (i.e., not return).

### Step 3: The Bug in `evalAssignStmt`

The investigation then turned to how `symgo` handles map assignments. The relevant code is in `evalAssignStmt` for a left-hand side of type `*ast.IndexExpr` (which handles `m[k] = v`).

```go
// symgo/evaluator/evaluator.go:1930 (approx)
case *ast.IndexExpr:
    // This is an assignment to a map or slice index, like `m[k] = v`.
    // We need to evaluate all parts to trace calls.
    e.Eval(ctx, lhs.X, env, pkg)     // e.g., `info.ProcessedPackages`
    e.Eval(ctx, lhs.Index, env, pkg) // e.g., `pkgInfo.ImportPath`
    e.Eval(ctx, n.Rhs[0], env, pkg)  // e.g., `true`
    return nil
```

This code reveals the bug: **The evaluator traces function calls within the expressions, but it does not model the actual state change.** It never modifies the underlying `object.Map` that represents `info.ProcessedPackages`.

### Step 4: The True Root Cause

1.  The `parser.go` code relies on updating the `info.ProcessedPackages` map to terminate its recursion.
2.  The `symgo` evaluator, when analyzing this code, evaluates the expressions involved in the map assignment (`info.ProcessedPackages[pkgInfo.ImportPath] = true`) but **never actually performs the assignment** on its internal symbolic representation of the map.
3.  Because the symbolic map is never updated, the guard condition `info.ProcessedPackages[pkgInfo.ImportPath]` always evaluates to a symbolic placeholder representing "key not found". The `if` statement's `then` block, which would terminate the function, is executed, but the function continues to be called from its call site in `resolveType`.
4.  Because the state of `info` never changes, `resolveType` eventually calls `processPackage` again with the **exact same arguments**.
5.  `symgo`'s recursion detector in `applyFunction` sees this second identical call. It compares the function definition and arguments to the previous frame on the call stack. Since they are identical, it correctly identifies a non-productive infinite loop and halts the analysis by returning an error.

## 3. Conclusion and Next Steps

The user's skepticism was correct. The issue is not that `symgo`'s recursion detector is too aggressive; it is working perfectly. The problem is a critical bug in the evaluator: **it does not model state changes for map index assignments**.

This is a significant limitation that prevents `symgo` from correctly analyzing any algorithm that uses maps to track visited states, a common pattern in graph traversal and recursive analysis.

**The next steps are:**
1.  **Implement Map Index Assignment**: Fix the `symgo` evaluator by implementing state changes for map index assignments (`m[k] = v`) in `evalAssignStmt`. This will involve retrieving the symbolic `object.Map` from the environment, evaluating the key and value, and setting the key-value pair within the symbolic map object.
2.  **Update `TODO.md`**: The task list must be updated to reflect this new, more accurate understanding of the required fix. The focus is no longer on refining the recursion detector but on fixing the handling of map assignments.
