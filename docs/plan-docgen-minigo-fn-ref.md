# Plan: Implementing Type-Safe `docgen` Patterns via a Robust Library

## 1. Objective

The primary goal is to refactor `docgen`'s custom pattern configuration to be type-safe and developer-friendly. This will be achieved by replacing string-based keys (e.g., `Key: "my/pkg.MyFunc"`) with direct function references (e.g., `Fn: mypkg.MyFunc`) in the `minigo` configuration script.

A core principle of this task is to **enhance the robustness of the underlying `go-scan` and `minigo` libraries**. The goal is not to find test-specific workarounds but to improve the libraries so they can correctly handle complex, real-world Go module structures, such as the nested modules often used in testing environments.

## 2. Investigation & Analysis: The Root Cause

Previous attempts to implement this feature were blocked by a critical issue: the `minigo` interpreter, powered by `go-scan`, failed to resolve package imports within a nested Go module set up for testing. The error `undefined: api.API` occurred because the scanner could not find a package within its own module context when run via an external `go test` command.

This is not a testing anomaly but a fundamental limitation in the library's module resolution logic. The correct path forward is to fix this root cause.

### 2.1. The `minigo` Interpreter's Role

The interpreter must be enhanced to correctly represent Go functions and methods as first-class objects. This involves:

-   **Typed Nil Pointers**: To support method references like `(*MyType)(nil).MyMethod`, the interpreter must be able to handle `nil` pointers that retain their type information.
-   **Definition Environment (`DefEnv`)**: To ensure that functions can be symbolically executed correctly, any object representing a Go function must carry a reference to the environment of the package in which it was defined. This `DefEnv` is essential for resolving other symbols (functions, variables) from the same package during the function's analysis.

## 3. Revised Implementation Plan

This plan prioritizes fixing the core library issues to support the `docgen` feature naturally.

### 3.1. Core Library Enhancement: Robust Module Resolution

This is the most critical task. The goal is to make the library's behavior match the standard Go toolchain's.

-   **Component**: `go-scan` and `locator` packages.
-   **Action**: Investigate and fix the module resolution logic. The system must be able to correctly locate packages when a scan is initiated from a parent module (`go test` at root) but operates within a nested submodule that uses `replace` directives (e.g., in `testdata`). This will eliminate the test-blocking errors.

### 3.2. Core Library Enhancement: `minigo` Interpreter

-   **`minigo` Object Model (`minigo/object/object.go`)**:
    -   Implement a `GoSourceFunction` object to represent a Go function. This object must store the function's metadata (`*scanner.FunctionInfo`), its full package path, and its definition environment (`DefEnv`).
    -   Implement a `GoMethodValue` object to represent a method resolved from a type.

-   **`minigo` Evaluator (`minigo/evaluator/evaluator.go`)**:
    -   Update the evaluator's selector logic (`evalSelectorExpr`) to handle method references on typed `nil` pointers, creating `GoMethodValue` objects.
    -   Update the symbol resolution logic (`findSymbolInPackage`) to create `GoSourceFunction` objects, ensuring the `DefEnv` is captured.
    -   Update the function application logic (`applyFunction`) to use the `DefEnv` when evaluating a `GoSourceFunction`, ensuring correct symbol resolution within the function body.

### 3.3. `docgen` Feature Implementation

With a robust library backend, the `docgen` changes become straightforward.

-   **`docgen` Configuration (`examples/docgen/patterns/patterns.go`)**:
    -   Refactor `PatternConfig` to remove the string `Key` and add the `Fn any` field.

-   **`docgen` Loader (`examples/docgen/loader.go`)**:
    -   Update `convertConfigsToPatterns` to process the `Fn` field. It will inspect the `GoSourceFunction` or `GoMethodValue` object received from `minigo` and use its properties (package path, name) to dynamically construct the internal matching key.

## 4. Revised Testing Strategy

The testing strategy is to **validate the library's robustness** by making the original, file-based test case work correctly. We will *not* use workarounds like programmatic injection.

-   **`docgen` Integration Test (`examples/docgen/main_test.go`)**:
    -   Create an integration test that uses a file-based, nested Go module in `testdata`. This test will have its own `go.mod` with a `replace` directive.
    -   This test will fail initially due to the module resolution bug.
    -   The test will be used to validate the fix in the `go-scan`/`locator` packages. Once the core library is fixed, this test should pass without any special configuration, proving the solution is robust.

## 5. High-Level Task List

-   **Core Library Robustness**:
    -   [ ] **Enhance Module Resolution**: Fix the `go-scan` locator to correctly resolve packages in nested test modules with `replace` directives.
    -   [ ] **Implement Typed Nil Method Values**: Enhance the `minigo` interpreter to support resolving method values from typed `nil` pointers.
    -   [ ] **Implement Environment-Aware Function Objects**: Enhance `minigo` to represent Go functions as objects that retain their definition environment (`DefEnv`).

-   **`docgen` Feature**:
    -   [ ] **Refactor `docgen` Configuration**: Update `PatternConfig` to use a type-safe `Fn` field instead of a string `Key`.
    -   [ ] **Implement Key Computation**: Update the `docgen` loader to dynamically generate the matching key from the `Fn` reference.

-   **Verification**:
    -   [ ] **Validate with Integration Test**: Create and pass a `docgen` integration test that uses a nested Go module, proving the module resolution fix and the `docgen` feature work together correctly.
