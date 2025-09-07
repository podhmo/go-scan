# `symgo` Evaluator: Design Summary

This document provides a consolidated design summary of the `symgo` evaluator, incorporating both the original design principles from `docs/plan-symbolic-execution-like.md` and the observed implementation details from the source code.

## 1. Core Philosophy and Purpose

`symgo` is a **symbolic execution-like engine**, not a general-purpose Go interpreter. Its primary purpose is to perform static analysis on Go codebases by tracing function calls, resolving types, and tracking the flow of symbolic values. It is designed to be the foundation for tools that analyze code structure and behavior, such as `docgen` (API documentation generation) and `find-orphans` (unused code detection).

The engine prioritizes **robustness, performance, and analytical utility** over precise simulation of Go's runtime semantics.

## 2. Module, Import, and Evaluation Strategy

The engine's strategy for evaluation is intertwined with how it handles modules and imports, ensuring analysis remains tractable and focused. It uses a dedicated `Resolver` component to manage package loading and analysis scope.

1.  **On-Demand Package Loading**: Packages are not loaded upfront. When the evaluator encounters an identifier for an imported package, it requests it from the `Resolver`. The `Resolver` uses `go-scan` to find and parse the package's source files, and the result is cached to prevent duplicate work.

2.  **Configurable Scan Policy**: The `Resolver` is configured with a `ScanPolicyFunc`. This function determines whether a given package import path is "in-policy" or "out-of-policy." This mechanism drives the core evaluation strategy:
    - **In-Policy (Recursive Evaluation)**: If a function call belongs to a package that is in-policy (typically the user's own workspace), the engine will recursively evaluate the function's body.
    - **Out-of-Policy (Symbolic Placeholder)**: If a function call belongs to an out-of-policy package (e.g., the standard library or a third-party dependency), the engine **does not** evaluate its body. It returns a `SymbolicPlaceholder` that represents the function's result, typed according to its signature.

3.  **Intrinsics (Highest Priority)**: The engine also maintains a registry of "intrinsic" handlers for specific, well-known functions (e.g., `net/http.HandleFunc`, `make`). These take precedence over the scan policy, allowing tools to provide custom models for important external functions without needing to analyze their source code.

## 3. Control Flow Handling

`symgo`'s handling of control flow is deliberately pragmatic and tailored for static analysis.

-   **`if`/`else` Statements**: The engine does not attempt to evaluate the condition to decide which branch to take. Instead, to discover all possible code paths, it **evaluates both the `if` block and the `else` block**. Each branch is evaluated in an isolated scope.

-   **`switch` Statements**: Following the same principle, the engine evaluates the body of **all `case` clauses** in a `switch` or `type switch` statement. This ensures complete discovery of all potential code paths and type usages.

-   **`for` and `for...range` Loops**: To avoid the halting problem and the complexity of analyzing loops to completion, the engine uses a **bounded analysis** strategy. It "unrolls" the loop body **exactly once**. This is sufficient to discover function calls and other patterns within the loop without getting stuck in infinite iterations.

## 4. State, Scope, and Assignment Model

The engine's model for state and scope is fundamental to its operation and is designed for analytical accuracy and performance.

-   **Lexical Scoping**: The engine correctly models Go's lexical scoping. Every block (`{...}`), control flow statement, or function call creates a new, enclosed environment. This ensures that variable lookups and assignments (`=` vs. `:=`) correctly resolve to the nearest variable in scope, naturally handling shadowing.

-   **Lazy Variable Evaluation**: For performance, package-level variables are evaluated lazily. Their initializer expressions are stored but not evaluated until the variable is first accessed.

-   **Hybrid Assignment**: The assignment model is a hybrid tailored for static analysis:
    -   **Default (In-Place Update)**: For most assignments (`v = ...`), the engine finds the variable `v` in its defining scope and updates its value in-place.
    -   **Interface (Additive Update)**: This is a key innovation. If the static type of `v` is an `interface`, an assignment **adds the concrete type** of the right-hand side to a running set of `PossibleTypes` on the variable object. This allows the engine to aggregate all possible concrete types an interface variable might hold across different control flow paths.

## 5. Function and Call Handling

The engine includes sophisticated handling for function calls to support deep analysis.

- **Lexical Scoping and Closures**: Function objects capture the environment in which they are defined. When a function is called, its execution scope is created as a child of this captured "definition" environment, not the "caller's" environment. This correctly models Go's lexical scoping and allows closures to access variables from their enclosing functions.

- **Anonymous Functions as Arguments**: The engine has a special heuristic to immediately "pre-scan" the body of any anonymous function passed as an argument to another call. This allows it to discover usages inside the anonymous function, which is critical for cases like `t.Run("name", func(t *testing.T) { ... })`, where the call to `t.Run` itself may not be deeply analyzed.

## 6. Method Resolution

The engine correctly models Go's rules for method resolution to support accurate call graph tracing.

- **Embedded Methods**: The engine performs a recursive, depth-first search to find methods on embedded types. This correctly simulates Go's method promotion, allowing calls like `myStruct.EmbeddedMethod()` to be resolved correctly.

- **Interface Methods**: Calls on interface-typed variables are handled symbolically. The engine does not perform dynamic dispatch. Instead, it traces the call to the method on the interface definition itself and produces a symbolic placeholder for the result, typed according to the interface method's signature. This provides the necessary information for analysis tools without the complexity of tracking all possible concrete implementations at the call site.

## 7. Key Use Cases and Driving Requirements

The design of `symgo` is directly driven by the needs of its consumer tools.

-   **`docgen`**: This tool requires `symgo` to trace calls from an HTTP server entry point, identify routing patterns (like `mux.HandleFunc`), and analyze handler logic to find request/response types. The intrinsic system and recursive evaluation are key to this.

-   **`find-orphans`**: This tool requires `symgo` to build a comprehensive call graph of an entire application or library. The "additive update" model for interfaces is critical for accurately tracking which interface methods are implemented and called. The policy-based evaluation (intra- vs. extra-module) is essential for performance and for scoping the analysis to the user's own code. The robust recursion and loop handling ensures the analysis always terminates.
