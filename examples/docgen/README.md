# `docgen`: OpenAPI 3.1 Documentation Generator

`docgen` is a command-line tool that generates an OpenAPI 3.1 specification from a standard `net/http` Go application. It serves as a practical example of the `symgo` symbolic execution engine.

## What It Does

The tool analyzes the Go source code of a sample API (`./sampleapi`) to extract API metadata. It works by:

1.  **Finding the Entrypoint**: It starts by locating a specific function (e.g., `NewServeMux`) in the target package.
2.  **Symbolic Execution with `symgo`**: It uses the `symgo` interpreter to "execute" the code symbolically. Instead of running an HTTP server, it traces function calls.
3.  **Pattern Matching with Intrinsics**: It uses custom handlers (intrinsics) to intercept calls to `net/http.HandleFunc` and similar routing functions. From these calls, it extracts:
    - The HTTP method and path pattern (e.g., `GET /users`).
    - The handler function associated with the route.
4.  **Deep Handler Analysis**: It then symbolically executes the handler function itself to find calls to `json.NewDecoder`, `r.URL.Query().Get()`, or `json.NewEncoder`. It uses these to infer request schemas, query parameters, and response schemas.
5.  **Generating the Spec**: Finally, it aggregates all the collected metadata and prints a valid OpenAPI 3.1 specification to standard output in either JSON or YAML format.

## How to Run

You can run `docgen` from the root of the `go-scan` repository.

### Prerequisites

The tool has its own dependencies defined in `examples/docgen/go.mod`. Ensure they are installed:
```sh
cd examples/docgen
go mod tidy
cd ../..
```

### Usage

```sh
go run ./examples/docgen [flags]
```

**Flags:**
- `-format <string>`: The output format. Can be `json` (default) or `yaml`.
- `-debug`: Enable debug logging for the analysis.

### Examples

**Generate JSON output (default):**
```sh
go run ./examples/docgen > openapi.json
```

**Generate YAML output:**
```sh
go run ./examples/docgen -format=yaml > openapi.yaml
```

## Sample Output (`-format=yaml`)

```yaml
openapi: 3.1.0
info:
    title: Sample API
    version: 0.0.1
paths:
    /users:
        get:
            description: |
                listUsers handles the GET /users endpoint.
                It returns a list of all users.
            operationId: listUsers
            parameters:
                - name: limit
                  in: query
                  schema:
                    type: string
            responses:
                "200":
                    description: OK
                    content:
                        application/json:
                            schema:
                                type: array
                                items:
                                    type: object
                                    properties:
                                        id:
                                            type: integer
                                            format: int32
                                        name:
                                            type: string
        post:
            description: |
                createUser handles the POST /users endpoint.
                It creates a new user.
            operationId: createUser
            requestBody:
                content:
                    application/json:
                        schema:
                            type: object
                            properties:
                                id:
                                    type: integer
                                    format: int32
                                name:
                                    type: string
                required: true
            responses:
                "200":
                    description: OK
                    content:
                        application/json:
                            schema:
                                type: object
                                properties:
                                    id:
                                        type: integer
                                        format: int32
                                    name:
                                        type: string
```
