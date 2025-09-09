# Troubleshooting: `symgo` Fails to Model Map Assignment, Causing Infinite Loop

This document details the investigation of an issue where the `find-orphans` tool, which is powered by the `symgo` engine, fails to analyze the `examples/convert` project.

The initial hypothesis was that `symgo`'s recursion detector was overly aggressive. However, a deeper analysis revealed the true root cause: **a bug in the `symgo` evaluator that prevents it from modeling state changes from map assignments.** This causes the analyzed code to enter a genuine infinite loop, which `symgo`'s recursion detector then correctly identifies and halts.

## 1. The Problem

When running `find-orphans` on `examples/convert`, the process hangs. Debug logs show a very deep call stack that alternates between two functions in `examples/convert/parser/parser.go`: `processPackage` and `resolveType`.

```
 stack.3.func=processPackage
 stack.3.pos=.../parser.go:38:12
 stack.4.func=resolveType
 stack.4.pos=.../parser.go:148:27
 stack.5.func=processPackage
 stack.5.pos=.../parser.go:270:13
 stack.6.func=resolveType
 stack.6.pos=.../parser.go:148:27
 ... (repeats for 400+ frames)
```
The fact that the line numbers in the call stack are repetitive (`270` -> `148` -> `270` -> ...) suggests the program is not making new progress and is stuck in a loop.

## 2. Investigation

### Step 1: Analyzing the Target Code (`parser.go`)

The `parser.go` code is designed to be recursive to discover type dependencies. However, it contains a crucial guard condition to prevent processing the same package multiple times:

```go
// examples/convert/parser/parser.go

func processPackage(ctx context.Context, s *goscan.Scanner, info *model.ParsedInfo, pkgInfo *scanner.PackageInfo) error {
	// THE GUARD: This check should prevent infinite loops.
	if pkgInfo == nil || info.ProcessedPackages[pkgInfo.ImportPath] {
		return nil
	}
	// THE STATE CHANGE: This update should make the guard effective on subsequent calls.
	info.ProcessedPackages[pkgInfo.ImportPath] = true

	// ... rest of the function which calls resolveType, leading to recursion ...
}
```

For the tool to hang, the `info.ProcessedPackages` map must not be getting updated correctly during symbolic execution.

### Step 2: Analyzing the `symgo` Evaluator (`evaluator.go`)

The investigation then turned to how `symgo` handles map assignments. The relevant code is in `evalAssignStmt` for a left-hand side of type `*ast.IndexExpr` (which handles `m[k] = v`).

```go
// symgo/evaluator/evaluator.go

func (e *Evaluator) evalAssignStmt(...) {
	// ...
	if len(n.Lhs) == 1 && len(n.Rhs) == 1 {
		switch lhs := n.Lhs[0].(type) {
		// ...
		case *ast.IndexExpr:
			// This is an assignment to a map or slice index, like `m[k] = v`.
			// We need to evaluate all parts to trace calls.
			e.Eval(ctx, lhs.X, env, pkg)     // Evaluate the map/slice expression (e.g., `info.ProcessedPackages`).
			e.Eval(ctx, lhs.Index, env, pkg) // Evaluate the index expression (e.g., `pkgInfo.ImportPath`).
			e.Eval(ctx, n.Rhs[0], env, pkg)  // Evaluate the RHS value (e.g., `true`).
			return nil
		// ...
		}
	}
	// ...
}
```

This code reveals the bug: **The evaluator traces function calls within the expressions, but it does not model the actual state change.** It never modifies the underlying `object.Map` that represents `info.ProcessedPackages`.

### Step 3: The Root Cause

1.  The `parser.go` code relies on updating the `info.ProcessedPackages` map to terminate its recursion.
2.  The `symgo` evaluator, when analyzing this code, evaluates the expressions involved in the map assignment (`info.ProcessedPackages[pkgInfo.ImportPath] = true`) but **never actually performs the assignment** on its internal symbolic representation of the map.
3.  Because the symbolic map is never updated, the guard condition `info.ProcessedPackages[pkgInfo.ImportPath]` is always `false` (or, more accurately, the result of indexing a map for a key that isn't there).
4.  This causes the symbolic execution of `parser.go` to enter a genuine infinite loop.
5.  `symgo`'s recursion detector (`applyFunction` in `evaluator.go`) correctly identifies this non-terminating loop (as the function arguments become identical on subsequent calls) and halts the analysis.

## 3. Conclusion and Next Steps

The original conclusion was incorrect. The `symgo` recursion detector is working as intended. The problem is a critical bug in the evaluator: **it does not model state changes for map index assignments**.

This is a significant limitation that prevents `symgo` from correctly analyzing any algorithm that uses maps to track visited states, a common pattern in graph traversal and recursive analysis.

**The next steps are:**
1.  Implement the logic in `evalAssignStmt` to correctly handle assignments to `*ast.IndexExpr`. This will involve retrieving the symbolic `object.Map` from the environment, evaluating the key and value, and setting the key-value pair within the symbolic map object.
2.  Update the `TODO.md` to reflect this new, more accurate understanding of the required fix. The task is no longer about refining the recursion detector, but about fixing the handling of map assignments.
