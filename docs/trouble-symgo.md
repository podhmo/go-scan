# Troubleshooting: `symgo` Recursion Detector on Recursive Parsers

This document details the investigation of an issue where the `find-orphans` tool, which is powered by the `symgo` engine, fails to analyze the `examples/convert` project. The root cause is `symgo`'s own recursion detection being triggered by the legitimate, deeply recursive design of the code it is analyzing.

## 1. The Problem

When running the `find-orphans` tool on the `examples/convert` package, the process would hang, consuming significant memory and generating a massive, repetitive log file. The logs showed endless warnings from `symgo/evaluator/evaluator.go`. The ultimate goal of the `find-orphans` run was to determine if the function `formatCode` was correctly identified as "used". Instead, the analysis never completed. This pointed to an infinite loop or an overly aggressive termination condition within the `symgo` engine itself.

## 2. Investigation

The investigation focused on the interaction between the `symgo` engine and the code it was being asked to analyze, specifically the parser located at `examples/convert/parser/parser.go`.

### Step 1: Analyzing the Target Code (`parser.go`)
The `parser.go` file contains the core logic for the `convert` example. A review of its source code revealed a deeply, but correctly, recursive structure for discovering and resolving type dependencies. The key functions involved are `processPackage`, `resolveType`, and `collectFields`.

The recursion flows as follows:

1.  `processPackage` is called for a package.
2.  It finds annotations (like `@derivingconvert`) or rules (`// convert:rule`) that reference other types.
3.  For each referenced type, it calls `resolveType`. The log confirms this happens at `parser.go:148`.
4.  `resolveType` may discover that the type belongs to a different package. It then uses the `go-scan` `Scanner` to load this new package.
5.  Crucially, after loading the new package, `resolveType` immediately calls **`processPackage`** on it to ensure its types and rules are also parsed before proceeding. The log confirms this recursive call happens at `parser.go:270`.

This creates a legitimate, but complex and deep, mutually recursive call chain.

### Step 2: Analyzing the Call Stack
The debug logs provided a clear picture of the recursive loop. The `symgo` evaluator's call stack grows extremely deep, alternating between two functions in `parser.go`:

- `processPackage` (called from `resolveType` at `parser.go:270:13`)
- `resolveType` (called from `processPackage` at `parser.go:148:27`)

A snapshot of the call stack demonstrates this pattern clearly, showing the stack depth exceeding 400 calls:

```
 stack.3.func=processPackage
 stack.3.pos=.../parser.go:38:12
 stack.4.func=resolveType
 stack.4.pos=.../parser.go:148:27
 stack.5.func=processPackage
 stack.5.pos=.../parser.go:270:13
 stack.6.func=resolveType
 stack.6.pos=.../parser.go:148:27
 ...
 stack.399.func=processPackage
 stack.399.pos=.../parser.go:270:13
 stack.400.func=resolveType
 stack.400.pos=.../parser.go:148:27
```

This stack trace proves that for every type resolution (`resolveType`), the parser may scan a new package (`processPackage`), which in turn can trigger more type resolutions. This is the intended behavior of the parser, but it creates a very deep call chain for the `symgo` analyzer to follow.

### Step 3: Identifying the Root Cause
The problem is not a bug in the `parser.go` logic, nor is it a simple infinite loop. The root cause is a **design conflict**:
-   The `convert` parser is intentionally and correctly recursive to handle complex, cross-package Go projects.
-   The `symgo` engine's recursion detector (`applyFunction` in `evaluator.go`) is designed to be cautious and prevent its own execution from hanging.

When `symgo` analyzes the execution of the `convert` parser, the parser's deep but valid recursion is indistinguishable from a dangerous infinite loop to `symgo`'s detector. The detector is overly aggressive and terminates the analysis prematurely, leading to the observed hang and log spam as the tool struggles to make progress.

## 3. Conclusion and Next Steps

The failure of `find-orphans` on `examples/convert` is not due to an error in the `find-orphans` logic itself, but a fundamental limitation in the `symgo` engine that powers it. The recursion detector, while necessary, is not sophisticated enough to distinguish between malicious infinite loops and the complex, recursive algorithms often found in compilers and static analysis tools (like the parser it was analyzing).

To fix this, the `symgo` recursion detector needs to be refined. It must be made less aggressive, potentially by allowing a deeper recursion limit or by using more sophisticated heuristics to identify truly non-terminating loops, while allowing for the analysis of complex, recursive-by-design programs.
