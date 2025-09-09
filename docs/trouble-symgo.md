# Troubleshooting: `symgo` Recursion Detector on Recursive Parsers

This document details the investigation of an issue where the `find-orphans` tool, which is powered by the `symgo` engine, fails to analyze the `examples/convert` project. The root cause is `symgo`'s own recursion detection being triggered by the legitimate, deeply recursive design of the code it is analyzing.

## 1. The Problem

When running the `find-orphans` tool on the `examples/convert` package, the process would hang, consuming significant memory and generating a massive, repetitive log file. The logs showed endless warnings from `symgo/evaluator/evaluator.go`, such as `expected multi-return value on RHS of assignment` and `unsupported LHS in parallel assignment`.

The ultimate goal of the `find-orphans` run was to determine if the function `formatCode` was correctly identified as "used". Instead, the analysis never completed. This pointed to an infinite loop or an overly aggressive termination condition within the `symgo` engine itself.

## 2. Investigation

The investigation focused on the interaction between the `symgo` engine and the code it was being asked to analyze, specifically the parser located at `examples/convert/parser/parser.go`.

### Step 1: Analyzing the Target Code (`parser.go`)
The `parser.go` file contains the core logic for the `convert` example. A review of its source code revealed a deeply, but correctly, recursive structure for discovering and resolving type dependencies. The key functions involved are `processPackage`, `resolveType`, and `collectFields`.

The recursion flows as follows:

1.  The analysis starts with `processPackage`. Inside this function, it iterates through the types in a package.
2.  In a loop over types (starting at `line 97`), it calls `resolveType` to resolve type names found in `@derivingconvert` annotations (`line 118`). It also calls `resolveType` when processing global conversion rules (`line 150`, `line 155`).
3.  `processPackage` also calls `collectFields` (`line 87`) to analyze struct fields.
4.  `collectFields` recursively calls `processPackage` (`line 278`) when it finds a field from another package, to ensure that package is fully processed.
5.  `resolveType` is the other major source of recursion. After resolving a type to a different package (`line 227`), it immediately calls `processPackage` on that newly discovered package (`line 232`) to parse its contents before continuing.

This creates a legitimate, but complex and deep, mutually recursive call chain:
-   `processPackage` -> `resolveType` -> `processPackage`
-   `processPackage` -> `collectFields` -> `processPackage`

Here are the specific code locations:

**`processPackage` calls `resolveType`:**
```go
// examples/convert/parser/parser.go:116
dstTypeInfo, err := resolveType(ctx, s, info, pkgInfo, dstTypeNameRaw)
```

**`resolveType` calls `processPackage`:**
```go
// examples/convert/parser/parser.go:232
if err := processPackage(ctx, s, info, resolvedPkgInfo); err != nil {
    return nil, fmt.Errorf("failed to process recursively discovered package %q: %w", resolvedPkgInfo.PkgPath, err)
}
```

### Step 2: Analyzing `symgo`'s Behavior
The `symgo` engine is a symbolic tracer. When it analyzes the `find-orphans` tool, it is essentially simulating its execution. As `find-orphans` executes the recursive logic in `parser.go`, `symgo`'s call stack deepens.

`symgo` has its own internal recursion detector (`applyFunction` in `evaluator.go`) designed to prevent it from getting stuck in infinite loops in the code it analyzes. This detector works by tracking function calls and halting if it detects a potentially non-terminating loop.

### Step 3: Identifying the Root Cause
The problem is not a bug in the `parser.go` logic, nor is it a simple infinite loop. The root cause is a **design conflict**:
-   The `convert` parser is intentionally and correctly recursive to handle complex, cross-package Go projects.
-   The `symgo` engine's recursion detector is designed to be cautious and prevent its own execution from hanging.

When `symgo` analyzes the execution of the `convert` parser, the parser's deep but valid recursion is indistinguishable from a dangerous infinite loop to `symgo`'s detector. The detector is overly aggressive and terminates the analysis prematurely, leading to the observed hang and log spam as the tool struggles to make progress.

## 3. Conclusion and Next Steps

The failure of `find-orphans` on `examples/convert` is not due to an error in the `find-orphans` logic itself, but a fundamental limitation in the `symgo` engine that powers it. The recursion detector, while necessary, is not sophisticated enough to distinguish between malicious infinite loops and the complex, recursive algorithms often found in compilers and static analysis tools (like the parser it was analyzing).

To fix this, the `symgo` recursion detector needs to be refined. It must be made less aggressive, potentially by allowing a deeper recursion limit or by using more sophisticated heuristics to identify truly non-terminating loops, while allowing for the analysis of complex, recursive-by-design programs.
