# Customizing `docgen` Analysis with `minigo` Patterns

The `docgen` tool uses a symbolic execution engine (`symgo`) to analyze Go code and generate OpenAPI specifications. While it has built-in support for standard `net/http` and `encoding/json` patterns, real-world applications often use custom helper functions for handling requests and responses. To support these, `docgen` allows you to define custom analysis patterns using a Go configuration file, which is interpreted by `minigo`.

This document explains how to create a custom patterns file and details all the available pattern types.

## Creating a Patterns Configuration File

To provide custom patterns to `docgen`, you create a Go file (e.g., `patterns.go`) in your project. This file must have a `//go:build minigo` build tag and define a global variable named `Patterns`.

You pass the path to this file to `docgen` using the `--patterns` flag:
```bash
docgen --patterns=./path/to/your/patterns.go <your-api-package>
```

### Structure of the Configuration File

The configuration file looks like this:

```go
//go:build minigo
// +build minigo

package main

import "github.com/podhmo/go-scan/examples/docgen/patterns"

// Patterns defines a list of custom patterns for the docgen tool.
var Patterns = []patterns.PatternConfig{
    // ... your pattern definitions here ...
}
```

The `Patterns` variable is a slice of `patterns.PatternConfig` structs. Each struct defines one custom analysis rule.

## `PatternConfig` Struct

The `patterns.PatternConfig` struct has the following fields:

- `Key` (string): **Required.** The fully qualified function or method name to match.
  - For functions: `"<module-path>/<package-name>.<function-name>"`
    - Example: `"myapp/helpers.RenderJSON"`
  - For methods: `"<module-path>/<package-name>.<receiver>.<method-name>"`
    - Example: `"myapp/models.(*User).GetProfile"`
- `Type` (PatternType): **Required.** The type of analysis to perform. See the "Pattern Types" section below for all available options.
- `ArgIndex` (int): **Required.** The 0-based index of the function argument to analyze. For parameter types, this refers to the argument holding the parameter's **value**.
- `NameArgIndex` (int): Required for parameter types. The 0-based index of the argument holding the parameter's **name**.
- `StatusCode` (string): Required for `CustomResponse`. The HTTP status code for the response.
- `Description` (string): Optional description for parameters.

## Pattern Types

Here are the available values for the `Type` field.

---

### `responseBody`

Treats a function argument as the success (200 OK) response body. `docgen` will generate a schema from the argument's type and place it in the `200` response.

- **`ArgIndex`**: The index of the argument containing the data to be encoded as the response body.

**Example:**
Your code has a helper `helpers.RenderJSON(w, r, myData)`.

```go
// patterns.go
{
    Key:      "myapp/helpers.RenderJSON",
    Type:     patterns.ResponseBody,
    ArgIndex: 2, // The `myData` argument
}
```

---

### `requestBody`

Treats a function argument as the request body. `docgen` will generate a schema from the argument's type and place it in the `requestBody` section of the operation. The argument at `ArgIndex` is expected to be a pointer to the struct that the request body should be decoded into.

- **`ArgIndex`**: The index of the argument that the request body is decoded into. Must be a pointer.

**Example:**
Your code has a helper `helpers.DecodeJSON(r, &input)`.

```go
// patterns.go
{
    Key:      "myapp/helpers.DecodeJSON",
    Type:     patterns.RequestBody,
    ArgIndex: 1, // The `&input` argument
}
```

---

### `defaultResponse`

Treats a function argument as the `default` response body. This is useful for defining a standard error response for an operation. It does not take a status code.

- **`ArgIndex`**: The index of the argument containing the error data structure.

**Example:**
Your code has a helper `helpers.RenderDefaultError(w, r, err)`.

```go
// patterns.go
{
    Key:      "myapp/helpers.RenderDefaultError",
    Type:     patterns.DefaultResponse,
    ArgIndex: 2, // The `err` argument
}
```

---

### `customResponse`

Treats a function argument as a response body for a specific, non-200 status code. This is useful for defining specific error responses (e.g., 400, 404, 500).

- **`ArgIndex`**: The index of the argument containing the response data structure.
- **`StatusCode`**: **Required.** The HTTP status code as a string (e.g., `"400"`).

**Example:**
Your code has a helper `helpers.RenderBadRequest(w, r, validationError)`.

```go
// patterns.go
{
    Key:        "myapp/helpers.RenderBadRequest",
    Type:       patterns.CustomResponse,
    StatusCode: "400",
    ArgIndex:   2, // The `validationError` argument
}
```

---

### `path`, `query`, `header`

Extracts a parameter from a function call where the parameter's name is passed as an argument. This is useful when you have generic helper functions for extracting data from a request.

- **`Type`**: `patterns.PathParameter`, `patterns.QueryParameter`, or `patterns.HeaderParameter`.
- **`NameArgIndex`**: **Required.** The index of the argument containing the parameter's **name** (must be a string literal).
- **`ArgIndex`**: **Required.** The index of the argument containing the parameter's **value**. The schema will be inferred from this argument's type. For helpers that *return* the value, this can often be pointed to an argument like `*http.Request` and the schema will default to a string.
- **`Description`**: An optional description for the parameter.

**Example:**
Your code has a helper `framework.GetQuery(r, "sort") string`.

```go
// patterns.go
{
    Key:          "myapp/framework.GetQuery",
    Type:         patterns.QueryParameter,
    NameArgIndex: 1, // The "sort" argument
    ArgIndex:     0, // The `*http.Request` argument, schema will default to string
}
```
This single pattern will now work for all calls to `GetQuery`, extracting the parameter name from the second argument of each call.
