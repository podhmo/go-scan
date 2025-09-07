# `symgo` Evaluator: Design Summary

This document provides a consolidated design summary of the `symgo` evaluator, incorporating both the original design principles from `docs/plan-symbolic-execution-like.md` and the observed implementation details from the source code.

## 1. Core Philosophy and Purpose

`symgo` is a **symbolic execution-like engine**, not a general-purpose Go interpreter. Its primary purpose is to perform static analysis on Go codebases by tracing function calls, resolving types, and tracking the flow of symbolic values. It is designed to be the foundation for tools that analyze code structure and behavior, such as `docgen` (API documentation generation) and `find-orphans` (unused code detection).

The engine prioritizes **robustness, performance, and analytical utility** over precise simulation of Go's runtime semantics.

## 2. Evaluation Strategy

The engine employs a multi-tiered strategy to decide how to handle a function call, ensuring that analysis remains tractable.

1.  **Intrinsics (Highest Priority)**: The engine maintains a registry of "intrinsic" handlers for specific, well-known functions (e.g., `net/http.HandleFunc`, `make`). These custom handlers provide framework-specific knowledge or model the behavior of built-in functions, giving the analysis tool fine-grained control.

2.  **Intra-Module Calls (Recursive Evaluation)**: For function calls defined within the primary analysis scope (i.e., the user's own workspace or module), the engine defaults to recursively evaluating the function's body. This allows for deep tracing of the application's own logic.

3.  **Extra-Module Calls (Symbolic Placeholder)**: For function calls to external dependencies (standard library, third-party packages) that do not have a registered intrinsic, the engine **does not** evaluate their bodies. Instead, it returns a `SymbolicPlaceholder` object. This placeholder represents the result of the call and carries the type information derived from the function's signature. This is a critical design choice that prevents the engine from getting lost in the vast call graph of external code, ensuring analysis remains fast and focused.

## 3. Control Flow Handling

`symgo`'s handling of control flow is deliberately pragmatic and tailored for static analysis.

-   **`if`/`else` Statements**: The engine does not attempt to evaluate the condition to decide which branch to take. Instead, to discover all possible code paths, it **evaluates both the `if` block and the `else` block**. Each branch is evaluated in an isolated scope.

-   **`for` and `for...range` Loops**: To avoid the halting problem and the complexity of analyzing loops to completion, the engine uses a **bounded analysis** strategy. It "unrolls" the loop body **exactly once**. This is sufficient to discover function calls and other patterns within the loop without getting stuck in infinite iterations.

## 4. State and Assignment Model

The engine's model for variable assignment is a hybrid designed to maximize analytical insight.

-   **Default Behavior (In-Place Update)**: For most assignments (`v = ...`), the engine finds the variable `v` in its defining scope and updates its value in-place.

-   **Interface Behavior (Additive Update)**: This is a key innovation of `symgo`. If the static type of the variable `v` is an `interface`, an assignment does not simply replace its value. Instead, it **adds the concrete type** of the right-hand side to a running set of `PossibleTypes` on the variable object. This allows the engine to aggregate all possible concrete types that an interface variable could hold across different control flow paths (e.g., from the `if` and `else` branches of a conditional). This is essential for the `find-orphans` tool to correctly determine which interface methods are actually used.

## 5. Key Use Cases and Driving Requirements

The design of `symgo` is directly driven by the needs of its consumer tools.

-   **`docgen`**: This tool requires `symgo` to trace calls from an HTTP server entry point, identify routing patterns (like `mux.HandleFunc`), and analyze handler logic to find request/response types. The intrinsic system and recursive evaluation are key to this.

-   **`find-orphans`**: This tool requires `symgo` to build a comprehensive call graph of an entire application or library. The "additive update" model for interfaces is critical for accurately tracking which interface methods are implemented and called. The policy-based evaluation (intra- vs. extra-module) is essential for performance and for scoping the analysis to the user's own code. The robust recursion and loop handling ensures the analysis always terminates.
