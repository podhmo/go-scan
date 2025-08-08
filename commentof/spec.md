# `commentof` Package Specification

## 1. Overview

The `commentof` package is a Go library for statically analyzing Go source code to extract comments associated with declarations such as functions, types, constants, and variables. It uses the standard `go/parser` and `go/ast` packages to build an Abstract Syntax Tree (AST) and provides the comment information associated with each node as structured data.

## 2. Data Structures

The results of the comment extraction are represented by the following structures.

### 2.1. `commentof.Function`

Stores comment information for a function.

-   `Name` (string): The function name.
-   `Doc` (string): The comment immediately preceding the function declaration.
-   `Params` ([]*Field): A list of the function's parameters.
-   `Results` ([]*Field): A list of the function's return values.

### 2.2. `commentof.TypeSpec`

Stores comment information for a type declaration.

-   `Name` (string): The type name.
-   `Doc` (string): The comment immediately preceding or on the same line as the type declaration.
-   `Definition` (interface{}): The definition of the type. Currently, this only stores `*commentof.Struct`. For type aliases, it is `nil`.

### 2.3. `commentof.ValueSpec`

Stores comment information for a `const` or `var` declaration.

-   `Names` ([]string): A list of constant or variable names.
-   `Doc` (string): The comment immediately preceding or on the same line as the declaration.
-   `Kind` (token.Token): The type of declaration (`token.CONST` or `token.VAR`).

### 2.4. `commentof.Struct`

Stores comment information for a struct definition.

-   `Fields` ([]*Field): A list of the struct's fields.

### 2.5. `commentof.Field`

Stores comment information for a named element, such as a function parameter, a return value, or a struct field.

-   `Names` ([]string): A list of element names, supporting grouped declarations like `x, y int`.
-   `Type` (string): The string representation of the element's type.
-   `Doc` (string): The comment immediately preceding or on the same line as the element.

## 3. Comment Extraction Rules

### 3.1. Basic Rules

-   **Doc Comments**: `//` or `/* ... */` comments that appear immediately before a declaration, with no blank lines between the comment and the declaration, are extracted.
-   **Line Comments**: `//` comments that appear at the end of the same line as a declaration are extracted.
-   Doc and Line comments are combined into a single `Doc` field, separated by a newline.

### 3.2. Functions (`func`)

-   `ast.FuncDecl.Doc` is extracted as the function's `Doc`.
-   For each `ast.Field` in a function's `Params` and `Results` (`ast.FieldList`):
    -   The parser attempts to associate comments from the source.
    -   **Manual Association (Current Implementation)**: Due to limitations in the Go AST's automatic comment association for function parameters, a manual, position-based search is performed. It searches for comments on the same line as a parameter/result, between the start of that parameter and the start of the next one.
    -   **Multi-line Parameters/Results**: If parameters or results are split across multiple lines, the manual association logic correctly handles comments on each line (e.g., `x int, // comment for x`).
    -   **Interstitial Comments**: Comments placed between parameters (e.g., `x int, /* interstitial */ y string`) are associated with the preceding parameter (`x`).

### 3.3. Types (`type`)

-   If a `ast.GenDecl` contains only one `type` declaration, the `ast.GenDecl.Doc` is treated as the `Doc` for that type.
-   In a grouped declaration like `type (...)`, the `ast.TypeSpec.Doc` for each spec is used. If a `TypeSpec` has no doc, the `GenDecl` doc is used as a fallback.
-   Comments for struct fields (`ast.StructType.Fields`) are extracted similarly to function parameters.

### 3.4. Constants (`const`) and Variables (`var`)

-   The documentation rules are analogous to `type` declarations. The `ast.GenDecl.Doc` is applied to all `ValueSpec`s within the declaration, and is combined with any docs specific to the `ValueSpec` itself (both preceding doc comments and trailing line comments).

## 4. API

### 4.1. `FromFile(filepath string) ([]interface{}, error)`

Parses a Go file from the given path and returns a slice containing the comment information for all top-level declarations.

### 4.2. `FromReader(src io.Reader, filename string) ([]interface{}, error)`

Parses Go source from an `io.Reader` and extracts comment information.

### 4.3. `FromDecls(decls []ast.Decl) ([]interface{}, error)`

Processes a slice of `ast.Decl` directly. **Note**: This function has limited comment association capabilities as it lacks full file context. For best results, use `FromFile` or `FromReader`.

### 4.4. `FromFuncDecl(d *ast.FuncDecl) *Function`

Extracts information from a `*ast.FuncDecl` node. Lacks file context.

### 4.5. `FromGenDecl(d *ast.GenDecl) ([]interface{}, error)`

Extracts information from a `*ast.GenDecl` node. Lacks file context.
