# Plan: A Symbolic-Execution-like Engine for Go (`net/http`)

This document outlines a revised plan for creating `symgo`, a library for symbolic code analysis, and a tool, `docgen`, that uses it to generate OpenAPI documentation from standard `net/http` applications.

This plan incorporates feedback to focus solely on `net/http`, establish a clear architectural separation between the generic engine (`symgo`) and the specific tool (`docgen`), and provide a much more detailed design.

## 1. Goals & Architecture

*   **`symgo` (The Generic Library):** A reusable library that provides the core AST interpretation engine.
    *   It will traverse the Go AST and manage symbolic state.
    *   It will have no built-in knowledge of `net/http` or any other framework.
    *   It will provide a mechanism for users to register "intrinsic handlers" for specific functions. This allows consumers to teach `symgo` how to handle framework-specific calls.

*   **`examples/docgen` (The `net/http` Tool):** A standalone application that uses `symgo` to achieve a specific goal.
    *   It will import `symgo`.
    *   It will define and register intrinsic handlers for `net/http` functions like `http.HandleFunc`.
    *   It will contain all logic for extracting API metadata (paths, handlers, request/response shapes) from `net/http` code.
    *   It will be responsible for formatting the extracted metadata into an OpenAPI specification.

## 2. Phased Implementation Plan

This plan is broken down into distinct phases, from foundational work to application-specific features.

### Phase 1: Foundational `symgo` Engine (The Generic Interpreter)
*   **Task:** Build the core, framework-agnostic AST evaluation engine.

### Phase 2: `docgen` Tool & Basic `net/http` Route Extraction
*   **Task:** Create the `docgen` tool and use it to extract basic route information from a sample `net/http` application.

### Phase 3: Deep Handler Analysis for API Schemas
*   **Task:** Enhance `docgen` to analyze the body of handler functions to find request and response object types by looking for patterns like `json.Decode` and `json.Encode`.

### Phase 4: Applied Enhancements (Handling Real-World Code)
*   **Task:** Improve the `symgo` engine to handle more complex, real-world code, such as helper functions and path construction using `fmt.Sprintf`.

### Phase 5: OpenAPI Generation and Finalization
*   **Task:** Convert the collected API metadata into a valid OpenAPI 3.1 document.
*   **Deliverables:**
    1.  A component that takes the final, structured API data.
    2.  Locally defined Go structs that mirror the required OpenAPI 3.1 specification structure.
    3.  Logic to iterate through the collected data and populate these local OpenAPI structs.
    4.  Code to marshal the top-level struct into a YAML or JSON string and print it to standard output.
    5.  A final end-to-end test that runs the entire analysis and compares the output against a golden OpenAPI file.
    6.  Run `make format` and `make test` across the whole project.
    7.  Submit the final, working solution.

## 3. Detailed Design and Code-to-Spec Mapping

This section provides concrete examples of how the `docgen` tool, powered by `symgo`, will analyze Go code to produce an OpenAPI specification.

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

#### 3.1. Path, Method, and Operation

*   **Go Code:** `http.HandleFunc("/users", UserHandler)`
*   **Extraction Logic:**
    1.  The `docgen` tool registers an intrinsic handler for `net/http.HandleFunc`. When `symgo` encounters this call, it executes the intrinsic.
    2.  The intrinsic extracts the path `/users` and the symbolic function for `UserHandler`.
    3.  **`minigo` inspiration:** The resolution of the `UserHandler` identifier to its declaration uses a scope-aware variable lookup, similar to `minigo`'s environment model.
    4.  The `docgen` tool then analyzes the `UserHandler`'s AST, looking for `switch r.Method` or `if r.Method == ...` statements to determine the handled HTTP methods (GET, POST).
*   **Final OpenAPI Snippet:**
    ```yaml
    paths:
      /users:
        get:
          ...
        post:
          ...
    ```

#### 3.2. Description and OperationID

*   **Go Code:** The handler's function declaration and godoc.
    ```go
    // UserHandler handles requests for the /users endpoint.
    // It supports GET to list users and POST to create a new user.
    func UserHandler(w http.ResponseWriter, r *http.Request) { ... }
    ```
*   **Extraction Logic:**
    1.  The `OperationID` is derived from the function name, e.g., `UserHandler`. To ensure uniqueness, it might be combined with the method, e.g., `UserHandler_GET`.
    2.  The `Description` is extracted from the `Doc` field of the function's `*ast.FuncDecl`.
    3.  **`go-scan` utility:** This relies on `go-scan`'s ability to provide the full AST node for any function, including its documentation comments, without parsing unrelated files.
*   **Final OpenAPI Snippet:**
    ```yaml
    get:
      operationId: UserHandler_GET
      description: |
        UserHandler handles requests for the /users endpoint.
        It supports GET to list users and POST to create a new user.
    ```

#### 3.3. Request Body

*   **Go Code:**
    ```go
    var req CreateUserRequest
    json.NewDecoder(r.Body).Decode(&req)
    ```
*   **Extraction Logic:**
    1.  The AST walker looks for a `CallExpr` to a method named `Decode` on a `json.NewDecoder`.
    2.  It inspects the argument to `Decode` (`&req`) to find the identifier `req`.
    3.  It looks up `req` in the current scope to find its type, `CreateUserRequest`.
    4.  The tool then finds the definition of the `CreateUserRequest` struct and converts its fields and struct tags into a JSON Schema representation.
*   **Final OpenAPI Snippet:**
    ```yaml
    post:
      requestBody:
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/CreateUserRequest'
    # ... in components/schemas ...
    CreateUserRequest:
      type: object
      properties:
        name:
          type: string
          description: The user's name
    ```

#### 3.4. Query Parameters

*   **Go Code:** `_ = r.URL.Query().Get("limit")`
*   **Extraction Logic:**
    1.  The AST walker looks for a `CallExpr` to `r.URL.Query().Get(...)`.
    2.  It extracts the string literal argument, `"limit"`, as the parameter name.
    3.  Type inference is complex. A simple starting point is to default all query parameters to `type: string`. A more advanced implementation could analyze how the resulting variable is used (e.g., in a call to `strconv.Atoi`) to infer the type.
*   **Final OpenAPI Snippet:**
    ```yaml
    get:
      parameters:
        - name: limit
          in: query
          schema:
            type: string
          required: false
    ```

#### 3.5. Responses

*   **Go Code:** `json.NewEncoder(w).Encode(users)` where `users` is of type `[]User`.
*   **Extraction Logic:**
    1.  The walker looks for a `CallExpr` to `Encode` on a `json.NewEncoder`.
    2.  It evaluates the argument to `Encode` to determine its symbolic type (`[]User`).
    3.  The tool finds the definition of `User` to generate its schema.
    4.  The HTTP status code is not explicit in the code. The tool will use a default status code (e.g., `200 OK`) as a starting point.
*   **Final OpenAPI Snippet:**
    ```yaml
    get:
      responses:
        '200':
          description: OK
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: '#/components/schemas/User'
    ```
