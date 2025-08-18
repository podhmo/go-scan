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

### 3.1. Selective Interpretation & Complexity Management

Real-world applications contain logic (logging, auth, metrics) not relevant to the API's shape. The engine must intelligently ignore this "noise."

*   **Strategy 1: Unknown Functions are No-ops:** By default, if the `symgo` evaluator encounters a function call that is **not** a registered intrinsic, it will treat the call as a no-op and continue analysis. This is the primary mechanism for ignoring irrelevant code like `log.Printf(...)` and is a core configurable behavior of the engine.

*   **Strategy 2: Stubbing Complex Types with Overrides:** Fully analyzing standard library types like `http.Request` is unnecessary and complex.
    *   **`go-scan` Feature:** We will leverage `go-scan`'s `WithExternalTypeOverrides` feature.
    *   **Implementation:** The `docgen` tool will provide `symgo` with a "synthetic" definition for types like `http.Request`. This stub will only expose the fields and methods we care about (e.g., `URL`, `Method`, `Body`), dramatically reducing the analysis scope.

### 3.2. Recursive Evaluation of Helper Functions

When analysis requires entering a helper function (e.g., a function that wraps `http.HandleFunc`), the engine will proceed as follows:

1.  **Function Call Encountered:** The evaluator sees a `CallExpr` to a function it does not have an intrinsic for, e.g., `myApp.RegisterRoute(...)`.
2.  **AST Loading (`go-scan`):** The engine uses `go-scan` to lazily find and parse the AST for `myApp.RegisterRoute`. `go-scan` automatically handles cross-package resolution within the same Go module. The use of `goscan.WithGoModuleResolver()` will allow resolution of stdlib packages as well.
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
