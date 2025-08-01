# MiniGo Interpreter Example

This directory contains `minigo`, a miniature Go interpreter, designed to showcase and test the capabilities of the `github.com/podhmo/go-scan` library.

## Overview

`minigo` is a simplified interpreter that can parse and execute a small subset of Go-like syntax. Its primary purpose is to serve as a practical example and a test bed for the `go-scan` library, particularly demonstrating how `go-scan` can be used to analyze Go source code for more complex applications like interpreters or static analysis tools.

The development of `minigo` and the underlying `go-scan` library has been an iterative process, with insights and design decisions often logged in the `docs/` directory of the main `go-scan` project.

## Core `go-scan` Features Highlighted

The `go-scan` library (`github.com/podhmo/go-scan`) provides robust tools for parsing Go source code and extracting type information without relying on `go/packages` or `go/types`. This example leverages several key features:

### 1. AST-based Parsing

`go-scan` directly parses the Abstract Syntax Tree (AST) of Go source files using `go/parser` and `go/ast`. This allows for fine-grained analysis of code structure.

### 2. Type Information Extraction

It can extract detailed information about:
- Struct definitions (fields, tags, embedded structs)
- Type aliases and their underlying types
- Function type declarations
- Constants and function signatures

### 3. Documentation Parsing

GoDoc comments associated with types, fields, functions, and constants are captured, enabling rich metadata extraction.

### 4. Cross-Package Type Resolution (Lazy Loading)

A crucial feature demonstrated through `minigo`'s potential need to understand types from different packages is `go-scan`'s ability to resolve type definitions across package boundaries within the same module. This is achieved through a **lazy loading** mechanism.

As detailed in `docs/ja/multi-project.md` (within the `go-scan` repository), the `scanner.FieldType.Resolve()` method plays a key role. When `go-scan` encounters a type from another package (e.g., `models.User`), it initially registers it by name. The actual definition of `models.User` (its fields, methods, etc.) is only parsed and loaded when `Resolve()` is explicitly called on that `FieldType`.

This on-demand parsing is managed by a `PackageResolver` (typically implemented by the top-level `typescanner.Scanner`), which caches parsed package information to avoid redundant work. This lazy approach offers several benefits:

-   **Efficiency**: Only necessary packages are parsed, saving time and resources, especially in large projects.
-   **Resilience**: It can potentially operate even if some unrelated parts of a project have errors, as long as the directly needed code is parsable.
-   **Flexibility**: Allows generators or tools to decide when and if they need the full definition of an external type.

For `minigo`, this means if it were to interpret code that uses types from various internal packages, `go-scan` would help in understanding those types efficiently.

### 5. Package Locator

`go-scan` includes a `locator` component that can find the module root (by locating `go.mod`) and resolve internal Go import paths to physical directory paths. This is essential for the cross-package type resolution feature.

## Package Imports in `minigo`

`minigo` supports importing symbols (currently constants only) from other packages within the same Go module. This functionality leverages `go-scan` for package resolution and parsing.

### Lazy Import Specification

Import handling in `minigo` is designed to be "lazy":

-   When an `import` statement (e.g., `import "my/pkg"` or `import p "my/pkg"`) is encountered, `minigo` only records a mapping between the local package name (either the base name of the import path, like `pkg`, or the specified alias, like `p`) and the actual import path (`"my/pkg"`).
-   **No files from the imported package are read or parsed at this stage.**
-   The actual scanning of the imported package and loading of its symbols occurs only when a symbol from that package is first referenced via a selector expression (e.g., `pkg.MyConst` or `p.AnotherConst`).

This lazy approach ensures that `minigo` only expends resources on parsing packages that are actually used by the script.

### Referencing Imported Symbols

You can reference symbols from imported packages using either the package's base name or an alias:

-   **Without Alias:**
    ```go
    // your_script.mgo
    package main

    import "mytestmodule/testpkg" // Assumes testpkg is in your Go module

    var MyMessage = testpkg.ExportedConst // Accesses ExportedConst from testpkg
    ```

-   **With Alias:**
    ```go
    // your_script.mgo
    package main

    import pkga "mytestmodule/testpkg"

    var MyNumber = pkga.AnotherExportedConst // Accesses AnotherExportedConst via alias
    ```

### Supported Symbols

Currently, `minigo` supports importing and referencing the following from other packages:
- **Exported constants**: e.g., `pkg.MyConst`. Values are parsed and converted to `minigo`'s internal object types (Integer, String, Boolean).
- **Exported functions**: e.g., `pkg.MyFunction`. These can be called from `minigo` scripts. The interpreter handles mapping arguments and return values between `minigo` objects and Go types, based on function signatures obtained via `go-scan`.

Support for importing and referencing exported global variables from other packages may be added in the future.

### Unsupported Import Forms

The following import forms are **not currently supported** by `minigo` and will result in an error during the initial parsing of import statements:

-   **Dot Imports:** `import . "my/pkg"`
    -   This form, which would bring all exported symbols from `my/pkg` into the current file's namespace, is not supported.
-   **Blank Imports for Execution:** `import _ "my/pkg"`
    -   While Go uses blank imports for their side effects (e.g., `init` functions), `minigo` does not currently support package initialization via `init` functions. Therefore, blank imports are **ignored** by `minigo` as they would have no effect in the current execution model and cannot be used to access symbols.

These limitations are in place to simplify the initial implementation of package imports.

## Running `minigo` (Conceptual)

While `minigo` is primarily an example for `go-scan`, a conceptual way to run it (once fully implemented) might look like this:

```bash
# Navigate to the minigo directory
cd examples/minigo

# Run the interpreter with a Go-like source file
go run main.go your_script.mgo
```
*(Note: `main.go` and the exact execution mechanism are illustrative at this stage of documenting `go-scan` features.)*

## Development Insights

The `docs/ja/llm-history.md` and `docs/ja/multi-project.md` files in the `go-scan` repository contain extensive discussions and design choices made during the development of `go-scan`, including the rationale behind features like lazy loading and the `ScanBroker` concept for managing shared scan states. These documents provide deeper context into the library's architecture.
