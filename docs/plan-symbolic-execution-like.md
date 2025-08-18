# Plan: A Symbolic-Execution-like Engine for Go (`net/http`)

This document outlines a revised and detailed plan for creating `symgo`, a library for symbolic code analysis, and a tool, `docgen`, that uses it to generate OpenAPI 3.1 documentation from standard `net/http` applications.

This final version incorporates multiple rounds of feedback to provide a deep and practical design.

## 1. Goals & Architecture

*   **`symgo` (The Generic Library):** A reusable, framework-agnostic AST interpretation engine. Its core responsibility is to traverse Go code, manage symbolic state, and provide hooks for customization.
*   **`examples/docgen` (The `net/http` Tool):** A consumer of the `symgo` library. It contains all logic specific to `net/http` and OpenAPI generation. It teaches the `symgo` engine how to understand an `net/http` application by registering custom handlers and analysis patterns.

## 2. Phased Implementation Plan

(Phases remain conceptually the same, but their implementation will be guided by the detailed design below.)

1.  **Phase 1: Foundational `symgo` Engine.**
2.  **Phase 2: `docgen` Tool & Basic `net/http` Route Extraction.**
3.  **Phase 3: Deep Handler Analysis for API Schemas.**
4.  **Phase 4: Applied Enhancements (Helper Functions, `fmt.Sprintf`).**
5.  **Phase 5: OpenAPI Generation and Finalization.**

## 3. Core Engine Design Principles

This section details the strategies for handling the complexities of analyzing real-world Go code.

### 3.1. Evaluation Strategy: Intra-Module vs. Extra-Module

To analyze real-world code without getting lost, the engine must differentiate between trusted, project-specific code and external dependencies. The "override everything" approach is not practical. Instead, the evaluation strategy for a function call follows this priority order:

1.  **Registered Intrinsics (Highest Priority):** If a function call matches a registered intrinsic (e.g., `http.HandleFunc`, or a special-cased `fmt.Sprintf`), the intrinsic handler is executed. This is the primary mechanism for teaching the engine about framework specifics.

2.  **Intra-Module Calls (Recursive Evaluation):** If a function is **not** an intrinsic but is defined **within the current Go module**, the engine will default to trusting it. It will use `go-scan` to find its source and evaluate it recursively. This allows the analysis to trace through any and all user-defined helper functions.

3.  **Extra-Module Calls (Symbolic Placeholder):** If a function is external (in the standard library or a third-party dependency) and is not a registered intrinsic, the engine will **not** attempt to evaluate it. Instead, it will return a symbolic placeholder (e.g., an `ExternalCall` object). This prevents the engine from parsing the entire Go standard library and vendor dependencies, which is critical for performance and stability.

4.  **Stubbing Complex Types:** To interact with external types like `http.Request`, we will use `go-scan`'s `WithExternalTypeOverrides` feature to provide simplified "stub" definitions, rather than analyzing the real, complex struct. This is a targeted way to handle essential external types without evaluating their packages.

### 3.2. Recursive Evaluation of Helper Functions

When analysis requires entering an intra-module helper function, the engine will proceed as follows:

1.  **Function Call Encountered:** The evaluator sees a `CallExpr` to a function it does not have an intrinsic for, e.g., `myApp.RegisterRoute(...)`.
2.  **AST Loading (`go-scan`):** Because the function is within the current module, the engine uses `go-scan` to lazily find and parse its AST. The use of `goscan.WithGoModuleResolver()` will aid in resolving package paths.
3.  **Scope Creation (`minigo` inspiration):** A new `Scope` is created for the `RegisterRoute` function call, inheriting from the caller's scope. This follows the classic interpreter lexical scoping model used in `minigo`.
4.  **Argument Mapping:** The arguments from the `CallExpr` are evaluated and mapped to the parameter names in the new scope.
5.  **Recursive Evaluation:** The evaluator is called recursively on the body of the `RegisterRoute` function. Inside this function, it will eventually encounter the `http.HandleFunc` call, which *is* an intrinsic, allowing `docgen` to capture the route.

### 3.3. Extensible Pattern Matching

Hardcoding analysis patterns is brittle. The `docgen` tool must be adaptable to project-specific coding styles.

*   **Design:** `docgen` will define and maintain a registry of "Pattern Analyzers." These are configurable, code-based rules that know how to find specific pieces of information.
*   **Example: Configurable Parameter Extraction:** Instead of hardcoding a search for `r.URL.Query().Get`, `docgen` will define a pattern. This allows users of `docgen` to add new patterns for their custom helpers.
    ```go
    // In docgen's configuration/setup
    type CallPattern struct {
        TargetFunc string // e.g., "net/http.Request.URL.Query.Get"
        ArgIndex   int    // Which argument has the info we want
    }
    // The docgen tool will use a list of these to find parameters.
    parameterPatterns := []CallPattern{
        {TargetFunc: "net/http.Request.URL.Query.Get", ArgIndex: 0},
        // A user could add a pattern for a custom project helper:
        // {TargetFunc: "myproject/utils.GetQueryParam", ArgIndex: 1},
    }
    ```

## 4. Detailed Design and Code-to-Spec Mapping

This section provides concrete examples of the end-to-end analysis process.

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
    *   **A:** The goal is exclusively to produce this comprehensive design document. The implementation is a separate, future task.

*   **Q: Which web framework is the initial target?**
    *   **A:** The initial target is exclusively `net/http`. The engine (`symgo`) will be generic, but the first tool (`docgen`) will be `net/http`-specific.

*   **Q: How should `symgo` (the engine) and `docgen` (the tool) be architected?**
    *   **A:** They must be separate. `symgo` is a generic AST interpretation library. `docgen` is a specific application that imports `symgo` and contains all the `net/http` and OpenAPI-specific logic.

*   **Q: Should an external library be used for OpenAPI struct definitions?**
    *   **A:** No. To minimize dependencies, the necessary OpenAPI structs will be defined locally within the `docgen` tool.

*   **Q: How will the engine handle application code not relevant to the API shape (e.g., logging, auth)?**
    *   **A:** Through a refined, multi-layered strategy. It prioritizes registered intrinsics, then recursively evaluates all calls within the same Go module, and treats calls to external dependencies as symbolic placeholders, effectively ignoring them. This provides a robust balance between deep analysis and practical performance.

*   **Q: How will the engine trace calls through helper functions, including those in other packages?**
    *   **A:** The engine will perform recursive evaluation for any function within the same Go module. It will use `go-scan` to lazily load the AST for any called function and analyze its body in a new, correctly-scoped context, inspired by `minigo`'s design.

*   **Q: How can the tool be adapted to project-specific coding patterns (e.g., custom parameter-fetching helpers)?**
    *   **A:** Through "Extensible Pattern Matching." `docgen` will not have hardcoded patterns but will use a configurable registry of "Pattern Analyzers," allowing analysis rules to be defined as data or code.
