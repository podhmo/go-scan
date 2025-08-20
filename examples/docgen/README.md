# `docgen`: OpenAPI 3.1 Documentation Generator

`docgen` is an experimental command-line tool that demonstrates the use of the `symgo` symbolic execution engine to generate an OpenAPI 3.1 specification from a standard `net/http` Go application.

## What It Does

The tool analyzes the Go source code of a sample API (`./sampleapi`) to extract API metadata. It works by:

1.  **Finding the Entrypoint**: It starts by locating the `main` function of the target application.
2.  **Symbolic Execution with `symgo`**: It uses the `symgo` interpreter to "execute" the code symbolically. Instead of running an HTTP server, it traces function calls.
3.  **Pattern Matching**: It uses custom handlers (intrinsics) to intercept calls to `net/http.HandleFunc`. From these calls, it extracts:
    - The HTTP method and path pattern (e.g., `GET /users/{id}`).
    - The handler function associated with the route.
4.  **Deep Handler Analysis**: It then symbolically executes the handler function itself to find calls to `json.NewDecoder` or `json.NewEncoder`, which it uses to infer the request and response body schemas.
5.  **Generating the Spec**: Finally, it aggregates all the collected metadata and prints a valid OpenAPI 3.1 JSON specification to standard output.

## How to Run

You can run the `docgen` tool from the root of the `go-scan` repository:

```sh
go run ./examples/docgen > openapi.json
```

This will execute the analysis on the `./sampleapi` package and pipe the resulting JSON into a new `openapi.json` file.
