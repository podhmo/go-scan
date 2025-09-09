# Troubleshooting: `symgo`'s Inconsistent Bounded Analysis Strategy

This document details the investigation of an issue where the `find-orphans` tool, powered by the `symgo` engine, fails to analyze the `examples/convert` project. The root cause is an **inconsistency in `symgo`'s bounded analysis strategy**: it correctly bounds loop iterations but fails to bound recursive function calls, leading to impractical, deep analysis that appears as a hang.

## 1. The Problem

When running `find-orphans` on `examples/convert`, the process hangs. Debug logs show a very deep call stack (400+ frames) that alternates between two functions in `examples/convert/parser/parser.go`: `processPackage` and `resolveType`. While the arguments to these functions change slightly with each call, preventing a true "infinite loop" detection, the analysis goes impractically deep.

## 2. Investigation: The Principle of Bounded Analysis

A core design principle of a practical static analyzer like `symgo` is **bounded analysis**. To ensure the analyzer always terminates in a reasonable time, it must place limits on how deeply it explores certain language constructs.

The investigation compared how `symgo` applies this principle to `for` loops versus recursive function calls.

### `for` Loops: Correctly Bounded

As documented in `docs/analysis-symgo-implementation.md` and implemented in `evalForStmt`, `symgo` correctly bounds loops. It follows an **"unroll once"** strategy, where the body of a `for` loop is evaluated exactly one time. This is a deliberate and sensible limitation to extract symbolic information without getting stuck analyzing complex loop conditions or a large number of iterations.

### Recursive Functions: Unbounded

The same bounded analysis principle is **not** applied to recursive function calls. The `applyFunction` in `evaluator.go` only has two termination conditions for recursion:

1.  **A hard stack depth limit** of 4096, which is too high for practical analysis.
2.  **A true infinite loop detector**, which only fires if the *exact same function* is called with the *exact same arguments* (or receiver object).

The recursive calls in `parser.go` are not technically infinite; the arguments (like the package being processed) change, so the detector doesn't fire. However, the recursion is extremely deep. `symgo` dutifully follows this deep recursion, exploring hundreds of frames, which is computationally expensive and provides diminishing returns.

## 3. Conclusion: An Inconsistent Strategy

The problem is not a bug in the recursion detector, nor is it related to state management or `if` statements. The root cause is an **inconsistency in `symgo`'s design philosophy**.

The principle of bounded analysis is applied to loops but not to function recursion. `symgo` should treat deep recursion just like it treats a long-running loop: as a construct that should be bounded to ensure timely analysis. By failing to do so, it gets bogged down in a deep analysis that is practically, if not theoretically, infinite.

The user's intuition was correct: "it should be enough to call the recursion once."

### Next Steps

The `symgo` evaluator needs to be modified to make its analysis strategy consistent.

1.  **Implement Bounded Recursion**: The primary task is to modify `applyFunction` in `symgo/evaluator/evaluator.go`. It should be enhanced with a mechanism to limit the depth of a recursive call chain in the same way `for` loops are limited. For example, it could track the number of times a given function definition appears in the current call stack and stop the analysis if it exceeds a small, configurable threshold (e.g., 2 or 3).

2.  **Update `TODO.md`**: The task list must be updated to reflect this new, accurate understanding of the required fix. The task is to implement a bounded analysis strategy for function recursion.
