# TODO

This file tracks implemented features and immediate, concrete tasks.

For more ambitious, long-term features, see [docs/near-future.md](./docs/near-future.md).

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
-   **Improved Scanning Logic in Example Tools:** Command-line tools in `examples/` now handle file and directory paths more intelligently, distinguishing between them and grouping file arguments by package. This was implemented as described in [docs/plan-scan-improvement.md](./docs/plan-scan-improvement.md).
-   **Testing Harness (`scantest`):** Implemented the `scantest` library to provide a testing harness for `go-scan` based tools. The implementation, detailed in [docs/plan-scantest.md](./docs/plan-scantest.md), uses a significant enhancement not in the original plan: I/O operations are intercepted via `context.Context`. This allows `scantest` to capture file generation output in memory without altering the tool's own code, a key difference from initial concepts.
-   **In-Memory File Overlay:** Added an "overlay" feature to `go-scan` to allow providing in-memory file content. This is useful for tools that generate or modify Go source code without writing to the filesystem. This was implemented as described in [docs/plan-overlay.md](./docs/plan-overlay.md).
-   **Integration Tests for Examples:** Added integration tests for the code generation tools in the `examples/` directory using the new `scantest` library.

## To Be Implemented

-   **Handle Recursive Type Definitions and Circular Dependencies:**
    -   **Description:** Enhance the type resolution logic in `FieldType.Resolve()` to gracefully handle recursive type definitions and circular dependencies between packages, preventing infinite loops.
    -   **Proposal Document:** [./docs/plan-support-recursive-definition.md](./docs/plan-support-recursive-definition.md)
    -   **Subtasks:**
        1.  **Introduce Resolution Context:** Modify `FieldType.Resolve()` to accept a context or map for tracking in-progress resolutions.
        2.  **Implement Cycle Detection:** Add logic to detect cycles by checking if a type is already in the resolution context.
        3.  **Update Call Sites:** Refactor internal calls to `Resolve()` to pass the new context.
        4.  **Add Tests:** Create test cases with direct and mutual recursion to validate the solution.
