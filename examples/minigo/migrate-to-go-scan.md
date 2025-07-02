# Migration Plan for Utilizing `go-scan` in `minigo`

## 1. Introduction

This document outlines the migration plan for the `examples/minigo` project (hereafter `minigo`) to utilize the `go-scan` package (hereafter `go-scan`) provided at the top level for improving its source code parsing capabilities.

`minigo` currently uses the Go standard library's `go/parser` directly to parse source files, build an AST (Abstract Syntax Tree), and execute it as an interpreter. On the other hand, `go-scan` is a utility specialized in extracting type information, function signatures, package structures, etc., from Go source code.

The objectives of this migration are:

*   **Standardize Code Parsing:** Enhance reusability of code parsing logic by using the common `go-scan` foundation.
*   **Improve Maintainability:** Enable `minigo` to benefit from more advanced parsing features as `go-scan` evolves.
*   **Potential Feature Enhancements:** Explore the possibility of leveraging type information and dependency resolution features provided by `go-scan` (in the future) for `minigo`'s type checking, module system, etc.

## 2. Current Parsing Process in `minigo`

The `LoadAndRun` function in `minigo`'s `interpreter.go` is the starting point for its parsing process.

*   It uses `go/parser.ParseFile()` to parse the specified source file and obtain an `*ast.File`.
*   It traverses the `Decls` field of the `*ast.File` to find function declarations (`ast.FuncDecl`) and global variable declarations (`ast.GenDecl` with `Tok == token.VAR`).
*   From function declarations, it extracts the function name, parameters, and, most importantly, the function body (`ast.BlockStmt`), then registers it into the interpreter's environment.
*   Global variable declarations are evaluated and registered via the `eval` function.
*   The core logic of the interpreter, the `eval` function group, directly processes various concrete types of `ast.Node` (e.g., `ast.ExprStmt`, `ast.BinaryExpr`, `ast.CallExpr`) and recursively evaluates the code.
*   Error reporting uses `*token.FileSet` and `token.Pos` to include detailed positional information such as filename, line number, and column number.

A current limitation is that `minigo` handles everything from file parsing to AST manipulation and evaluation monolithically, without leveraging a more generic code parsing infrastructure.

## 3. Overview of `go-scan`

`go-scan` is a tool for statically analyzing Go source code and extracting package-level structural information. Its main features and data structures include:

*   **`scanner.Scanner`**: The primary type પાણીng parsing. Provides `ScanPackage()` and `ScanFiles()` methods.
*   **`scanner.PackageInfo`**: Information for an entire package.
    *   `Name`, `Path`, `Files` (list of files)
    *   `Types []*TypeInfo`: List of type definitions.
    *   `Constants []*ConstantInfo`: List of constant definitions.
    *   `Functions []*FunctionInfo`: List of top-level function definitions.
    *   `Fset *token.FileSet`: The file set used for parsing, crucial for resolving positional information in error reports.
*   **`scanner.FunctionInfo`**: Function signature information (name, parameters, result types, etc.).
*   **`scanner.TypeInfo`**: Detailed information about type definitions (structs, interfaces, aliases, etc.).
*   Various info structures (`TypeInfo`, `ConstantInfo`, etc.) may contain references to the original AST node (`Node ast.Node`) and the definition file path (`FilePath`), aiding in detailed analysis and positional information.

`go-scan` primarily focuses on collecting type and signature information and does not directly provide detailed AST structures at the expression level (e.g., the internals of an `ast.BinaryExpr`) required for code execution in `minigo`.

## 4. Challenges in Integration

### 4.1. Deficiencies in `minigo` (Required Changes)

*   **Adapting to Parser Interface Changes:**
    *   Shift from direct calls of `go/parser.ParseFile` to using `go-scan`'s `Scanner.ScanFiles` (or `ScanPackage`).
    *   The `Interpreter` struct might need modification to manage and utilize a `go-scan` `Scanner` instance.
*   **Modifying Input Source for `eval` Functions:**
    *   Currently, `eval` directly accepts `ast.Node`. `go-scan` provides `FunctionInfo` or `PackageInfo`. A mechanism is needed to extract `*ast.BlockStmt` (function bodies) or `*ast.GenDecl` (global variables) digestible by `eval` from these.
*   **Adjusting Error Handling:**
    *   Errors from `go-scan`'s scanning process and `minigo`'s evaluation phase must be handled cohesively. The `FileSet` provided by `go-scan` needs to be integrated with `minigo`'s error reporting (`formatErrorWithContext`).

### 4.2. Desired Features from `go-scan` (Recommended Enhancements)

For `minigo` to efficiently utilize `go-scan`, the following information/features from `go-scan` would be highly beneficial:

*   **Access to Function Bodies (AST):**
    *   Ideally, `scanner.FunctionInfo` should contain a reference to the corresponding `*ast.FuncDecl` node (e.g., a field like `Node *ast.FuncDecl`). This would allow `minigo` to easily access the function body via `FunctionInfo.Node.(*ast.FuncDecl).Body`.
    *   Currently, `FunctionInfo` lacks this direct field.
*   **Access to Global Variable Declarations (AST):**
    *   It would be desirable for `scanner.PackageInfo` to include a list of top-level `var` declarations (`*ast.GenDecl` where `Tok == token.VAR`) from the files (e.g., `GlobalVars []*ast.GenDecl`).
    *   Currently, `PackageInfo` primarily lists types, functions, and constants, without explicitly aggregating global variable information.
*   **Retention of `*ast.File` by `PackageInfo`:**
    *   If `scanner.PackageInfo` could hold a slice or map of `*ast.File` for each scanned file (e.g., `AstFiles map[string]*ast.File`, keyed by file path), `minigo` could directly access necessary AST nodes (function bodies, global variable declarations). This would significantly simplify integration by reducing the need for `minigo` to re-parse files or for `go-scan` to add specific `Node` fields to `FunctionInfo` or `GlobalVars` to `PackageInfo` for these particular cases. This is a **highly recommended enhancement** for `go-scan`.

If `go-scan` does not provide this information, `minigo` would need to implement workarounds (e.g., re-parsing based on file paths, or `go-scan` for structure and `go/parser` for details), leading to processing overhead and complexity.

### 4.3. Interface Mismatches

*   **Granularity Difference:**
    *   `minigo`'s `eval` depends on detailed AST nodes (expression, statement level).
    *   `go-scan` provides higher-level structural information (package, file, function/type signature level).
*   **Specific Data Structure Differences:**
    *   `minigo` directly gets `Body` from `ast.FuncDecl`, whereas `go-scan` yields `FunctionInfo`. Bridging these is key.

## 5. Proposed Integration Strategy: Hybrid Approach

A **hybrid approach** is proposed, where `go-scan` is used for the primary **package/file structure analysis phase** in `minigo`, while `minigo`'s core **evaluation phase** (`eval` function group) continues to operate directly on `go/ast` nodes.

**Specific Steps:**

1.  **Initialization:** `minigo`'s `Interpreter` creates a `go-scan` `Scanner` instance, sharing the `FileSet`.
2.  **Scanning:** During `LoadAndRun`, `Scanner.ScanFiles()` (or `ScanPackage`) is called to get `PackageInfo`.
3.  **Registering Functions and Global Variables:**
    *   **Functions:** Process `PackageInfo.Functions`.
        *   **Ideal (with `go-scan` enhancement):** Obtain function name, parameters, and body (`Body`) from `FunctionInfo.Node.(*ast.FuncDecl)` (if `FunctionInfo.Node` refers to `*ast.FuncDecl`) or from the `*ast.File` held by `PackageInfo` (if `PackageInfo.AstFiles` is available). Register as `UserDefinedFunction` in `minigo`'s environment.
        *   **Alternative (current `go-scan`):** If `FunctionInfo.Node` is not available or `PackageInfo` doesn't hold `*ast.File`, `minigo` might re-parse the file using `FunctionInfo.FilePath` and function name to find the `*ast.FuncDecl`.
    *   **Global Variables:**
        *   **Ideal (with `go-scan` enhancement):** Process a list of global variables (e.g., `[]*ast.GenDecl`) provided by `PackageInfo` or obtained from `*ast.File` held by `PackageInfo`. Evaluate and register them via `eval`.
        *   **Alternative (current `go-scan`):** `minigo` parses each file in `PackageInfo.Files` using `go/parser.ParseFile` to extract `ast.GenDecl` (VAR) from `*ast.File.Decls`.
4.  **Executing Entry Point:** Find the `FunctionInfo` for the entry point function from `PackageInfo.Functions`. Obtain the function body (`*ast.BlockStmt`) using the methods above and call `applyUserDefinedFunction` to start evaluation.
5.  **Evaluation:** The `eval` function group remains largely unchanged, continuing to process `ast.Node`.
6.  **Error Reporting:** Utilize the `FileSet` and positional information from `go-scan` with `minigo`'s `formatErrorWithContext` for accurate error messages.

**Expectations from `go-scan` (Recommended Enhancements):**

To make this hybrid approach most effective, it is **highly recommended** that `go-scan` be enhanced as discussed in section "4.2. Desired Features from `go-scan`", particularly:
*   `scanner.PackageInfo` retaining the `*ast.File` for each scanned file.
*   Alternatively, or additionally, `scanner.FunctionInfo` providing a `Node *ast.FuncDecl` and `scanner.PackageInfo` providing a list of global variable declarations.

**Future Consideration: Lazy Import:**

*   `go-scan`'s `PackageResolver` interface (used by `FieldType.Resolve()`) suggests a capability for lazy-loading and resolving types across package boundaries. `minigo` could potentially leverage this in the future to interpret codebases spanning multiple files or (simulated) external packages by implementing a `PackageResolver` that tells `go-scan` how to find and scan imported packages. This is not part of the initial integration scope but a valuable future direction.

## 6. Expected Benefits

*   **Separation of Concerns in Parsing:** `go-scan` handles file/package level structural analysis, allowing `minigo` to focus on AST-based evaluation.
*   **Consistent Positional Information:** Using `go-scan`'s `FileSet` ensures consistency in error reporting and debug information.
*   **Future Extensibility:** Makes it easier for `minigo` to incorporate advancements in `go-scan` (e.g., type resolution, dependency analysis).

## 7. Future Work / Open Issues

*   **Discuss `go-scan` Enhancements:** Collaborate with the `go-scan` team/contributors to discuss and potentially implement the recommended enhancements (e.g., `PackageInfo` holding `*ast.File`, `FunctionInfo` referencing `*ast.FuncDecl`, `PackageInfo` listing global variables).
*   **Performance Evaluation:** Assess the performance impact of introducing `go-scan`, especially if alternative approaches (like re-parsing) are used.
*   **Enhancing Interpreter Features:** Utilize type information from `go-scan` to improve `minigo`'s type checking capabilities or support more advanced language features.
*   **Implementing Lazy Import for Multi-File Projects:** Explore the use of `PackageResolver` for `minigo` to handle imports and interpret projects spread across multiple files.

This concludes the migration plan for utilizing `go-scan` within `minigo`.
