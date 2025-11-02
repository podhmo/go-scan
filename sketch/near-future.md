# TODO List for go-scan

This document outlines the current status, planned features, and areas for improvement for the `go-scan` library. It is based on analysis of the existing codebase, README, and specific use-case documents.

**For more ambitious, long-term, and "dream-like" features and concepts, please refer to [./dream2.md](./dream2.md).** That document explores the ultimate potential and broader vision for `go-scan`. This `todo.md` focuses on more concrete and immediate next steps.

## Implemented

-   **AST-based Parsing:** Core functionality using `go/parser` and `go/ast`.
-   **Cross-Package Type Resolution:** Lazy resolution of type definitions within the same module (`FieldType.Resolve()`, `PackageResolver` interface).
-   **Type Definition Extraction:**
    -   Structs (fields, tags, embedded structs): `StructInfo`, `FieldInfo`.
    -   Complex types (pointers, slices, maps): `FieldType` properties (`IsPointer`, `IsSlice`, `IsMap`, `Elem`, `MapKey`).
    -   Type aliases and underlying types: `TypeInfo.Kind == AliasKind`, `TypeInfo.Underlying`.
    -   Function type declarations: `TypeInfo.Kind == FuncKind`, `TypeInfo.Func`.
    -   Interface types: `TypeInfo.Kind == InterfaceKind`, `TypeInfo.Interface`, `InterfaceInfo`, `MethodInfo`.
-   **Constant Extraction:** Top-level `const` declarations (`ConstantInfo`).
-   **Function/Method Signature Extraction:** Top-level functions and methods (`FunctionInfo`, including receiver).
-   **Documentation Parsing:** GoDoc comments for types, fields, functions, constants (`TypeInfo.Doc`, `FieldInfo.Doc`, etc.).
-   **Package Locator:** Module root finding and import path resolution (`locator.Locator`).
-   **External Type Overrides:** Mechanism to treat specified external types as other Go types (`Scanner.ExternalTypeOverrides`, `FieldType.IsResolvedByConfig`).
-   **Basic Generic Type Parsing:**
    -   Type parameters for types and functions: `TypeInfo.TypeParams`, `FunctionInfo.TypeParams`, `TypeParamInfo`.
    -   Type arguments for instantiated generic types: `FieldType.TypeArgs`.
-   **AST Access:** `PackageInfo.AstFiles`, `PackageInfo.Fset`, `TypeInfo.Node`, `FunctionInfo.AstDecl`.
-   **File Generation Helpers:** `goscan.GoFile`, `goscan.PackageDirectory`, `SaveGoFile`.
-   **Import Management:** `goscan.ImportManager` for generated code.
-   **Basic Interface Implementation Check:** `goscan.Implements` (structs implementing interfaces).
-   **Improved Scanning Logic in Example Tools:** Command-line tools in `examples/` now handle file and directory paths more intelligently, distinguishing between them and grouping file arguments by package. This was implemented as described in [sketch/plan-scan-improvement.md](./docs/plan-scan-improvement.md).
-   **Testing Harness (`scantest`):** Implemented the `scantest` library to provide a testing harness for `go-scan` based tools. The implementation, detailed in [sketch/plan-scantest.md](./docs/plan-scantest.md), uses a significant enhancement not in the original plan: I/O operations are intercepted via `context.Context`. This allows `scantest` to capture file generation output in memory without altering the tool's own code, a key difference from initial concepts.
-   **In-Memory File Overlay:** Added an "overlay" feature to `go-scan` to allow providing in-memory file content. This is useful for tools that generate or modify Go source code without writing to the filesystem. This was implemented as described in [sketch/plan-overlay.md](./docs/plan-overlay.md).
-   **Integration Tests for Examples:** Added integration tests for the code generation tools in the `examples/` directory using the new `scantest` library.
-   **Recursive Type and Dependency Handling**: Gracefully handles recursive type definitions and circular dependencies between packages, preventing infinite loops during type resolution.
-   **External and Standard Library Package Resolution**: Can resolve packages from GOROOT and GOMODCACHE via `WithGoModuleResolver`.
-   **Variadic Parameter Parsing**: Correctly parses variadic parameters (e.g., `...string`) as slices.
-   **Fix for Standard Library Scanning in Tests**: Resolved `mismatched package names` errors that occurred during tests via an enhanced `ExternalTypeOverride` mechanism.
-   **On-Demand, Multi-Package AST Scanning**: Implemented a lazy, on-demand scanning system to robustly support multi-package analysis.
-   **`convert` Tool**: A powerful, annotation-driven tool for generating type conversion functions has been fully implemented as detailed in `sketch/plan-neo-convert.md`.
-   **Generator Logic Enhancements**: The code generator now supports recursive converter generation, pointer-aware global rules, and has improved type resolution for generics and pointers.
-   **CLI and Build Fixes**: Corrected underlying issues with `make e2e` and other build/test targets, which are now integrated into the main test suite.

## Partially Implemented/Experimental/Needs Refinement

-   **Symbol Definition Cache (Experimental):**
    -   Currently marked as experimental in README (`cache.SymbolCache`, `Scanner.CachePath`, `FindSymbolDefinitionLocation`).
    -   Needs further testing and refinement for cache invalidation strategies (e.g., handling file content modifications, not just existence/deletion) and overall robustness.
    -   *See [./dream2.md](./dream2.md) for visions on advanced, dynamic caching in a multi-generator context.*
-   **Interface Implementation Check (`goscan.Implements`, `goscan.compareFieldTypes`):**
    -   Basic signature comparison exists.
    -   Requires more robust type comparison logic, especially for:
        -   Types from different packages (fully qualified name comparison).
        -   Type aliases and their underlying types.
        -   Consideration of `ExternalTypeOverride` during type comparisons.
        -   Correct handling of generic types and their instantiations in signatures.
    -   *Advanced type system utilities like full assignability checks are discussed in [./dream2.md](./dream2.md).*
-   **Method Access from `TypeInfo` for Structs:**
    -   `sketch/ja/from-minigo.md` suggests a desire for easier access to a struct's methods directly from its `TypeInfo`.
    -   Currently, methods are part of `PackageInfo.Functions` and require filtering by receiver type and name. Consider adding a `Methods []*FunctionInfo` field to `StructInfo` or `TypeInfo` for structs.
-   **Package-Level Documentation:**
    -   `scanner.PackageInfo` does not directly store the package's GoDoc comments.
    -   This needs to be extracted from `ast.File.Doc` of the relevant file(s) in the package (often the file with the `package` declaration that has associated comments).
-   **Aggregated Package-Level Imports:**
    -   `scanner.PackageInfo` does not have a direct field listing all unique import paths used by the package.
    -   This requires aggregation from `ast.File.Imports` across all files in `PackageInfo.AstFiles`.
-   **Type Parameter Resolution in Complex Scenarios (Generics):**
    -   Ensuring correct identification and resolution of type parameters (e.g., `T` in `List[T]`) when they are used in method signatures, field types within generic structs, etc., especially with nested generics or multiple levels of type parameterization.
    -   The current implementation in `parseFuncDecl` (for method receivers) and `parseTypeExpr` attempts to handle this but may require more extensive testing and refinement for edge cases.
    -   *Full generics support, including resolving method signatures for instantiated generic types, is a major item. See also [./dream2.md](./dream2.md) for advanced concepts.*

## To Be Implemented (Minigo Driven - from `examples/minigo/improvement.md`)

-   **Extraction of Package-Level Variables:**
    -   `scanner.PackageInfo` should include a list of exported top-level variables.
    -   This would be similar to `ConstantInfo`, potentially `VariableInfo { Name string, FilePath string, Doc string, Type *FieldType, IsExported bool, Node ast.Node }`.
-   **Consolidated Package Information for `minigo/inspect.GetPackageInfo`:**
    -   (Covered above) Direct field for package documentation in `scanner.PackageInfo`.
    -   (Covered above) Direct field for aggregated import list in `scanner.PackageInfo`.
    -   (Covered above) List of package-level variables.
-   **`iota` Evaluation for Constants:** Implement basic logic to correctly evaluate the integer values of constants defined using `iota` (e.g., for simple enums).
    -   *More complex `iota` scenarios and deeper semantic understanding are discussed in [./dream2.md](./dream2.md).*

## Broader Vision & Advanced Features

For a detailed exploration of more ambitious, long-term features such as:
-   Advanced Semantic Analysis (Symbol Tables, DFA/CFG)
-   Rich AST Navigation and Transformation Frameworks
-   Sophisticated GoDoc and Annotation Metaprogramming
-   Next-Generation Code Generation Ecosystem (ScanBroker, DAGs)
-   Ultimate Build and Environment Awareness
-   Interactive and Incremental Scanning
-   And other "dream-like" capabilities...

**Please refer to [./dream2.md](./dream2.md).**

This `todo.md` will focus on the more immediate and concrete enhancements listed above and below.

## Considerations/Known Issues

-   **Recursive Type Information & Circular Dependencies:**
    -   Need robust handling for recursive type definitions and circular dependencies between packages during `FieldType.Resolve()` and other information gathering stages to prevent infinite loops or crashes.
    -   *Advanced theoretical solutions and handling in a multi-generator context are part of the vision in [./dream2.md](./dream2.md).*
-   **Performance for Large Packages:**
    -   Operations like `GetPackageInfo` (if it were to scan and aggregate all data for a large package on demand) could be performance-sensitive. Caching strategies (like the current `packageCache` in `typescanner.Scanner` and the experimental `SymbolCache`) and efficient aggregation are important.
-   **Resolution of Replaced Modules (Module-to-Module):**
    -   The current `locator.FindPackageDir` has limitations in resolving module import paths that are replaced by *other* external modules in `go.mod`. It primarily handles local filesystem replacements or replacements that resolve to paths within the same main module context.
    -   *Full, robust resolution of complex `go.mod` scenarios (including inter-module replacements) is a significant challenge, further discussed in [./dream2.md](./dream2.md).*
-   **Complexity of `ImportManager.Add`:**
    -   The alias generation logic in `ImportManager.Add` has several checks and fallback mechanisms. While aiming for correctness, its complexity might indicate potential edge cases that need thorough testing.
-   **Clarity of Scanner Method Behaviors (`ScanFiles`, `ScanPackageFromFilePath`, `ScanPackageFromImportPath`):**
    -   The interaction between the instance-level `visitedFiles` set, the instance-level `packageCache` (for `ScanPackageFromImportPath` and `ScanPackageFromFilePath`), and the persistent `symbolCache` can be intricate.
    -   The design choice for `ScanFiles` not to update the `packageCache` (as it represents partial information) should be clearly documented for users.
    -   The "no merge" principle for `PackageInfo` objects returned by different scan calls means that obtaining a "complete" or "fully merged" view of a package might require a specific final scan (e.g., `ScanPackageFromImportPath` after other partial scans) or careful orchestration by the user.
-   **Scanning and Resolution of External Dependencies:**
    -   The current `ExternalTypeOverride` mechanism allows treating external types as other Go types. A more advanced (and potentially optional) feature would be to fully scan and resolve types from external dependencies (via `go.mod`).
    -   *This is a significant feature, with performance and complexity implications, further explored in [./dream2.md](./dream2.md).*
