# `docgen`: OpenAPI 3.1 Documentation Generator

`docgen` is a command-line tool that generates an OpenAPI 3.1 specification from a standard `net/http` Go application. It serves as a practical example of the `symgo` symbolic execution engine.

## What It Does

The tool analyzes the Go source code of a sample API (`./sampleapi`) to extract API metadata. It works by:

1.  **Finding the Entrypoint**: It starts by locating a specific function (e.g., `NewServeMux`) in the target package.
2.  **Symbolic Execution with `symgo`**: It uses the `symgo` interpreter to "execute" the code symbolically. Instead of running an HTTP server, it traces function calls.
3.  **Route Analysis**: It intercepts calls to `net/http.HandleFunc` and similar routing functions to extract the HTTP method, path pattern, and associated handler function.
4.  **Deep Handler Analysis**: It then symbolically executes the handler function itself to find calls that indicate how the request is read and how the response is written.
5.  **Extensible Pattern Matching**: The logic for step 4 is not hardcoded. It is driven by a set of user-configurable patterns that map specific function calls (e.g., `(*json.Encoder).Encode`) to OpenAPI concepts (e.g., a response body). This makes the tool highly extensible.
6.  **Generating the Spec**: Finally, it aggregates all the collected metadata and prints a valid OpenAPI 3.1 specification to standard output in either JSON or YAML format.

## Extensible Patterns

A key feature of `docgen` is that its analysis logic is extensible at runtime. Instead of hardcoding that `json.NewEncoder(...).Encode(v)` means "v is a response body," this logic is defined in a separate file: `patterns.go`.

This file is a standard Go source file that is evaluated by the `minigo` interpreter when `docgen` starts. It defines a slice of patterns, allowing users to:
-   Add new rules for custom response-writing functions or request-reading utilities.
-   Adapt the tool to analyze APIs that don't use the standard library `net/http` conventions directly.
-   Modify existing patterns without recompiling the `docgen` tool.

Each pattern maps a function call signature (the "key") to an OpenAPI concept like `"responseBody"` or `"queryParameter"`. This demonstrates how `symgo` can be used to build powerful, adaptable static analysis tools.

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
