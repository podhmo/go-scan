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

- [ ] **Implement scantest library**
  - *Description:* Implement the `scantest` library as described in `docs/plan-scantest.md`.
  - *Plan Document:* [docs/plan-scantest.md](./docs/plan-scantest.md)
  - Subtasks:
    - [ ] Implement the `WriteFiles` function to set up test files.
    - [ ] Implement the `Run` function to execute `go-scan`.
        - [ ] Add logic to detect `go.mod` in the test directory and use it if present.
        - [ ] Ensure it falls back to the project's `go.mod` if no local `go.mod` is found.
- [ ] **Use scantest for testing**
  - *Description:* Create example tests using the `scantest` library to demonstrate its usage and integrate it into the project's testing workflow.
  - Subtasks:
    - [ ] Create a new test suite for an existing analyzer using `scantest`.
    - [ ] Integrate `scantest`-based tests into the CI/CD pipeline.
