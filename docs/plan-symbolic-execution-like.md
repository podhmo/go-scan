# Plan: A Symbolic-Execution-like Engine for Go (`net/http`)

This document outlines a revised and detailed plan for creating `symgo`, a library for symbolic code analysis, and a tool, `docgen`, that uses it to generate OpenAPI 3.1 documentation from standard `net/http` applications.

This final version incorporates multiple rounds of feedback to provide a deep and practical design.

## 1. Goals & Architecture

*   **`symgo` (The Generic Library):** A reusable, framework-agnostic AST interpretation engine. Its core responsibility is to traverse Go code, manage symbolic state, and provide hooks for customization.
*   **`examples/docgen` (The `net/http` Tool):** A consumer of the `symgo` library. It contains all logic specific to `net/http` and OpenAPI generation. It teaches the `symgo` engine how to understand an `net/http` application by registering custom handlers and analysis patterns.

## 2. Phased Implementation Plan

1.  **Phase 1: Foundational `symgo` Engine.** Build the core, framework-agnostic AST evaluation engine. This includes the `Evaluator`, `Scope`, `Object` system, and a registry for "intrinsic" functions.
2.  **Phase 2: `docgen` Tool & Basic `net/http` Route Extraction.** Create the `docgen` tool in `examples/docgen`. Implement and register a custom intrinsic handler for `net/http.HandleFunc` to extract basic route paths and handler functions from a sample API.
3.  **Phase 3: Deep Handler Analysis for API Schemas.** Enhance `docgen` to analyze the body of handler functions. It will look for patterns like `json.Decode` and `json.Encode` to determine the request and response object types.
4.  **Phase 4: Applied Enhancements.** Improve the `symgo` engine to handle more complex, real-world code. This includes ensuring helper functions are traced correctly and adding support for path construction via `fmt.Sprintf` and string concatenation.
5.  **Phase 5: OpenAPI Generation and Finalization.** In `docgen`, convert the collected API metadata into a valid OpenAPI 3.1 YAML/JSON document. Add end-to-end tests, run `make format` and `make test`, and prepare for submission.

## 3. Core Engine Design Principles

This section details the strategies for handling the complexities of analyzing real-world Go code.

### 3.1. Evaluation Strategy: Intra-Module vs. Extra-Module

The evaluation strategy for a function call follows this priority order:

1.  **Registered Intrinsics (Highest Priority):** If a function call matches a registered intrinsic (e.g., `http.HandleFunc`), the intrinsic handler is executed.
2.  **Intra-Module Calls (Recursive Evaluation):** If a function is **not** an intrinsic but is defined **within the current Go module**, the engine will default to trusting it and evaluate it recursively.
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

### 3.4. Handling Control Flow (`if`, `for`)

A full symbolic execution of all control flow paths is computationally expensive and often unnecessary for static analysis tools like `docgen`. `symgo` will adopt a pragmatic, heuristic-based approach.

*   **Conditional Statements (`if`, `switch`):**
    The primary goal is not to prove a single path is taken, but to discover what *could* happen in any branch. Instead of complex state forking and path constraint analysis, the engine will favor a simpler traversal model. For example, when analyzing an `http.HandlerFunc` that uses a `switch r.Method` block, the analyzer will simply inspect the AST of each `case` block sequentially to collect the patterns for `GET`, `POST`, etc. This approach gathers all potential behaviors without the overhead of simulating a true execution fork.

*   **Loops (`for`):**
    Loops present a halting problem and cannot be analyzed to completion in the general case. The engine will use a **bounded analysis** strategy:
    1.  **Limited Unrolling (Default):** By default, the engine will "unroll" a loop once. This means it will evaluate the body of the loop a single time to discover important function calls or patterns within it (e.g., decoding a single item from a request stream).
    2.  **Symbolic Generalization (Fallback):** After the single iteration (or if the loop is skipped entirely), any variables assigned or modified within the loop body will be treated as `SymbolicPlaceholder`s. This correctly marks their state as indeterminate after the loop, preventing the engine from making unsound assumptions.

This strategy balances the need to inspect code within control flow structures with the practical limitations of static analysis, ensuring the engine remains fast and fit for its purpose of pattern extraction.

## 4. Detailed Design and Code-to-Spec Mapping

This section provides concrete examples of the end-to-end analysis process.

### Example Target Go Code

```go
// In main.go
package main

import (
	"encoding/json"
	"net/http"
)

// CreateUserRequest represents the request body for creating a user.
type CreateUserRequest struct {
	Name string `json:"name"` // The user's name
}

// User represents a user in our system.
type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// UserHandler handles requests for the /users endpoint.
// It supports GET to list users and POST to create a new user.
func UserHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// GET /users?limit=10
		_ = r.URL.Query().Get("limit") // Parameter extraction

		// Response
		users := []User{{ID: 1, Name: "Alice"}}
		json.NewEncoder(w).Encode(users)

	case http.MethodPost:
		// Request Body
		var req CreateUserRequest
		json.NewDecoder(r.Body).Decode(&req)

		// Response
		user := User{ID: 2, Name: req.Name}
		json.NewEncoder(w).Encode(user)
	}
}

func main() {
	http.HandleFunc("/users", UserHandler)
	http.ListenAndServe(":8080", nil)
}
```

### Mapping Details

#### 4.1. Path, Method, and Operation

*   **Go Code:** `http.HandleFunc("/users", myapp.UserHandler)`
*   **Extraction Logic:**
    1.  `docgen` registers an intrinsic for `http.HandleFunc`.
    2.  `symgo` executes the intrinsic, extracting the path `/users` and the symbolic function for `myapp.UserHandler`.
    3.  `docgen` analyzes the `UserHandler`'s AST, looking for `switch r.Method` blocks to find the handled HTTP methods (GET, POST).
*   **Final OpenAPI Snippet:** `paths: { /users: { get: ..., post: ... } }`

#### 4.2. Description and OperationID

*   **Go Code:** `// UserHandler handles user requests. \n func UserHandler(...)`
*   **Extraction Logic:**
    1.  The `OperationID` is derived from the function name (`UserHandler_GET`).
    2.  The `Description` is extracted from the `Doc` field of the function's `*ast.FuncDecl`.
    3.  **`go-scan` Note:** This relies on `go-scan` providing the full AST node with its documentation.
*   **Final OpenAPI Snippet:** `get: { operationId: UserHandler_GET, description: "UserHandler handles user requests." }`

#### 4.3. Request Body

*   **Go Code:** `utils.DecodeJSON(r, &req)` where `req` is `CreateUserRequest`.
*   **Extraction Logic:**
    1.  The engine encounters `utils.DecodeJSON`, which is not an intrinsic. It recursively evaluates it (per section 3.2).
    2.  Inside `DecodeJSON`, it finds the `json.NewDecoder(...).Decode(...)` call.
    3.  The `docgen` tool has a registered pattern analyzer for this call. The analyzer finds the variable passed to `Decode` (`&req`).
    4.  **`minigo` Note:** Using the `minigo`-style scope, it resolves `req` to its type, `CreateUserRequest`.
    5.  It then analyzes the `CreateUserRequest` struct definition to build the schema.
*   **Final OpenAPI Snippet:** `post: { requestBody: { content: { application/json: { schema: { $ref: '#/components/schemas/CreateUserRequest' } } } } }`

#### 4.4. Query Parameters

*   **Go Code:** `name := GetQuery(r, "name")`
*   **Extraction Logic:**
    1.  The `docgen` AST walker applies its registered `CallPattern` list to the handler's code.
    2.  It finds a match with a user-defined pattern: `{TargetFunc: "myapp/utils.GetQuery", ArgIndex: 1}`.
    3.  It extracts the parameter name "name" from the second argument.
    4.  It defaults the type to `string`, as type inference is a complex extension.
*   **Final OpenAPI Snippet:** `get: { parameters: [ { name: name, in: query, schema: { type: string } } ] }`

#### 4.5. Responses

*   **Go Code:** `render.JSON(w, 200, users)` where `users` is `[]User`.
*   **Extraction Logic:**
    1.  `docgen` registers an intrinsic for its `render.JSON` helper.
    2.  The intrinsic is executed. It identifies the second argument (`200`) as the status code and the third argument (`users`) as the response body.
    3.  It resolves the symbolic type of `users` to `[]User` and generates the corresponding schema.
*   **Final OpenAPI Snippet:** `get: { responses: { '200': { description: OK, content: { application/json: { schema: { type: array, items: { $ref: '#/components/schemas/User' } } } } } } }`

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
