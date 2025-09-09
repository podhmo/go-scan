# Troubleshooting: `symgo` vs. State-Dependent Algorithms

This document details the investigation of an issue where the `find-orphans` tool, powered by the `symgo` engine, fails to analyze the `examples/convert` project.

The root cause is not a simple bug, but a fundamental design mismatch: **`symgo`, as a stateless symbolic tracer, is not designed to analyze algorithms that rely on stateful memoization for termination.**

## 1. The Problem

When running `find-orphans` on `examples/convert`, the process hangs. Debug logs show a very deep call stack that alternates between two functions in `examples/convert/parser/parser.go` at the exact same line numbers, indicating a non-productive loop.

## 2. Investigation

### Step 1: `symgo`'s Stateless Design

`symgo` is a symbolic tracer, not a standard interpreter. Its goal is to discover all possible execution paths. As documented in `docs/analysis-symgo-implementation.md`, it achieves this by:
-   Exploring **both** branches of an `if` statement, rather than evaluating the condition to choose one.
-   Unrolling loops **once** rather than tracking a loop counter to determine the exact number of iterations.

This stateless approach is a deliberate design choice. It allows `symgo` to analyze complex code without getting stuck in the halting problem or a combinatorial explosion of states. The trade-off is that it does not track the precise, concrete state of variables as they change.

### Step 2: The Parser's Stateful Algorithm

The code being analyzed, `parser.go`, uses a classic state-dependent algorithm to handle potentially circular dependencies between Go packages. It uses a map as a "visited" set to prevent processing the same package more than once.

```go
// examples/convert/parser/parser.go:41
func processPackage(...) error {
	// THE GUARD: This check relies on the state of the map.
	if pkgInfo == nil || info.ProcessedPackages[pkgInfo.ImportPath] {
		return nil
	}
	// THE STATE CHANGE: This mutation is critical for termination.
	info.ProcessedPackages[pkgInfo.ImportPath] = true
	// ...
}
```
Without the state change on line 45, any call to `processPackage` for a project with circular dependencies would lead to infinite recursion.

### Step 3: The Design Mismatch

The core of the problem lies in how `symgo` "executes" the parser's code:

1.  **State is Ignored**: In accordance with its stateless design, when `symgo` encounters the map assignment on line 45 (`info.ProcessedPackages[...] = true`), it notes that an assignment occurred but **does not modify its internal representation of the map object.** The symbolic `info.ProcessedPackages` map remains empty throughout the analysis.

2.  **Guard Fails**: When `symgo` evaluates the guard condition on line 42, the map access `info.ProcessedPackages[...]` always behaves as if the key is not present, because the symbolic map is always empty.

3.  **A "Real" Infinite Loop is Created**: Because the guard never effectively stops the recursion in the symbolic world, the execution of `parser.go` enters a genuine infinite loop. `resolveType` calls `processPackage` for a package, and that `processPackage` call, without a functioning guard, eventually calls `resolveType` again, leading back to another call to `processPackage` for the same package with the same arguments.

4.  **Recursion Detector Works Correctly**: `symgo`'s own recursion detector (`applyFunction` in `evaluator.go`) spots this non-productive loop (the same function being called with the exact same object references as arguments) and correctly halts the analysis to prevent its own process from hanging.

## 3. Conclusion

The issue is not a simple bug in the `symgo` evaluator, nor is its recursion detector "too aggressive." The problem is a fundamental **design limitation**. `symgo` is behaving as designed, but its stateless design is incompatible with the state-dependent termination logic of the code it is trying to analyze.

The `find-orphans` tool fails because it is asking a stateless analyzer to do something that requires stateful analysis.

### Next Steps

This issue requires a strategic decision about the future of `symgo`:

1.  **Option 1: Enhance `symgo` (Make it more stateful)**: Modify `symgo` to correctly model state changes for common cases like map assignments. This would make `symgo` more powerful and capable of analyzing a wider class of algorithms. However, it would add significant complexity and could degrade performance if not implemented carefully. It represents a shift in `symgo`'s core design philosophy.

2.  **Option 2: Keep `symgo` Stateless (Accept the limitation)**: Acknowledge this as a known limitation. The tool is working as designed, and users should be aware that it cannot be used to analyze code whose termination depends on state changes that `symgo` does not model.

The `TODO.md` should be updated to reflect this choice. The immediate task is no longer a simple "bug fix" but a "design decision."
