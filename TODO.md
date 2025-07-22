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
-   **Improved Scanning Logic in Example Tools:** Command-line tools in `examples/` now handle file and directory paths more intelligently, distinguishing between them and grouping file arguments by package.

## To Be Implemented

- [ ] **Implement Overlay Feature**
  - *Description:* Add an "overlay" feature to `go-scan` to allow providing in-memory file content, useful for tools that generate or modify Go source code without writing to the filesystem.
  - *Plan Document:* [docs/plan-overlay.md](./docs/plan-overlay.md)
  - Subtasks:
    - [ ] Define `scanner.Overlay` type.
    - [ ] Update `locator.Locator` to accept and use the overlay for `go.mod`.
    - [ ] Update `scanner.Scanner` to accept and use the overlay for source files.
    - [ ] Implement overlay key resolution (project-relative paths).

- [ ] **Implement `scantest` library**
  - *Description:* Implement the `scantest` library to provide a testing harness for `go-scan` based tools, as detailed in the plan.
  - *Plan Document:* [docs/plan-scantest.md](./docs/plan-scantest.md)
  - Subtasks:
    - [ ] Implement `scantest.WriteFiles` to set up test environments in temporary directories.
    - [ ] Implement the core `scantest.Run` function.
    - [ ] Implement the file change detection logic within `Run` to capture side effects.
    - [ ] Add comprehensive tests for the `scantest` library itself.

- [ ] **Enhance `go-scan` for `scantest` integration**
  - *Description:* Modify `go-scan`'s top-level functions to support the interception mechanism required by `scantest`.
  - *Plan Document:* [docs/plan-scantest.md](./docs/plan-scantest.md) (see "Run Function and go-scan Integration" section)
  - Subtasks:
    - [ ] Design a mechanism for passing an I/O interceptor via `context.Context`.
    - [ ] Update `goscan.WriteFile` (and similar functions) to check the context for the interceptor and use it if present. This ensures that file writes during tests are captured by `scantest` instead of hitting the disk.
    - [ ] Ensure the changes are backward-compatible for users not using `scantest`.
