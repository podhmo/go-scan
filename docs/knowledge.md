# Knowledge Base

## Testing Go Modules with Nested `go.mod` Files

When a subdirectory within a Go module contains its own `go.mod` file, it effectively becomes a nested or sub-module. This is sometimes done intentionally, for example, to conduct acceptance tests where the sub-module mimics an independent consumer of the parent module's packages, or to manage a distinct set of dependencies for a specific part of the project (like examples or tools).

### Running Tests in a Nested Module

If you try to run tests for packages within such a nested module from the parent module's root directory using standard package path patterns (e.g., `go test ./path/to/nested/module/...`), you might encounter errors like "no required module provides package" or other resolution issues. This happens because the Go toolchain gets confused about which `go.mod` file to use as the context.

To correctly run tests for a nested module:

1.  **Change Directory**: Navigate into the root directory of the nested module (i.e., the directory containing its specific `go.mod` file).
    ```bash
    cd path/to/nested/module
    ```

2.  **Run Tests**: Execute `go test` commands from within this directory. You can specify packages relative to this nested module's root.
    ```bash
    # To test all packages within the nested module
    go test ./...

    # To test a specific sub-package within the nested module
    go test ./subpackage
    ```

**Example from this Repository (`go-scan`):**

The `examples/derivingjson` directory in this repository contains its own `go.mod` file. This is intentional, designed to simulate an acceptance test environment where `derivingjson` (and its generated code) is treated as if it were a separate module consuming functionalities from the main `go-scan` module.

To test the models within `examples/derivingjson/models`:

```bash
cd examples/derivingjson
go test ./models
```

**Important Note on `examples` Directory `go.mod`:**

Please do not delete the `go.mod` file located in example directories (like `examples/derivingjson/go.mod`). These are specifically set up to ensure that the examples can be built and tested as if they were separate modules, which is crucial for acceptance testing of the main library's features from an external perspective.

This approach ensures that the tests accurately reflect how an external consumer would use the library and helps catch integration issues that might not be apparent when testing everything within a single module context.

## Using Go 1.24+ Iterator Functions (Range-Over-Function)

### Context

For the `astwalk` package, specifically the `ToplevelStructs` function, a decision was made to return an iterator function compatible with Go's "range-over-function" feature (stabilized in Go 1.24). The function signature is `func(fset *token.FileSet, file *ast.File) func(yield func(*ast.TypeSpec) bool)`.

### Rationale

This approach was chosen to leverage modern Go idioms for iterating over AST nodes, offering potential benefits in ergonomics and efficiency, especially for large datasets.

- **Ergonomics**: The `for ... range` syntax over a function call is idiomatic and readable.
- **Efficiency**: Iterators process items one by one, which can be more memory-efficient than allocating a slice for all items upfront.
- **Lazy Evaluation**: Work to find the next item is done only when requested.

### Implementation Notes

The `ToplevelStructs` function in the `astwalk` package provides an iterator for top-level struct type specifications within a Go source file.

- **Usage**: It can be used with a `for...range` loop in Go 1.24 and later.
- **Go Version Dependency**: This feature requires Go 1.24 or newer. The main module's `go.mod` file is set to `go 1.24`.

### Conclusion

The use of the range-over-function pattern for `ToplevelStructs` aligns with modern Go practices, offering a clean and efficient way to process AST nodes. Users of the `astwalk` package should ensure their environment uses Go 1.24 or a later version.

---

## Centralized Import Management in Code Generators: `ImportManager`

**Context:**

When developing multiple code generators (e.g., `examples/derivingjson`, `examples/derivingbind`), a common challenge arises in managing `import` statements for the generated Go code. Each generator needs to:
1.  Identify all necessary packages to import based on the types and functions used in the generated code.
2.  Assign appropriate package aliases, especially to avoid conflicts if multiple imported packages have the same base name (e.g., `pkg1/errors` and `pkg2/errors`).
3.  Ensure that generated aliases do not clash with Go keywords (e.g., `type`, `range`).
4.  Handle sanitization of package names that might not be valid Go identifiers if used directly as aliases (e.g., paths containing dots or hyphens like `example.com/ext.pkg`).
5.  Produce a final list of import paths and their aliases for inclusion in the generated file.

Implementing this logic independently in each generator leads to code duplication, potential inconsistencies, and an increased likelihood of bugs.

**Solution: `goscan.ImportManager`**

To address this, a utility struct `goscan.ImportManager` was introduced within the `go-scan` library itself. This provides a centralized and reusable solution for import management.

**Key Features and Design Decisions:**

*   **Centralized Logic**: `ImportManager` encapsulates all the common logic for adding imports, resolving alias conflicts, and formatting the final import list.
*   **`Add(path string, requestedAlias string) string`**:
    *   Registers an import path.
    *   If `requestedAlias` is empty, it derives a base alias from the last element of the import path.
    *   **Sanitization**: It sanitizes the base alias by replacing common non-identifier characters like `.` and `-` with `_` (e.g., `ext.pkg` becomes `ext_pkg`).
    *   **Keyword Handling**: If the sanitized alias is a Go keyword (e.g., `range`), it appends `_pkg` (e.g., `range_pkg`). This check is performed *before* the identifier validity check to ensure keywords are handled correctly even if they are valid identifiers.
    *   **Identifier Validity**: If the (potentially keyword-adjusted) alias is still not a valid Go identifier (e.g., starts with a number, or was empty after sanitization), it prepends `pkg_`. A fallback to a hash-based name is included for extreme edge cases where the alias becomes problematic (e.g. just "pkg_").
    *   **Conflict Resolution**: If the final candidate alias is already in use by a different import path, it appends a numeric suffix (e.g., `myalias1`, `myalias2`) until a unique alias is found.
    *   Returns the actual alias to be used in qualified type names.
    *   If the `path` is the same as the `currentPackagePath` (the package for which code is being generated), it returns an empty string, as types from the current package do not need qualification.
*   **`Qualify(packagePath string, typeName string) string`**:
    *   A convenience method that calls `Add(packagePath, "")` internally to ensure the package is registered and to get its managed alias.
    *   Returns the correctly qualified type name (e.g., `alias.TypeName` or just `TypeName` if it's a local type or built-in).
*   **`Imports() map[string]string`**:
    *   Returns a map of all registered import paths to their final, resolved aliases, suitable for passing to `goscan.GoFile`.

**Benefits:**

*   **Reduced Boilerplate**: Generators no longer need to implement their own complex import tracking and alias resolution logic.
*   **Consistency**: Ensures a consistent approach to alias generation and conflict handling across different generators using `go-scan`.
*   **Improved Robustness**: Centralized logic is easier to test thoroughly, leading to more reliable import management. The `ImportManager` includes specific handling for keywords, invalid identifiers, and common path-based naming issues.
*   **Simpler Generator Code**: The client code in generators (like `examples/derivingjson/main.go`) becomes cleaner as they can delegate import path and type qualification to the `ImportManager`.

**Application:**

The `ImportManager` was successfully integrated into `examples/derivingjson/main.go` and `examples/derivingbind/main.go`, significantly simplifying their import management sections. The development process involved iterative refinement of the alias generation rules within `ImportManager.Add` based on test cases covering various scenarios (keyword clashes, dot/hyphen in package names, alias conflicts).

---

## Refactoring `examples/convert2` to Use `go-scan`

**Context:**

The `examples/convert2` tool, a code generator for struct conversions, initially used `go/parser` directly to parse Go source files and build its internal model of types and conversion rules. This refactoring effort aimed to replace the direct `go/parser` usage with the `go-scan` library (the parent module) to leverage its AST parsing capabilities and potentially simplify `convert2`'s parser logic.

**Key Changes and Learnings:**

1.  **Integration of `go-scan`:**
    *   The `convert2/parser.ParseDirectory` function was modified to use `goscan.New()` to initialize a `go-scan` scanner and then `scanner.ScanPackage()` to get `scannermodel.PackageInfo`.
    *   This provided `convert2` with structured information about types (`scannermodel.TypeInfo`), fields (`scannermodel.FieldInfo`), and access to AST nodes (`PackageInfo.AstFiles`, `TypeInfo.Node`) without needing to manage `go/parser` or `token.FileSet` directly within `convert2`.

2.  **Type Information Mapping (`convertScannerTypeToModelType`):**
    *   `go-scan` provides type information primarily through `scannermodel.FieldType` (for field types, underlying types of aliases, etc.) and `scannermodel.TypeInfo` (for type definitions).
    *   A crucial part of the refactoring was creating a new helper function, `convertScannerTypeToModelType`, in `convert2/parser.go`. This function translates `scannermodel.FieldType` instances into `convert2`'s own `model.TypeInfo` structure.
    *   **Responsibility for Resolution**: `go-scan` performs AST-level parsing but not full `go/types`-style type resolution. This means that while `go-scan` can identify that a field is `*foo.Bar`, the `convert2` layer is still responsible for:
        *   Determining the simple name ("Bar"), package alias ("foo"), and full import path for "foo".
        *   Constructing the `FullName` (e.g., "path/to/foo.Bar") for `model.TypeInfo`.
        *   Handling built-in types, pointers, slices, and maps by recursively calling `convertScannerTypeToModelType` for element/key/value types.
    *   The `scannermodel.FieldType` provides `Name` (which can be "pkg.Type" or "Type"), `PkgName` (alias), and `FullImportPath()`. Logic was added to correctly derive the simple name and fully qualified name for `model.TypeInfo`.
    *   A specific challenge was handling pointer types: `go-scan`'s `FieldType` for `*T` has `IsPointer=true` and its `Name` relates to `T`. The `convertScannerTypeToModelType` function had to be careful to construct the `Elem` field of `model.TypeInfo` for `*T` by effectively describing `T` based on the information in the `FieldType` for `*T`.

3.  **Comment Directive Parsing:**
    *   `go-scan`'s `PackageInfo.AstFiles` provides access to the `*ast.File` nodes. This allowed `convert2` to continue using its existing logic for parsing `// convert:pair` and `// convert:rule` directives from comments (`fileAst.Doc`, `fileAst.Comments`).
    *   The `resolveTypeFromString` function in `convert2/parser.go`, which resolves type names found in comment directives (e.g., `"mypkg.MyType"`), was rewritten. Instead of parsing the string to an `ast.Expr` and then using a complex AST-based resolver, it now:
        *   Handles prefixes like `*`, `[]`, `map[]` through string manipulation and recursive calls.
        *   For identifiers, it checks if the type is a known basic type.
        *   For local types (e.g., "MyType" or "currentPackageName.MyType"), it looks up the type in the `parsedInfo.Structs` and `parsedInfo.NamedTypes` maps (which were populated from `go-scan`'s output).
        *   For external types (e.g., "alias.ExtType"), it uses the file's import map (passed to it) to find the full import path for "alias" and constructs a `model.TypeInfo` representing the external type.

4.  **Struct Field Tag Processing:**
    *   `scannermodel.FieldInfo` (from `go-scan`) provides the full struct tag string via its `Tag` field.
    *   A helper `extractConvertTag(fullTag string) string` was added to `convert2/parser.go` to specifically extract the content of the `convert:"..."` part from the full tag string.
    *   The existing `parseFieldTag(convertContent string) model.ConvertTag` function then parses this extracted content. This removed the need for `convert2` to access the `*ast.Field` node directly for tag parsing.

5.  **`go.mod` Configuration:**
    *   Since `examples/convert2` now imports `github.com/podhmo/go-scan` (the parent module), its `go.mod` file required a `replace` directive: `replace github.com/podhmo/go-scan => ../../`.

6.  **Testing and Debugging Insights:**
    *   The refactoring significantly changed how type information is gathered. Iterative testing (`make test` within `examples/convert2`) was essential.
    *   Initial test failures in the generated code were traced back to how `model.TypeInfo` (especially `FullName` and `Elem` for pointers) was being populated by `convertScannerTypeToModelType`.
    *   Temporarily adding detailed debug prints (as comments) into the code generator (`generator.go`) to inspect the `model.TypeInfo` fields for problematic conversions was key to diagnosing mismatches in `elementsMatch` conditions.

**Outcome:**

The refactoring successfully integrated `go-scan` as the primary parsing engine for `examples/convert2`, simplifying its direct AST manipulation logic. While `convert2` still retains significant responsibility for interpreting `go-scan`'s output into its specific internal model, the foundational parsing is now delegated. The process highlighted the importance of clear contracts for the type information provided by `go-scan` and meticulous mapping in the consuming tool.

**Troubleshooting Notes for `convert2` Refactoring:**

*   **`undefined: variable` error**: During development, encountered perplexing compiler errors where a variable defined in an outer scope was reported as `undefined` on lines where it was being reassigned within an inner `if` block. This was resolved by ensuring the variable name was absolutely identical (no typos or invisible characters) across its definition and all uses/assignments. Using a temporary variable for intermediate results before assigning back to the main variable in the `if` block also helped clarify the scope and assignment.
*   **`scannermodel.FieldType` interpretation**: Correctly interpreting the fields of `go-scan`'s `scannermodel.FieldType` (like `Name`, `PkgName`, and how `Elem` is used for pointers vs. slices/maps) was crucial for the `convertScannerTypeToModelType` translator function. For pointer types (`*T`), `FieldType.IsPointer` is true, and `FieldType.Name` refers to `T`'s name (possibly qualified); `FieldType.Elem` is *not* used for the pointed-to type in this case for `go-scan`'s model. `convert2`'s `model.TypeInfo` for `*T`, however, *does* use an `Elem` field to describe `T`. This mapping required careful implementation.
*   **Remaining Warnings**: Some warnings like `WARN Unhandled type expression type=*ast.Ellipsis` and `WARN Package alias not found in file imports alias=simple typeStr=simple.MyTime` (for specific global rule patterns) may still appear during parsing. The ellipsis warning likely originates from `go-scan`'s parsing of standard library or other dependencies and doesn't affect `convert2`'s current test cases. The package alias warning for global rules might indicate that `resolveTypeFromString` could be further improved for types qualified with the current package's own name in rule definitions. These did not break tests but point to areas for future robustness improvements in the parser or `go-scan`.
