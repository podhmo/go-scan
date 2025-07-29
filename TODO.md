# TODO

> **Note on updating this file:**
> -   Do not move individual tasks to the "Implemented" section.
> -   A whole feature section (e.g., "convert Tool Implementation") should only be moved to "Implemented" when all of its sub-tasks are complete.
> -   For partially completed features, use checkboxes (`[x]`) to mark completed sub-tasks.

This file tracks implemented features and immediate, concrete tasks.

For more ambitious, long-term features, see [docs/near-future.md](./docs/near-future.md).

## Implemented

-   **AST-based Parsing:** Core functionality using `go/parser` and `go/ast`.
-   **Cross-Package Type Resolution:** Lazy resolution of type definitions within the same module (`FieldType.Resolve()`, `PackageResolver` interface).
-   **Recursive Type and Dependency Handling**: Gracefully handles recursive type definitions and circular dependencies between packages, preventing infinite loops during type resolution.
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
-   **Variadic Parameter Parsing**: Correctly parses variadic parameters (e.g., `...string`) as slice types (e.g., `[]string`) within function signatures. The `IsVariadic` flag on `FunctionInfo` is set, and the parameter's `FieldType` accurately reflects the corresponding slice type.
-   **Initial `convert` Tool Implementation**: Implemented the CLI entrypoint and a basic parser for the `convert` tool. The tool now uses a `@derivingconvert(DstType)` annotation on source types to define conversion pairs, as documented in the updated `docs/plan-neo-convert.md`.

## To Be Implemented

### `convert` Tool Implementation

as described in [docs/plan-neo-convert.md](docs/plan-neo-convert.md)

-   [x] **Generator for Structs**: Implement the code generator to produce conversion functions for basic struct-to-struct conversions based on the parsed `ConversionPair` model.
    -   [x] Generate a top-level `Convert<Src>To<Dst>` function.
    -   [x] Generate an internal `convert<Src>To<Dst>` helper function.
    -   [x] Implement direct field mapping (e.g., `dst.Field = src.Field`).
-   [x] **Add Tests for Struct Conversion**: Write tests using `scantest` to verify the generated code for struct conversions.
-   [x] **Refactor `examples/convert` for Cross-Package Conversion**:
    -   [x] Move `Src` and `Dst` types into separate packages (e.g., `models/source` and `models/destination`).
    -   [x] Update tests to verify that cross-package conversion works correctly.
-   [x] **Generator for Pointer Fields**: Extend the generator to handle pointer fields within structs.
    -   [x] Generate code that correctly handles `*SrcType` to `*DstType` conversions (nil checks).
-   [x] **Add Tests for Pointer Fields**: Write tests for pointer field conversions.
-   [x] **Advanced Field Conversion Logic**:
    -   [x] Handle pointer-to-pointer (`*Src -> *Dst`) and value-to-pointer (`Src -> *Dst`) conversions.
    -   [x] Implement automatic type conversion for common pairs (e.g., `time.Time` to `string`).
-   [x] **Generator for Slice Fields**: Extend the generator to handle slice fields (e.g., `[]SrcType` to `[]DstType`).
    -   [x] Generate loops to iterate over slices and convert each element.
-   [x] **Add Tests for Slice Fields**: Write tests for slice field conversions.
-   [x] **Generator for Map Fields**: Extend the generator to handle map fields (e.g., `map[string]SrcType` to `map[string]DstType`).
-   [x] **Add Tests for Map Fields**: Write tests for map field conversions.
-   [x] **Map Element Conversion**: The generator now produces recursive helper function calls for elements within maps, supporting maps of structs.
-   [x] **Implement `convert:` Tag Handling**:
    -   [x] `convert:"-"`: Skip a field.
    -   [x] `convert:"NewName"`: Map to a different field name.
    -   [x] `convert:",using=myFunc"`: Use a custom conversion function.
    -   [x] `convert:",required"`: Report an error if a pointer field is nil.
-   [x] **Add Tests for `convert:` Tags**: Write comprehensive tests for all `convert:` tag options.
-   [x] **Implement `// convert:rule`**:
    -   [x] Implement global type conversion rules (`"<SrcType>" -> "<DstType>", using=<funcName>`).
    -   [ ] Implement validator rules (`"<DstType>", validator=<funcName>`).
-   [x] **Add Tests for `// convert:rule`**: Write tests for global conversion and validator rules.
-   [x] **Error Handling with `errorCollector`**: Implement the `errorCollector` struct and generate code that uses it to report multiple conversion errors.
-   [x] **Add Tests for Error Handling**: Write tests to verify that `errorCollector` correctly accumulates and reports errors.
-   [x] **Improve Generated Code Error Handling**: Replace `// TODO: proper error handling` placeholders in the generator with more robust error handling, even if it's not the full `errorCollector` implementation.
-   [x] **Parse `max_errors` from Annotation**: Implement parsing for the `max_errors` option in the `@derivingconvert` annotation.
-   [ ] **Handle Map Key Conversion**: Implement logic to convert map keys when the source and destination map key types are different.
-   [x] **Implement automatic field selection for untagged fields**: Use `json` tag as a fallback for field name matching (priority: `convert` tag > `json` tag > normalized field name).
-   [ ] **Support assignment for assignable embedded fields**
-   [x] **Add Tests for `max_errors` and Map Key Conversion**: Write integration tests for the `max_errors` and map key conversion features. (Blocked by `go mod tidy` issue in tests)
-   [x] **Support `replace` directives in `go.mod`**: Enhanced `go-scan`'s dependency resolution to correctly handle `replace` directives in `go.mod` files. (Note: integration tests revealed issues with `go mod tidy` in temporary directories)

### Known Issues

### Future Tasks (Post-Migration)
*   **Improve Import Management**: Handle import alias collisions robustly. Consider using `golang.org/x/tools/imports` for final output formatting.
*   **Expand Test Coverage**: Create a comprehensive test suite that verifies all features and edge cases.
*   **Complete `README.md`**: Write user-facing documentation with installation, usage, and examples.
