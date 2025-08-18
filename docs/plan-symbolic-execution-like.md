# Plan: A Symbolic-Execution-like Engine for Go (`net/http`)

This document outlines a revised plan for creating `symgo`, a library for symbolic code analysis, and a tool, `docgen`, that uses it to generate OpenAPI documentation from standard `net/http` applications.

This plan incorporates feedback to focus solely on `net/http` for the initial implementation and to establish a clear architectural separation between the generic engine (`symgo`) and the specific tool (`docgen`).

## 1. Goals & Architecture

*   **`symgo` (The Generic Library):** A reusable library that provides the core AST interpretation engine.
    *   It will traverse the Go AST and manage symbolic state.
    *   It will have no built-in knowledge of `net/http` or any other framework.
    *   It will provide a mechanism for users to register "intrinsic handlers" for specific functions. This allows consumers to teach `symgo` how to handle framework-specific calls.

*   **`examples/docgen` (The `net/http` Tool):** A standalone application that uses `symgo` to achieve a specific goal.
    *   It will import `symgo`.
    *   It will define and register intrinsic handlers for `net/http` functions like `http.HandleFunc` and `http.ListenAndServe`.
    *   It will contain all logic for extracting API metadata (paths, handlers, request/response shapes) from `net/http` code.
    *   It will be responsible for formatting the extracted metadata into an OpenAPI specification.

## 2. Phased Implementation Plan

This plan is broken down into distinct phases, from foundational work to application-specific features.

### Phase 1: Foundational `symgo` Engine (The Generic Interpreter)

*   **Task:** Build the core, framework-agnostic AST evaluation engine.
*   **Deliverables:**
    1.  `symgo/` directory with the initial code.
    2.  `object.go`: Define the `Object` interface and basic symbolic types (`String`, `Integer`, etc.).
    3.  `scope.go`: Implement the `Scope` for variable tracking.
    4.  `evaluator.go`: Implement the main `Evaluator`. It will handle basic Go expressions and statements (`ast.AssignStmt`, `ast.Ident`, literals).
    5.  `intrinsics.go`: Implement a registry for intrinsic function handlers (`map[string]IntrinsicFunc`). The `Evaluator` will consult this registry when it encounters a `CallExpr`. Add a public `RegisterIntrinsic` method.
    6.  Basic tests for the evaluator's handling of generic language constructs.

### Phase 2: `docgen` Tool & Basic `net/http` Route Extraction

*   **Task:** Create the `docgen` tool and use it to extract basic route information.
*   **Deliverables:**
    1.  `examples/docgen/` directory.
    2.  `examples/docgen/main.go`: The main application.
    3.  **Inside `main.go`:**
        *   Instantiate the `symgo.Evaluator`.
        *   Define a custom handler function for `net/http.HandleFunc`.
        *   Register this handler with the evaluator: `evaluator.RegisterIntrinsic("net/http.HandleFunc", myHandleFuncHandler)`.
        *   The `myHandleFuncHandler` will inspect the call's arguments, extract the path (a `SymbolicString`) and the handler (a `SymbolicFunction`), and store this pair in a custom struct.
    4.  `examples/docgen/sampleapi/`: A new directory containing a simple `main.go` with a few `http.HandleFunc` calls. This will be the target for analysis.
    5.  A test that runs the `docgen` tool on the `sampleapi` and asserts that the correct number of routes with the correct paths are extracted.

### Phase 3: Deep Handler Analysis for API Schemas

*   **Task:** Analyze the body of each discovered HTTP handler function to find request and response types.
*   **Deliverables (within `examples/docgen`):**
    1.  A new analysis step that takes a `SymbolicFunction` (the handler).
    2.  This step will walk the handler's AST (`*ast.BlockStmt`).
    3.  It will look for specific patterns:
        *   `json.NewDecoder(r.Body).Decode(&req)`: From this, it will resolve the type of the `req` variable to determine the request body schema.
        *   `json.NewEncoder(w).Encode(resp)`: Similarly, it will resolve the type of `resp` for the response schema.
    4.  The data structure holding the extracted API information will be updated to include these request/response types.
    5.  Tests for this deep analysis logic.

### Phase 4: Applied Enhancements (Handling Real-World Code)

*   **Task:** Improve the engine's ability to understand more complex, but common, code patterns.
*   **Deliverables:**
    1.  **Helper Function Support:** Ensure the `symgo` engine's call stack and scope management correctly handle cases where `http.HandleFunc` is called from within a helper function. This should be a natural outcome of the core evaluator design, but it must be explicitly tested.
    2.  **`fmt.Sprintf` and String Concatenation:**
        *   In `symgo`, add a built-in intrinsic handler for `fmt.Sprintf`. This handler will be a "best-effort" implementation. It will evaluate the format string and replace format verbs (`%s`, `%d`) with generic placeholders like `{param1}`. The result will be a `SymbolicString`.
        *   Enhance the `symgo` evaluator to handle the `+` operator for `SymbolicString` types.
        *   Add tests in `examples/docgen` with a `sampleapi` that constructs its routes using `fmt.Sprintf` to verify this works.

### Phase 5: OpenAPI Generation and Finalization

*   **Task:** Convert the collected API metadata into a valid OpenAPI 3.0 document.
*   **Deliverables (within `examples/docgen`):**
    1.  A component that takes the final, structured API data.
    2.  Logic to iterate through this data and build up an OpenAPI struct (using a library like `github.com/getkin/kin-openapi`).
    3.  Code to marshal this struct into a YAML or JSON string and print it to standard output.
    4.  A final end-to-end test that runs the entire analysis and compares the output against a golden OpenAPI file.
    5.  Run `make format` and `make test` across the whole project.
    6.  Submit the final, working solution.
