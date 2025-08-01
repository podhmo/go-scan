# TODO

> **Note on updating this file:**
> -   Do not move individual tasks to the "Implemented" section.
> -   A whole feature section (e.g., "convert Tool Implementation") should only be moved to "Implemented" when all of its sub-tasks are complete.
> -   For partially completed features, use checkboxes (`[x]` for complete, `[-]` for partially complete). A feature is considered partially complete if it has been implemented but has associated tests that are currently disabled.
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
-   **External and Standard Library Package Resolution**: Added an option (`WithGoModuleResolver`) to enable resolving packages from the Go standard library (`GOROOT`) and external dependencies listed in `go.mod` (from `GOMODCACHE`). This allows the scanner to "see" types outside the main module without relying on `go/packages`.
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
-   **Improved Import Management**: Handle import alias collisions robustly. By pre-registering types with the `ImportManager` and using `golang.org/x/tools/imports` for final output formatting, the generator now correctly handles complex import scenarios and avoids unused imports.
-   **`convert` Tool Implementation**: as described in [docs/plan-neo-convert.md](docs/plan-neo-convert.md)
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
        -   [x] Implement validator rules (`"<DstType>", validator=<funcName>`).
    -   [x] **Add Tests for `// convert:rule`**: Write tests for global conversion and validator rules.
    -   [x] **Implement `// convert:variable`**: Support for declaring local variables (`// convert:variable <name> <type>`) within the generated function's scope, enabling stateful operations across multiple `using` functions (e.g., sharing a `strings.Builder`).
    -   [x] **Implement `// convert:computed`**: Support for computing destination fields from a Go expression (`// convert:computed <FieldName> = <GoExpression>`), useful for combining fields.
    -   [x] **Error Handling with `errorCollector`**: Implement the `errorCollector` struct and generate code that uses it to report multiple conversion errors.
    -   [x] **Add Tests for Error Handling**: Write tests to verify that `errorCollector` correctly accumulates and reports errors.
    -   [x] **Improve Generated Code Error Handling**: Replace `// TODO: proper error handling` placeholders in the generator with more robust error handling, even if it's not the full `errorCollector` implementation.
    -   [x] **Parse `max_errors` from Annotation**: Implement parsing for the `max_errors` option in the `@derivingconvert` annotation.
    -   [x] **Handle Map Key Conversion**: Implement logic to convert map keys when the source and destination map key types are different.
    -   [x] **Implement automatic field selection for untagged fields**: Use `json` tag as a fallback for field name matching (priority: `convert` tag > `json` tag > normalized field name).
    -   [x] **Support assignment for assignable embedded fields**
    -   [x] **Add Tests for `max_errors` and Map Key Conversion**: Write integration tests for the `max_errors` and map key conversion features.
    -   [x] **Support `replace` directives in `go.mod`**: Enhanced `go-scan`'s dependency resolution to correctly handle `replace` directives in `go.mod` files.
    -   [x] **Implement `// convert:import` annotation**: Introduce a new global annotation (`// convert:import <alias> <path>`) to allow `using` and `validator` rules to reference functions from external packages. This will remove the current limitation that requires these functions to be in the same package as the generated code.
        -   [x] Update the parser to recognize and process the `// convert:import` annotation.
        -   [x] Ensure the parser registers the specified alias and path with the `ImportManager`.
        -   [x] Modify the `using` and `validator` logic to correctly resolve function references that use these imported aliases (e.g., `pkg.MyFunc`).
-   **Fix for Standard Library Scanning in Tests**: Resolved the `mismatched package names` error that occurred when scanning standard library packages (like `time`) from within a test binary. This was achieved by enhancing the `ExternalTypeOverride` mechanism to accept synthetic `scanner.TypeInfo` objects, allowing tools to bypass problematic scans without hardcoding workarounds in their parsers.
-   **On-Demand, Multi-Package AST Scanning**: All features based on the plan in [docs/plan-multi-package-handling.md](./docs/plan-multi-package-handling.md) have been implemented, including core library logic, consumer updates, and CI checks.
-   **Generator Logic Enhancements**:
    -   **Recursive Converter Generation**: The generator now automatically creates converters for nested struct types.
    -   **Pointer-Aware Global Rules**: Global conversion rules now correctly apply to pointer types (e.g., a rule for `T` -> `S` also works for `*T` -> `*S`).
    -   **Improved Type Resolution for Generics/Pointers**: The core scanner now correctly populates the `Elem` and `TypeArgs` fields for pointer types, resolving previous inconsistencies.
-   **CLI and Build**:
    -   Fixed the underlying scanner and generator bugs that caused the `make e2e` command to fail.
    -   The `go install` command in the `e2e` target has been updated to `go build`.
    -   The main `test` target now incorporates the `e2e` tests.
-   **Enum-like Constant Scanning**: Implemented scanning for idiomatic Go enums (a custom type with a group of related constants). This includes package-level discovery and linking constants to their type, and it correctly handles `iota`. The feature is detailed in [docs/plan-scan-enum.md](./docs/plan-scan-enum.md).
    -   [x] Modify `scanner/models.go` to support enum members.
    -   [x] Implement Package-Level Discovery for enums.
    -   [x] Implement Lazy Symbol-Based Lookup for enums (via the existing package resolver).
    -   [x] Add Tests for Both Enum Scanning Strategies.
-   **Unified Single-Pass Generator**: Combined the `derivingjson` and `derivingbind` tools into a single, efficient `deriving-all` command. This tool parses source files only once and uses refactored, composable generator functions to produce the combined output. The implementation followed the plan in [docs/plan-walk-once.md](./docs/plan-walk-once.md).
    -   [x] Refactored `derivingjson` and `derivingbind` to separate generation logic from file I/O.
    -   [x] Implemented the `deriving-all` orchestrator tool.
    -   [x] Added integration tests for the unified tool using `scantest`.
-   **`derivingjson` Tool**:
    -   [x] `UnmarshalJSON` generation for `oneOf` style interfaces using `@deriving:unmarshall`.
    -   [x] `MarshalJSON` generation for `oneOf` concrete types using `@derivingmarshall` to add a type discriminator.

## To Be Implemented

### `convert` Tool Future Tasks ([docs/plan-neo-convert.md](./docs/plan-neo-convert.md))
- [ ] **Expand Test Coverage**: Create a comprehensive test suite that verifies all features and edge cases, including the new import functionality.
- [ ] **Complete `README.md`**: Write user-facing documentation with installation, usage, and examples.

### `scantest` Library Future Work ([docs/plan-scantest.md](./docs/plan-scantest.md))
- [x] Implement file change detection to verify modifications to existing files during tests.
