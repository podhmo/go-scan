# Plan for minigo2 Implementation

## 1. Introduction and Concept

**minigo2** will be a powerful, embeddable **code generation engine** for Go, inspired by `examples/convert`. It will enable developers to create custom, type-safe code generators with a remarkable developer experience, including **full code completion and type checking in IDEs** by leveraging existing Go tooling like `gopls`.

The primary goal is to replace complex, reflection-based code generation logic with simple, declarative **mapping scripts**. These scripts, written in standard Go, will define the transformation logic (e.g., from a database model to a DTO), which `minigo2` will interpret to produce the generated Go code.

The core principles of `minigo2` are:
- **IDE-First Development**: Mapping scripts are valid Go files, ensuring that developers get full language server support (completion, go-to-definition, find usages) out of the box.
- **Declarative Logic**: Mappings are defined using a clear, declarative API (e.g., `generate.ConverterFor(...)`) rather than imperative template logic.
- **Type Safety**: The engine uses `go-scan` to deeply understand the Go types involved, enabling the generation of type-safe code.
- **Extensibility**: The engine can be extended with custom mapping functions and rules.

This approach pivots from the initial concept of a runtime configuration interpreter to a build-time tool focused on developer productivity and code quality.

## 2. Core Architecture & Workflow

The `minigo2` engine will operate as a command-line tool, typically invoked via `go:generate`.

**Workflow:**

1.  **Invocation**: A developer adds a `go:generate` comment to their Go source file.
    ```go
    //go:generate go run github.com/podhmo/go-scan/examples/minigo2 -mapping=./mapping.go
    ```
2.  **Scanning**: The `minigo2` tool uses `go-scan` to parse the package where it was invoked. It identifies all types in that package.
3.  **Mapping Script Interpretation**: `minigo2` parses the specified mapping script (e.g., `mapping.go`). This script is a standard Go file (`package main`).
4.  **AST Analysis**: The tool walks the AST of the mapping script's `main` function. It looks for calls to its special `generate` API (e.g., `generate.ConverterFor`).
5.  **Code Model Construction**: By analyzing the arguments to the `generate` API calls, `minigo2` builds an internal, structured representation of the code to be generated (a "code model"). This model contains all necessary information: source and destination types, field mappings, required imports, etc.
6.  **Code Generation**: The constructed code model is passed to a `text/template` engine, which renders the final Go source code for the generated file.
7.  **Output**: The generated code is formatted using standard Go tools (`goimports`) and written to disk.

**Architectural Components:**

- **CLI (`main.go`)**: Handles command-line flags and orchestrates the entire process.
- **Scanner (`go-scan`)**: The underlying engine for parsing Go source code, resolving types across packages, and providing the necessary type information (`scanner.TypeInfo`).
- **Mapping Script Parser**: A component responsible for analyzing the AST of the user's mapping script and extracting the declarative mapping rules.
- **Code Model (`model` package)**: A set of structs that represent the code to be generated (similar to `examples/convert/model`).
- **Generator (`generator` package)**: Contains the `text/template` and the logic for rendering the code model into Go source code.
- **Built-in `generate` API**: A predefined Go package (`github.com/podhmo/minigo2/generate`) that users import into their mapping scripts. This API provides the declarative functions (`ConverterFor`, `Ignore`, `Using`, etc.).

## 3. Language Server (LSP) Integration Design

The key to providing a first-class developer experience is ensuring seamless integration with `gopls`. Our design achieves this by making the mapping script a standard, valid Go file.

**The Strategy:**

1.  **Mapping Scripts are `.go` files**: We abandon the idea of a custom `.mgo` extension. Scripts will be regular `.go` files, typically in a `main` package.
2.  **Declarative API**: The mapping logic is defined by calling functions from a dedicated `generate` package. The `main` function of the mapping script serves as a container for these declarative calls. **This `main` function is never executed directly by `go run` or `go build`**. It exists purely for `gopls` to have an entry point for analysis.
3.  **Type Imports**: The mapping script directly imports the actual Go types it needs to reference. This allows `gopls` to "see" the types and provide accurate information.

**Example Mapping Script (`mapping.go`):**

```go
package main

// Import the user's actual types to make them visible to gopls
import (
    "github.com/my/project/models"
    "github.com/my/project/transport/dto"

    // Import the minigo2 "verbs"
    "github.com/podhmo/minigo2/generate"
)

// This function is analyzed by the minigo2 tool, not executed by `go run`.
func main() {
    // Define a converter from a model.User to a dto.User
    generate.ConverterFor(
        models.User{}, // Pass a zero value of the type to capture it
        dto.User{},
        generate.Options{
            FieldMap: generate.FieldMap{
                "ID":        "ID",
                "Name":      "FullName",  // Map Name to FullName
                "Password":  generate.Ignore(), // Explicitly ignore the Password field
                "CreatedAt": generate.Using(models.FormatTime), // Use a custom function for conversion
            },
        },
    )
}
```

**Benefits of this Design:**

- **Code Completion**: When typing `models.` inside the `ConverterFor` call, the IDE will suggest `User`, `Account`, etc.
- **Go to Definition**: Right-clicking on `models.User` will navigate directly to its struct definition.
- **Find Usages**: Searching for usages of `models.User` will correctly list `mapping.go` as a reference.
- **No Custom LSP Needed**: We leverage `gopls` entirely, avoiding the immense complexity of building a custom language server.

The `minigo2` tool simply needs to parse this Go file and interpret the AST of the `generate.ConverterFor` function call to build its code generation model.

## 4. Implementation Phases

The project will be developed in phases, building from a core engine to the full vision.

1.  **Phase 1: Core Engine and `go-scan` Integration**
    - [ ] Set up the `minigo2` CLI application structure.
    - [ ] Integrate `go-scan` to parse a target package specified by a command-line flag.

2.  **Phase 2: Mapping Script Parser**
    - [ ] Define the built-in `generate` API (`generate.ConverterFor`, etc.).
    - [ ] Implement the parser that walks the AST of a mapping script (`mapping.go`).
    - [ ] The parser's goal is to find calls to `generate.ConverterFor` and extract the type information and mapping options.

3.  **Phase 3: Code Model and Generator**
    - [ ] Define the internal `model` structs to represent the generated code (converters, functions, fields).
    - [ ] Implement the `generator` which takes the `model` and, using a `text/template`, produces the final Go code. This will be very similar to the generator in `examples/convert`.

4.  **Phase 4: End-to-End Workflow**
    - [ ] Connect all the pieces: CLI -> Scanner -> Mapping Parser -> Code Model -> Generator -> Output file.
    - [ ] Create a full working example demonstrating the `go:generate` workflow.

5.  **Phase 5: Advanced Features & Refinement**
    - [ ] Implement recursive discovery of required converters (as seen in `examples/convert`).
    - [ ] Add support for more complex mapping rules (`// convert:rule` equivalent).
    - [ ] Refine error handling to provide clear messages when a mapping script is invalid.
    - [ ] Write comprehensive documentation for the tool and the `generate` API.

## 5. Revised Conceptual Usage Example

**1. User's Go code (`models/user.go`):**
```go
package models

import "time"

//go:generate go run github.com/podhmo/go-scan/examples/minigo2 -mapping=./mapping.go -output=./generated.go

type User struct {
    ID        int
    Name      string
    Password  string
    CreatedAt time.Time
}

func FormatTime(t time.Time) string {
    return t.RFC3339
}
```

**2. User's DTO (`dto/user.go`):**
```go
package dto

type User struct {
    ID       int
    FullName string
    CreatedAt string
}
```

**3. User's Mapping Script (`models/mapping.go`):**
```go
package main

import (
    "github.com/my/project/models"
    "github.com/my/project/transport/dto"
    "github.com/podhmo/minigo2/generate"
)

// This main is the entry point for the minigo2 code generator analysis.
func main() {
    generate.ConverterFor(
        models.User{},
        dto.User{},
        generate.Options{
            FieldMap: generate.FieldMap{
                "ID":        "ID",
                "Name":      "FullName",
                "Password":  generate.Ignore(),
                "CreatedAt": generate.Using(models.FormatTime),
            },
        },
    )
}
```

**4. Running `go generate ./...` would create `models/generated.go`:**
```go
// Code generated by minigo2. DO NOT EDIT.
package models

import (
    "github.com/my/project/transport/dto"
    // ... other necessary imports
)

func ConvertUserToDTO(src *User) *dto.User {
    if src == nil {
        return nil
    }
    dst := &dto.User{}
    dst.ID = src.ID
    dst.FullName = src.Name
    dst.CreatedAt = FormatTime(src.CreatedAt)
    return dst
}
```
