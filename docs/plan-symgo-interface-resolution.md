# `symgo`: Interface Resolution and Analysis

This document describes the `symgo` engine's capabilities for analyzing code that uses interfaces. The engine is designed to be robust, handling various scenarios from simple interface-to-struct implementation checks to complex, path-sensitive analysis and manual bindings.

## 1. Core Principles

`symgo`'s interface analysis is built on two main phases:

1.  **Symbolic Execution (Collection):** As the engine evaluates code, it tracks all method calls on variables that are typed as interfaces. It records which concrete types are assigned to each interface-typed variable, even across different control-flow paths.
2.  **Finalization (Resolution):** After symbolic execution is complete, a `Finalize()` step can be called. This step uses the collected information to build a complete map of which structs implement which interfaces across all scanned packages. It then connects the recorded interface method calls to their concrete implementations, marking the concrete methods as "used".

## 2. Supported Scenarios

The `symgo` engine and its test suite validate the following interface resolution scenarios.

### Scenario 1: Basic Implementation (`TestInterfaceResolution`)

This is the most fundamental case. The engine can correctly determine that a struct implements an interface, even when the interface, the struct, and the code using them are in different packages.

-   **Package A:** Defines an interface `Speaker` with a `Speak()` method.
-   **Package B:** Defines a struct `Dog` with a `Speak()` method, implementing the interface.
-   **Package C:** Contains a function that accepts a `Speaker` and calls `Speak()`.

`symgo`'s `Finalize()` step correctly identifies that `Dog.Speak` is a valid implementation for a call to `Speaker.Speak` and marks it as used. This works regardless of the order in which the packages are discovered.

### Scenario 2: Path-Sensitive Type Accumulation (`TestEval_InterfaceMethodCall_AcrossControlFlow`)

The evaluator is path-sensitive, meaning it explores all branches of control-flow statements like `if/else`. It correctly accumulates all possible concrete types that an interface variable can hold.

**Example Code:**
```go
var a Animal // Animal is an interface
if condition {
    a = &Dog{}
} else {
    a = &Cat{}
}
a.Speak() // symgo understands this could be Dog.Speak() or Cat.Speak()
```

The engine tracks that the variable `a` can be either a `*Dog` or a `*Cat`. When `Finalize()` is run, it correctly marks both `Dog.Speak` and `Cat.Speak` as "used". This was made possible by ensuring the internal `PossibleTypes` map for a variable uses a robust keying strategy that can distinguish between different pointer types.

### Scenario 3: Manual Interface Binding (`TestInterfaceBinding`)

For cases where the analysis cannot automatically determine the concrete type of an interface, a manual binding can be provided. This is particularly useful for setting up analysis entry points or for dealing with interfaces whose concrete types are determined by external factors.

**Example Code:**
```go
// Test setup
interp.BindInterface("io.Writer", "*bytes.Buffer")
interp.RegisterIntrinsic("(*bytes.Buffer).WriteString", myIntrinsic)

// Code to be analyzed
func TargetFunc(writer io.Writer) {
	writer.WriteString("hello") // symgo resolves this to the intrinsic
}
```

The `BindInterface` call instructs the engine to treat any variable of type `io.Writer` as if it were a `*bytes.Buffer`. When `writer.WriteString()` is evaluated, the engine uses this binding to look for the method `(*bytes.Buffer).WriteString`. This allows it to find not only the method itself but also any **intrinsics** registered for that specific concrete method.

This mechanism was fixed by ensuring that the pointer information (`*` in `*bytes.Buffer`) is preserved during the binding process, allowing the engine to construct the correct key for intrinsic lookups.

### Scenario 4: Method Calls on Nil Interface Receivers (`TestDefaultIntrinsic_InterfaceMethodCall`)

The engine correctly handles method calls on `nil` interface values. In Go, it is valid to call a method on a `nil` interface variable, which will result in a panic at runtime if the method is called. In the context of symbolic analysis, `symgo` models this by identifying that the receiver of the call is the `*object.Variable` holding the `nil` value, not the `nil` value itself. This allows analysis to proceed without crashing and enables tools to reason about such calls. The test for this scenario was updated to reflect this correct and more precise behavior.
