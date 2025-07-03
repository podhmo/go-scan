```markdown
# TODO List for go-scan

This document outlines the current status, planned features, and areas for improvement for the `go-scan` library. It is based on analysis of the existing codebase, README, and specific use-case documents like `examples/minigo/improvement.md` and `docs/ja/from-minigo.md`.

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
-   **AST Iteration Utilities:** `astwalk.ToplevelStructs`.

## Partially Implemented/Experimental/Needs Refinement

-   **Symbol Definition Cache (Experimental):**
    -   Currently marked as experimental in README (`cache.SymbolCache`, `Scanner.CachePath`, `FindSymbolDefinitionLocation`).
    -   Needs further testing and refinement for cache invalidation strategies (e.g., handling file content modifications, not just existence/deletion) and overall robustness.
-   **Interface Implementation Check (`goscan.Implements`, `goscan.compareFieldTypes`):**
    -   Basic signature comparison exists.
    -   Requires more robust type comparison logic, especially for:
        -   Types from different packages (fully qualified name comparison).
        -   Type aliases and their underlying types.
        -   Consideration of `ExternalTypeOverride` during type comparisons.
        -   Correct handling of generic types and their instantiations in signatures.
-   **`scanner.FunctionInfo.IsVariadic`:**
    -   This flag is not explicitly present in `scanner.FunctionInfo`.
    -   Information is available from `ast.FuncType.Params.List[last].Ellipsis` and should be populated.
-   **Method Access from `TypeInfo` for Structs:**
    -   `docs/ja/from-minigo.md` suggests a desire for easier access to a struct's methods directly from its `TypeInfo`.
    -   Currently, methods are part of `PackageInfo.Functions` and require filtering by receiver type and name. Consider adding a `Methods []*FunctionInfo` field to `StructInfo` or `TypeInfo` for structs.
-   **Package-Level Documentation:**
    -   `scanner.PackageInfo` does not directly store the package's GoDoc comments.
    -   This needs to be extracted from `ast.File.Doc` of the relevant file(s) in the package (often the file with the `package` declaration that has associated comments).
-   **Aggregated Package-Level Imports:**
    -   `scanner.PackageInfo` does not have a direct field listing all unique import paths used by the package.
    -   This requires aggregation from `ast.File.Imports` across all files in `PackageInfo.AstFiles`.
-   **Type Parameter Resolution in Complex Scenarios:**
    -   Ensuring correct identification and resolution of type parameters (e.g., `T` in `List[T]`) when they are used in method signatures, field types within generic structs, etc., especially with nested generics or multiple levels of type parameterization.
    -   The current implementation in `parseFuncDecl` (for method receivers) and `parseTypeExpr` attempts to handle this but may require more extensive testing and refinement for edge cases.

## To Be Implemented (Minigo Driven - from `examples/minigo/improvement.md`)

-   **`scanner.FunctionInfo.IsVariadic` flag:** (Covered above) Explicitly add and populate this boolean flag.
-   **Extraction of Package-Level Variables:**
    -   `scanner.PackageInfo` should include a list of exported top-level variables.
    -   This would be similar to `ConstantInfo`, potentially `VariableInfo { Name string, FilePath string, Doc string, Type *FieldType, IsExported bool, Node ast.Node }`.
-   **Consolidated Package Information for `minigo/inspect.GetPackageInfo`:**
    -   (Covered above) Direct field for package documentation in `scanner.PackageInfo`.
    -   (Covered above) Direct field for aggregated import list in `scanner.PackageInfo`.
    -   (Covered above) List of package-level variables.

## To Be Implemented (Broader Vision - from `docs/ja/from-minigo.md`)

-   **Source Code Context API:**
    -   Implement an API like `scanner.GetSourceContext(pos token.Pos, window int) SourceContext` (or similar) to retrieve source code snippets around a given position for enhanced error reporting and diagnostics.
-   **Symbol Table and Scope Analysis Support:**
    -   Develop features to assist in identifying symbol definitions, references, and their scope relationships (e.g., lexical scope, visibility, shadowing).
-   **Advanced GoDoc Parsing:**
    -   Enhance GoDoc parsing beyond the current basic `TypeInfo.Annotation()` to structurally parse common GoDoc tags (e.g., `@param <name> <description>`, `@return <description>`, `@see <symbol>`) and user-defined structured annotations.
-   **Advanced Type System Utilities:**
    -   (Covered above) More robust interface implementation checks.
    -   Resolution of method signatures for fully instantiated generic types (e.g., determining the concrete signature of `Add(int)` for `List[int].Add(T)`).
    -   Type compatibility and assignability checks (e.g., `isTypeAssignableTo(typeA, typeB)`).
-   **AST Traversal and Transformation Utilities:**
    -   Provide higher-level utilities for querying and manipulating ASTs, such as XPath-like queries, CSS selector-style matching for nodes, or helpers for common AST transformation patterns.
-   **Auxiliary Analysis Features:**
    -   **Control Flow Graph (CFG) / Data Flow Analysis (DFA) Foundations:** Provide basic information or utilities that could serve as a foundation for building CFGs or performing DFA (e.g., for unused variable detection, reachability analysis).
    -   **Incremental/Partial Scanning Enhancements:** Improve capabilities for incremental scanning beyond the current file-level symbol caching, potentially for faster feedback in REPLs or large projects.
    -   **Build Tag and `go:generate` Directive Awareness:** Implement recognition of build tags and `go:generate` directives to allow the scanner to consider conditional compilation and code generation aspects.

## Considerations/Known Issues

-   **Recursive Type Information & Circular Dependencies:**
    -   Need robust handling for recursive type definitions and circular dependencies between packages during `FieldType.Resolve()` and other information gathering stages to prevent infinite loops or crashes.
-   **Performance for Large Packages:**
    -   Operations like `GetPackageInfo` (if it were to scan and aggregate all data for a large package on demand) could be performance-sensitive. Caching strategies and efficient aggregation are important.
-   **Resolution of Replaced Modules (Module-to-Module):**
    -   The current `locator.FindPackageDir` has limitations in resolving module import paths that are replaced by *other* external modules in `go.mod`. It primarily handles local filesystem replacements or replacements that resolve to paths within the same main module context.
-   **Complexity of `ImportManager.Add`:**
    -   The alias generation logic in `ImportManager.Add` has several checks and fallback mechanisms. While aiming for correctness, its complexity might indicate potential edge cases that need thorough testing.
-   **Clarity of Scanner Method Behaviors (`ScanFiles`, `ScanPackage`, `ScanPackageByImport`):**
    -   The interaction between the instance-level `visitedFiles` set, the instance-level `packageCache` (for `ScanPackageByImport` and `ScanPackage`), and the persistent `symbolCache` can be intricate.
    -   The design choice for `ScanFiles` not to update the `packageCache` (as it represents partial information) should be clearly documented for users.
    -   The "no merge" principle for `PackageInfo` objects returned by different scan calls means that obtaining a "complete" or "fully merged" view of a package might require a specific final scan (e.g., `ScanPackageByImport` after other partial scans) or careful orchestration by the user.
```
