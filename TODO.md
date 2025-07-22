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

## To Be Implemented

- [ ] **Implement scantest library**
  - *Description:* Implement the `scantest` library as described in `docs/plan-scantest.md`.
  - *Plan Document:* [docs/plan-scantest.md](./docs/plan-scantest.md)
  - Subtasks:
    - [ ] Implement the `Run` function.
    - [ ] Implement the `WriteFiles` function.
- [ ] **Use scantest for testing**
  - *Description:* Create example tests using the `scantest` library to demonstrate its usage and integrate it into the project's testing workflow.
  - Subtasks:
    - [ ] Create a new test suite for an existing analyzer using `scantest`.
    - [ ] Integrate `scantest`-based tests into the CI/CD pipeline.

- [x] **Implement Improved Scanning Logic in Example Tools**
  - *Description:* The command-line tools in `examples/` have been updated to handle file and directory paths more intelligently, as outlined in the proposal. This involves distinguishing between file and directory arguments and grouping multiple file arguments by package.
  - *Proposal Document:* [docs/plan-scan-improvement.md](./docs/plan-scan-improvement.md)
  - Subtasks:
    - [x] Refactor `examples/derivingjson`: Modified `main.go` to implement the proposed scanning logic.
    - [x] Refactor `examples/derivingbind`: Modified `main.go` to implement the same logic.
    - [x] Verify Behavior: Manually verified that the tools correctly handle single-file, multi-file, and directory inputs.
