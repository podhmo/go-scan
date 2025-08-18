# Plan: A Symbolic-Execution-like Engine for Go (`net/http`)

This document outlines a revised and detailed plan for creating `symgo`, a library for symbolic code analysis, and a tool, `docgen`, that uses it to generate OpenAPI 3.1 documentation from standard `net/http` applications.

This final version incorporates multiple rounds of feedback to provide a deep and practical design.

## 1. Goals & Architecture

*   **`symgo` (The Generic Library):** A reusable, framework-agnostic AST interpretation engine. Its core responsibility is to traverse Go code, manage symbolic state, and provide hooks for customization.
*   **`examples/docgen` (The `net/http` Tool):** A consumer of the `symgo` library. It contains all logic specific to `net/http` and OpenAPI generation. It teaches the `symgo` engine how to understand an `net/http` application by registering custom handlers and analysis patterns.

## 2. Phased Implementation Plan

(Phases remain conceptually the same, but their implementation will be guided by the detailed task list in the final section.)

1.  **Phase 1: Foundational `symgo` Engine.**
2.  **Phase 2: `docgen` Tool & Basic `net/http` Route Extraction.**
3.  **Phase 3: Deep Handler Analysis for API Schemas.**
4.  **Phase 4: Applied Enhancements (Helper Functions, `fmt.Sprintf`).**
5.  **Phase 5: OpenAPI Generation and Finalization.**

## 3. Core Engine Design Principles

This section details the strategies for handling the complexities of analyzing real-world Go code.

### 3.1. Evaluation Strategy: Intra-Module vs. Extra-Module

The evaluation strategy for a function call follows this priority order:

1.  **Registered Intrinsics (Highest Priority):** If a function call matches a registered intrinsic (e.g., `http.HandleFunc`), the intrinsic handler is executed.
2.  **Intra-Module Calls (Recursive Evaluation):** If a function is not an intrinsic but is defined within the current Go module, the engine will default to trusting it and evaluate it recursively.
3.  **Extra-Module Calls (Symbolic Placeholder):** If a function is external (stdlib, third-party) and not an intrinsic, the engine will not evaluate it. It will return a symbolic placeholder. This is critical for performance.
4.  **Stubbing Complex Types:** To interact with essential external types like `http.Request`, we will use `go-scan`'s `WithExternalTypeOverrides` feature to provide simplified "stub" definitions.

### 3.2. Recursive Evaluation of Helper Functions

When analysis requires entering an intra-module helper function, the engine will:
1.  Use `go-scan` to lazily find and parse the function's AST.
2.  Create a new `Scope` for the function call, inspired by `minigo`'s lexical scoping.
3.  Map call arguments to the function's parameters in the new scope.
4.  Call the evaluator recursively on the function's body.

### 3.3. Extensible Pattern Matching

To avoid brittle, hardcoded analysis, `docgen` will use a configurable registry of "Pattern Analyzers." This allows analysis rules (e.g., how to find a query parameter) to be defined as data, making the tool adaptable to project-specific helper functions.

## 4. Detailed Design and Code-to-Spec Mapping

(This section remains as previously defined, providing concrete examples.)

## 5. Design Q&A Checklist

This section summarizes the key design decisions clarified during the planning process.

*   **Q: Is the goal of this task to implement the engine or to produce a design document?**
    *   **A:** The goal is exclusively to produce this comprehensive design document.

*   **Q: Which web framework is the initial target?**
    *   **A:** Exclusively `net/http`.

*   **Q: How are `symgo` (engine) and `docgen` (tool) architected?**
    *   **A:** As separate modules. `symgo` is a generic library, `docgen` is a specific consumer.

*   **Q: How will the engine handle code not relevant to the API shape?**
    *   **A:** Through a refined strategy: it evaluates intra-module code, ignores extra-module code (unless an intrinsic is provided), and uses stubs for complex types.

*   **Q: How can the tool be adapted to project-specific coding patterns?**
    *   **A:** Through "Extensible Pattern Matching," a configurable registry of code patterns that `docgen` will search for.

## 6. Handling Interfaces and Higher-Order Functions

To handle interfaces (like `io.Writer`) and higher-order functions (like `http.HandlerFunc`), the engine cannot know the concrete type at all times. The solution is configurable type binding.

*   **Design:** The `symgo` evaluator will be launched with a "context" or "environment" object. This object will contain a map of interface types to the concrete symbolic types they should be treated as for that specific analysis run.
*   **Example (Interface):** Before analyzing an `http` handler, `docgen` will configure the context: `context.Bind("io.Writer", symbolicResponseWriter)`. When the evaluator encounters a variable of type `io.Writer`, it will consult the context and know to treat it as a `ResponseWriter` object, allowing analysis to continue.
*   **Example (Higher-Order Function):** For a call like `http.TimeoutHandler(myHandler, ...)`, `docgen` will have an intrinsic for `http.TimeoutHandler`. This intrinsic will know that the "real" handler to analyze is the first argument, and it will proceed with analyzing `myHandler`.

## 7. Incremental Implementation Tasks

This section breaks down the project into a granular, actionable task list.

### M1: `symgo` Core Engine
-   [ ] `symgo/object`: Define `Object` interface and initial types (`String`, `Function`, `Error`).
-   [ ] `symgo/scope`: Implement `Scope` struct with `Get`/`Set` and support for enclosing scopes.
-   [ ] `symgo/evaluator`: Implement `Evaluator` struct and `Eval` method dispatcher.
-   [ ] `symgo/evaluator`: Implement evaluation for `ast.BasicLit`, `ast.Ident`.
-   [ ] `symgo/evaluator`: Implement evaluation for `ast.AssignStmt`, `ast.ReturnStmt`.
-   [ ] `symgo/goscan`: Integrate `go-scan` for package loading.
-   [ ] `symgo/evaluator`: Implement function call evaluation for intra-module functions (recursive `Eval`).
-   [ ] `symgo/intrinsics`: Implement registry for intrinsic functions.
-   [ ] `symgo/engine`: Implement logic to handle extra-module calls as symbolic placeholders.

### M2: `docgen` Tool & Basic `net/http` Analysis
-   [ ] `examples/docgen/main.go`: Create skeleton application.
-   [ ] `examples/docgen/openapi`: Define local structs for a minimal OpenAPI 3.1 spec.
-   [ ] `examples/docgen/sampleapi`: Create a simple `net/http` server to act as the analysis target.
-   [ ] `examples/docgen/analyzer`: Implement the main analysis orchestrator.
-   [ ] `examples/docgen/analyzer`: Define and register an intrinsic for `http.HandleFunc` that extracts path and handler function.
-   [ ] `examples/docgen/analyzer`: Implement analysis of handler function's AST to find `switch r.Method` statements.
-   [ ] `examples/docgen/analyzer`: Implement extraction of godoc comments from handler `FuncDecl` for `description`.
-   [ ] **Test:** Write an integration test that runs the analyzer on `sampleapi` and confirms routes and descriptions are extracted.

### M3: Schema and Parameter Analysis
-   [ ] `examples/docgen/analyzer`: Implement pattern matching for `json.NewDecoder(...).Decode(&var)`.
-   [ ] `examples/docgen/analyzer`: Implement logic to resolve the `var` type and its struct fields/tags.
-   [ ] `examples/docgen/analyzer`: Implement pattern matching for `json.NewEncoder(...).Encode(var)`.
-   [ ] `examples/docgen/analyzer`: Implement logic to resolve the response `var` type.
-   [ ] `examples/docgen/patterns`: Implement the configurable `CallPattern` registry for query parameters.
-   [ ] **Test:** Enhance integration test to verify request/response schemas and parameters are correctly identified.

### M4: Finalization
-   [ ] `examples/docgen/generator`: Implement the logic to convert the collected metadata into the local OpenAPI structs.
-   [ ] `examples/docgen/generator`: Implement YAML/JSON marshaling to print the final spec.
-   [ ] Add support for `fmt.Sprintf` in `symgo` as a built-in intrinsic.
-   [ ] Write final end-to-end test comparing output to a golden file.
-   [ ] Write `README.md` for `symgo` and `examples/docgen`.
-   [ ] Run `make format` and `make test` for the entire repository.
