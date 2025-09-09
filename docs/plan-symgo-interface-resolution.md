# `symgo`: A Test-Driven Approach to Interface Resolution

This document describes the strategy and process used to build a robust, test-driven interface resolution mechanism for the `symgo` engine. The primary goal was to create a comprehensive test suite that could validate complex, real-world scenarios, and then to implement the necessary engine features to make those tests pass.

## 1. The Problem

The initial version of the `symgo` engine had an unreliable and incomplete implementation for resolving interface method calls. It was not able to correctly identify all concrete implementations of an interface, particularly when those implementations existed in different packages or were assigned to variables across different control-flow paths. This made it impossible to build reliable analysis tools like `find-orphans`.

## 2. The Strategy: Two-Phase Resolution via `Finalize()`

To solve this problem, the core idea was to implement a two-phase analysis mechanism.

-   **Phase 1: Collection (During Symbolic Execution):** As the engine evaluates code, it records all method calls made on interface-typed variables. Crucially, it also tracks every concrete type that is assigned to an interface variable, accumulating a set of "possible types".
-   **Phase 2: Resolution (Post-Execution):** A new public `Finalize()` method was added to the `Evaluator`. This method is called *after* symbolic execution is complete. It uses the collected information to build a complete map of all struct-to-interface implementations across all scanned packages. It then iterates through the recorded interface method calls and connects them to all possible concrete implementations, marking them as "used".

This architecture provided the foundation needed to build a comprehensive set of validation tests.

## 3. Building a Comprehensive Test Suite

The most important part of this effort was defining and implementing a test suite that covered the complex scenarios that the engine must support. The following key test cases were developed, and the engine was enhanced until they passed.

### 3.1. Test Case: Cross-Package and Order-Independent Resolution

A fundamental requirement is that analysis should work correctly regardless of the project structure. The `TestInterfaceResolution` test validates this by creating a three-package setup:
-   **Package A:** Defines an interface `I`.
-   **Package B:** Defines a struct `S` that implements `I`.
-   **Package C:** Contains a function that accepts `I` and calls its method.

The test confirms that `symgo` can connect the interface method call in package C to the concrete implementation in package B. While the current test validates a single, successful discovery order, the test harness was originally designed with the intention of expanding it to cover all six possible permutations of package discovery (e.g., A->B->C, A->C->B, etc.) to guarantee the resolution logic is truly order-independent. This full permutation testing remains a future enhancement.

### 3.2. Test Case: Path-Sensitive Type Accumulation

Real-world code frequently assigns different concrete types to the same interface variable in different control-flow branches. The `TestEval_InterfaceMethodCall_AcrossControlFlow` test validates this exact scenario:

```go
var a Animal // Animal is an interface
if condition {
    a = &Dog{}
} else {
    a = &Cat{}
}
a.Speak() // Must be linked to both Dog.Speak and Cat.Speak
```

To make this test pass, the evaluator's state management was significantly improved. It now correctly accumulates all possible concrete types for a variable across `if/else` branches. This was achieved by fixing a bug where the internal `PossibleTypes` map was using non-unique keys for different pointer types, causing them to overwrite each other. With the fix, the engine correctly identifies that `a.Speak()` can refer to methods on both `*Dog` and `*Cat`.

### 3.3. Test Case: Manual Bindings and Intrinsics

For advanced use cases, the engine must provide an "escape hatch" to manually specify the concrete type of an interface. The `TestInterfaceBinding` test validates this feature, including its interaction with the intrinsics system.

The test binds `io.Writer` to `*bytes.Buffer` and confirms that a call to `writer.WriteString()` correctly triggers an intrinsic registered for `(*bytes.Buffer).WriteString`. This required fixing a critical bug where the `BindInterface` mechanism was discarding pointer information (`*`). The binding logic was enhanced to preserve this information, allowing the evaluator to construct the correct lookup key for the intrinsic.

### 3.4. Test Case: Edge Cases (e.g., Nil Receivers)

The test suite also covers edge cases, such as method calls on `nil` interface values. The `TestDefaultIntrinsic_InterfaceMethodCall` test confirms that the engine correctly models this scenario without crashing, allowing analysis to continue.

## 4. Conclusion

By prioritizing the creation of a comprehensive test suite first, we were able to drive the development of a robust and reliable interface resolution engine. The final implementation, centered around the two-phase `Finalize()` mechanism, now successfully passes all of these complex test cases, providing a solid foundation for building powerful static analysis tools.
