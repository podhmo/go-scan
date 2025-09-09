# Troubleshooting: `symgo` Interface Binding and Intrinsics

This document details the investigation and resolution of a bug where the `symgo` engine's `BindInterface` feature failed to correctly resolve method calls that had registered intrinsics.

## 1. The Problem

The `symgo.TestInterfaceBinding` test case was failing with the error: `undefined method "WriteString" on interface "Writer"`.

The test was designed to validate the following scenario:
1.  An interface (`io.Writer`) is manually bound to a concrete type (`*bytes.Buffer`) using `Interpreter.BindInterface()`.
2.  An intrinsic (a custom handler) is registered for the method on the concrete type: `(*bytes.Buffer).WriteString`.
3.  The symbolic engine analyzes a function that calls `WriteString` on an `io.Writer` variable.

The expectation was that the engine would use the binding to identify that `writer.WriteString` should resolve to `(*bytes.Buffer).WriteString` and then execute the registered intrinsic. The error message indicated that the engine was still treating `writer` as a generic `io.Writer` interface, which does not have a `WriteString` method.

## 2. Investigation

The investigation traced the execution flow from the `symgo.Interpreter` down to the `symgo.evaluator.Evaluator`.

### Step 1: Checking the `Interpreter`
The `symgo.Interpreter.BindInterface` method was examined first. It was correctly parsing the concrete type name (`*bytes.Buffer`), identifying it as a pointer, and looking up the `TypeInfo` for the base type (`bytes.Buffer`). It then called the evaluator's `BindInterface` method.

### Step 2: Discovering the Root Cause
The first critical bug was found in the communication between the `Interpreter` and the `Evaluator`. The `Interpreter` correctly determined if the concrete type was a pointer, but it **discarded this boolean flag**. It only passed the base `*goscan.TypeInfo` to the evaluator.

As a result, the `evaluator.Evaluator` stored its bindings in a `map[string]*goscan.TypeInfo`, which only knew that `io.Writer` should be treated as `bytes.Buffer`, with no knowledge of the crucial pointer (`*`).

### Step 3: Analyzing `evalSelectorExpr`
The `evalSelectorExpr` function in the evaluator is responsible for handling method calls (`x.Method()`). The logic for handling calls on interface types was attempting to use the bindings map. However, due to the missing pointer information, it could not construct the correct key to look up a potential intrinsic.

The test registered its intrinsic with the key `(*bytes.Buffer).WriteString`. The evaluator, lacking the pointer information, would have tried to look for something like `(bytes.Buffer).WriteString`, and failed. This caused it to skip the intrinsic check and proceed to a direct method lookup, which also failed because `io.Writer` doesn't have `WriteString`.

## 3. The Solution

A three-part fix was implemented to correctly propagate and use the pointer information.

### Part 1: Improved Data Structure
A new struct, `interfaceBinding`, was introduced in `evaluator.go`:
```go
type interfaceBinding struct {
	ConcreteType *goscan.TypeInfo
	IsPointer    bool
}
```
The `Evaluator.interfaceBindings` map was changed from `map[string]*goscan.TypeInfo` to `map[string]interfaceBinding`.

### Part 2: Updated `BindInterface` Methods
The `BindInterface` methods in both the interpreter and the evaluator were updated.
-   `symgo.Interpreter.BindInterface` was modified to pass the `isPointer` boolean it had already calculated to the evaluator.
-   `symgo.evaluator.Evaluator.BindInterface` was updated to accept the `isPointer` flag and store the new `interfaceBinding` struct in its map.

### Part 3: Fixed `evalSelectorExpr` Logic
The core logic in `evalSelectorExpr` was fixed. When a method call on a bound interface is found:
1.  It now retrieves the complete `interfaceBinding` struct.
2.  It uses the `IsPointer` flag and `ConcreteType`'s package path and name to construct the correct, fully-qualified receiver name (e.g., `*bytes.Buffer`).
3.  It uses this name to build the correct intrinsic key (e.g., `(*bytes.Buffer).WriteString`).
4.  It looks up this key in the intrinsics registry. **If found, it returns the intrinsic.**
5.  If not found, it falls back to the previous behavior of resolving the method as a standard function.

This change ensures that intrinsics registered on concrete types are correctly triggered even when the method call in the source code is on an interface type that has been manually bound. After implementing this fix, `TestInterfaceBinding` and all other tests in the `symgo` suite passed successfully.

---

# Troubleshooting: `symgo` Recursion Detector on Recursive Parsers

This document details the investigation of an issue where the `find-orphans` tool, which is powered by the `symgo` engine, fails to analyze the `examples/convert` project. The root cause is `symgo`'s own recursion detection being triggered by the legitimate, deeply recursive design of the code it is analyzing.

## 1. The Problem

When running the `find-orphans` tool on the `examples/convert` package, the process would hang, consuming significant memory and generating a massive, repetitive log file. The logs showed endless warnings from `symgo/evaluator/evaluator.go`, such as `expected multi-return value on RHS of assignment` and `unsupported LHS in parallel assignment`.

The ultimate goal of the `find-orphans` run was to determine if the function `formatCode` was correctly identified as "used". Instead, the analysis never completed. This pointed to an infinite loop or an overly aggressive termination condition within the `symgo` engine itself.

## 2. Investigation

The investigation focused on the interaction between the `symgo` engine and the code it was being asked to analyze, specifically the parser located at `examples/convert/parser/parser.go`.

### Step 1: Analyzing the Target Code (`parser.go`)
The `parser.go` file contains the core logic for the `convert` example. A review of its source code revealed a deeply, but correctly, recursive structure for discovering and resolving type dependencies. The key functions involved are:
-   `processPackage`: The main function that iterates over types and comments in a package.
-   `resolveType`: Resolves a type name (e.g., `"mypkg.MyType"`) to a full `scanner.TypeInfo`.
-   `collectFields`: Recursively gathers all fields from a struct, including those from embedded structs.

The recursion flows as follows:
1.  `processPackage` is called for a package.
2.  It finds annotations (like `@derivingconvert`) or rules (`// convert:rule`) that reference other types.
3.  For each referenced type, it calls `resolveType`.
4.  `resolveType` may discover that the type belongs to a different package. It then uses the `go-scan` `Scanner` to load this new package.
5.  Crucially, after loading the new package, `resolveType` immediately calls **`processPackage`** on it to ensure its types and rules are also parsed before proceeding.

This creates a legitimate, mutually recursive call chain: `processPackage` -> `resolveType` -> `processPackage` -> ...

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
