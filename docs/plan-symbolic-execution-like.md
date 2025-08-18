# Plan: A Symbolic-Execution-like Engine for Go Code Analysis

This document outlines the plan for creating a new library, `symgo`, which leverages concepts from symbolic execution to analyze Go source code. The primary goal is to interpret Go code that defines application behavior (like HTTP routes) and extract structured information without running the actual code or requiring manual annotations.

## 1. Goals

*   **Primary Use-Case:** To analyze `net/http` or `go-chi` router definitions to automatically generate OpenAPI (or similar) documentation.
*   **Symbolic Interpretation:** "Execute" Go code by interpreting its Abstract Syntax Tree (AST). Instead of operating on concrete values (e.g., `int 42`), the engine will operate on symbolic values (e.g., a representation of a string literal, a type definition).
*   **No Magic Comments:** The system should derive information from the code itself, not from metadata in comments.
*   **No Special Builders:** Users should not have to rewrite their code using a special DSL or builder pattern. The engine should analyze standard Go code.
*   **Handle Code Structure:** The engine must be able to follow function calls, including those to helper functions defined in the same or different packages.
*   **Leverage Existing Tools:**
    *   Use `go-scan` for its efficient, on-demand AST loading capabilities.
    *   Use `minigo`'s design as inspiration for package management and scope resolution, but without creating a hard dependency.

## 2. Core Concepts

The system will not be a true symbolic execution engine, which often involves complex SMT solvers. Instead, it will be an **AST Interpreter** focused on a specific domain (API definitions).

*   **Symbolic Value:** A representation of a value at "runtime". It doesn't hold the concrete value but rather its type, and possibly its origin or literal value.
    *   `SymbolicString{Literal: "/users/{userID}"}`
    *   `SymbolicStruct{Type: "main.CreateUserRequest"}`
    *   `SymbolicFunction{Name: "CreateUserHandler", AST: *ast.FuncDecl}`
    *   `SymbolicChiRouter{...}`: A special symbolic value representing a `go-chi` router, which tracks registered routes.

*   **Evaluation Context (Scope):** A data structure that acts as the interpreter's "memory." It maps variable identifiers (`*ast.Ident`) to their `SymbolicValue`. This context will be stacked to handle lexical scoping (e.g., entering a new function).

*   **Intrinsic Functions:** The interpreter will have special knowledge of certain key functions (e.g., `chi.NewRouter`, `router.Get`, `json.NewDecoder`). When it encounters a call to an intrinsic function, it will not evaluate the function's real body. Instead, it will execute a custom "hook" that manipulates the interpreter's state. For example, encountering `router.Get(...)` will add a route to the `SymbolicChiRouter` value.

## 3. Architecture

The system will be composed of several key components within the `symgo` package.

1.  **The `symgo` Engine (`evaluator.go`):**
    *   The core of the library. It will contain an `Evaluator` struct.
    *   The `Evaluator` will hold the overall state, including the `go-scan` instance for loading packages.
    *   It will have methods like `Eval(node ast.Node, scope *Scope)` which recursively walk the AST.

2.  **State Management (`scope.go`):**
    *   A `Scope` object will manage the mapping of variable names to symbolic values within a given block or function.
    *   Scopes can be nested to correctly represent variable shadowing.

3.  **`go-scan` Integration (`loader.go`):**
    *   A wrapper around `go-scan` to provide a simple interface for the `Evaluator`.
    *   When the `Evaluator` encounters a call to a function in another package, it will use this component to request the AST for that function.

4.  **Intrinsic Function Definitions (`intrinsics.go`):**
    *   A registry of known function signatures and their corresponding handler logic.
    *   For example, `map["github.com/go-chi/chi/v5.Get"] = handleChiGet`.
    *   The `handleChiGet` function would inspect the arguments of the `ast.CallExpr`, extract the path and handler, and update the symbolic router state.

## 4. Phased Implementation Plan

### Phase 1: Core AST Interpreter
*   **Task:** Create the basic `Evaluator` and `Scope` structures.
*   **Details:**
    *   Implement evaluation for basic expression types: `ast.BasicLit` (strings, numbers), `ast.Ident`.
    *   Implement evaluation for basic statements: `ast.AssignStmt` (`=` and `:=`).
    *   The result of evaluating a node will be a `SymbolicValue`.

### Phase 2: Function Call Resolution & `go-scan` Integration
*   **Task:** Enable the interpreter to follow function calls.
*   **Details:**
    *   When `Eval` encounters an `ast.CallExpr`, resolve the function being called.
    *   If the function is in the current package, find its `ast.FuncDecl`.
    *   If the function is in another package, use `go-scan` to load its AST.
    *   Implement the logic to create a new `Scope` for the function call, map arguments to parameters, and evaluate the function's body.

### Phase 3: Intrinsic Functions for `go-chi`
*   **Task:** Specialize the engine for the `go-chi` use case.
*   **Details:**
    *   Define the `SymbolicChiRouter` value, which will contain a slice of `Route` structs (`{Method, Path, Handler}`).
    *   Implement intrinsic handlers for:
        *   `chi.NewRouter()`: Returns a new `SymbolicChiRouter` instance.
        *   `router.Get()`, `router.Post()`, etc.: These handlers will extract the path string literal and the handler function identifier from the arguments. They will then add a new `Route` to the `SymbolicChiRouter`'s state.
        *   `router.Mount()`, `router.Route()`: These will be handled by recursively evaluating the sub-router definitions within a new scope.

### Phase 4: Handler Analysis
*   **Task:** Analyze the handler functions themselves to extract request and response shapes.
*   **Details:**
    *   After identifying a route and its handler, the engine will perform a secondary, targeted analysis of the handler's AST.
    *   It will look for specific patterns:
        *   **Request Body:** A call like `json.NewDecoder(r.Body).Decode(&req)`. The engine will identify the type of the `req` variable and register it as the request body's schema.
        *   **Response Body:** A call like `json.NewEncoder(w).Encode(resp)`. The engine will identify the type of the `resp` variable as the response schema.
        *   **Path Parameters:** A call like `chi.URLParam(r, "userID")`. This confirms the usage of a path parameter.

### Phase 5: Example Implementation (`examples/docgen`)
*   **Task:** Build a working example that generates an OpenAPI document.
*   **Details:**
    *   Create a simple sample API using `go-chi` in a separate file (e.g., `examples/sampleapi/main.go`).
    *   Create a program in `examples/docgen/main.go` that:
        1.  Initializes the `symgo` `Evaluator`.
        2.  Tells the evaluator to start at the `main` function of the sample API.
        3.  Retrieves the final `SymbolicChiRouter` state after evaluation completes.
        4.  Iterates through the collected routes.
        5.  For each route, formats the information (path, method, request/response schemas) into an OpenAPI 3.0 JSON or YAML file and prints it to standard output.

### Phase 6: Refinement and Documentation
*   **Task:** Polish the library.
*   **Details:**
    *   Add comprehensive godoc comments to the `symgo` package.
    *   Write a `README.md` for `symgo` explaining how to use it.
    *   Improve error handling and diagnostics.

## 5. Use-Case Walkthrough: `examples/docgen`

**1. The Target Go Code (`examples/sampleapi/main.go`)**
```go
package main

import (
	"net/http"
	"github.com/go-chi/chi/v5"
)

type CreateUserInput struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type User struct {
	ID int `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func CreateUserHandler(w http.ResponseWriter, r *http.Request) {
	var input CreateUserInput
	// The engine will see this line to determine the request body shape.
	json.NewDecoder(r.Body).Decode(&input)

	user := User{ID: 1, Name: input.Name, Email: input.Email}
	// The engine will see this line to determine the response body shape.
	json.NewEncoder(w).Encode(user)
}

func main() {
	r := chi.NewRouter()
	r.Post("/users", CreateUserHandler)
	http.ListenAndServe(":3000", r)
}
```

**2. The Generator (`examples/docgen/main.go`)**
```go
package main

import (
    "fmt"
    "github.com/your-repo/symgo" // Assuming this path
)

func main() {
    // Point the evaluator to the target package and entrypoint function.
    evaluator := symgo.NewEvaluator("github.com/your-repo/examples/sampleapi")
    symbolicState := evaluator.Run("main") // Start at the 'main' function

    // The evaluator returns the final state, which we can inspect.
    openAPIDoc := generateOpenAPI(symbolicState)
    fmt.Println(openAPIDoc)
}

// (generateOpenAPI would be a function that formats the symbolic state)
```

**3. Expected Symbolic Result (Internal to `symgo`)**

After running, the `evaluator`'s state would contain a symbolic representation equivalent to:
```
{
  "main.r": {
    Type: "SymbolicChiRouter",
    Routes: [
      {
        Method: "POST",
        Path: "/users",
        Handler: {
          Name: "CreateUserHandler",
          RequestSchema: { Type: "main.CreateUserInput" },
          ResponseSchema: { Type: "main.User" }
        }
      }
    ]
  }
}
```

**4. Final Output (Printed by `docgen`)**

```yaml
openapi: 3.0.0
info:
  title: Sample API
  version: 1.0.0
paths:
  /users:
    post:
      summary: Creates a new user
      requestBody:
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/CreateUserInput'
      responses:
        '200':
          description: The created user
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/User'
components:
  schemas:
    CreateUserInput:
      type: object
      properties:
        name:
          type: string
        email:
          type: string
    User:
      type: object
      properties:
        id:
          type: integer
        name:
          type: string
        email:
          type: string
```

## 6. Challenges & Considerations

*   **Control Flow:**
    *   **Conditionals (`if`):** For now, the engine will likely have to ignore conditional branches or require the user to analyze a specific, non-conditional setup function. True path-sensitive analysis is out of scope.
    *   **Loops (`for`):** Loops that define routes are an advanced case. Initially, they will not be supported. The engine will focus on statically defined routes.
*   **Complex Go Features:** Goroutines, channels, and interfaces will not be modeled. The interpreter will focus on a synchronous, single-threaded execution path.
*   **Variable State:** Tracking the state of variables that are not structs or literals (e.g., a map modified in a loop) is complex. The initial focus will be on identifiable function calls and struct definitions.
*   **Performance:** Heavy use of `go-scan` can be slow if many packages need to be parsed. Caching strategies may be necessary in the future.
```
