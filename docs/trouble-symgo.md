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
