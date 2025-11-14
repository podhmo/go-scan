> [!NOTE]
> This feature has been implemented.

# Design Doc: Re-implementing `genschema` with `go-scan`

**Author:** Jules
**Status:** Implemented
**Created:** 2025-11-14
**Last updated:** 2025-11-14

## 1. Introduction

This document outlines the plan to re-implement the `genschema` tool. The original tool, located at `github.com/podhmo/genschema`, generates JSON Schema from Go struct definitions. Its implementation relies on the `go/packages` and `go/types` libraries, which perform eager loading of package information.

The goal of this project is to replace the core type analysis engine with `go-scan`, our in-house AST-based scanner. This change will align `genschema` with the conventions and constraints of our broader toolkit, which prohibits the direct use of `go/packages` and `go/types` in favor of `go-scan`'s lazy, on-demand resolution capabilities.

## 2. Goals and Non-Goals

### Goals

-   Replace all usage of `go/packages` and `go/types` with `go-scan`.
-   Achieve functional parity with the original `genschema` tool. This includes supporting all existing command-line flags and schema generation logic (e.g., struct tag parsing, circular reference handling).
-   The new implementation will be located under `tools/genschema/`.

### Non-Goals

-   Adding new features or flags to the `genschema` tool.
-   Changing the output JSON Schema format. The output should be compatible with the original tool.

## 3. High-Level Design

The new `genschema` will follow the same overall structure as the original but will be built on `go-scan`'s types.

1.  **CLI & Configuration:** The command-line interface will be managed by the `flagstruct` library, just like the original. All existing flags (`--query`, `--loose`, `--name-tag`, etc.) will be preserved.
2.  **Type Loading:** Instead of using `packages.Load()` to load type information, we will use `scanner.NewScanner().Scan()`. The input `query` (e.g., `example.com/mypackage.MyStruct`) will be parsed to identify the target package and type name.
3.  **Schema Generation:** A new `Generator` struct will be implemented. It will recursively traverse the type graph, starting from the target type provided by the user. This `Generator` will operate on `go-scan`'s `scanner.ObjectInfo` types instead of `go/types.Type`. It will handle structs, primitives, slices, maps, pointers, and named types, including the detection of circular dependencies to generate `$ref` definitions.

## 4. Detailed Design

### Main Logic Flow

1.  Parse command-line arguments into a `Config` struct.
2.  Initialize `scanner.NewScanner()` with appropriate options.
3.  Parse the `--query` argument to get the package path and type name.
4.  Use the scanner to find the `scanner.PackageInfo` for the given package path.
5.  Look up the target `scanner.ObjectInfo` (representing the root type) within the package's scope.
6.  Create an instance of the `Generator`.
7.  Call `generator.Generate(rootTypeInfo)` to start the schema generation process.
8.  The `Generator` will produce a main schema object and a map of definitions (`$defs`).
9.  Assemble the final JSON Schema, including `$schema`, `title`, and `description` from the CLI options.
10. Marshal the final schema to JSON and print it to standard output.

### The `Generator` Component

The core of the logic will reside in a `Generator` struct.

```go
type Generator struct {
    Config    *Config
    Scanner   *scanner.Scanner
    defs      map[string]*orderedmap.OrderedMap // To store generated definitions for $ref
    seen      map[*scanner.ObjectInfo]string    // For circular dependency detection
    useCounts map[string]int                    // To track usage of definitions
}
```

The main method will be `Generate(info *scanner.ObjectInfo) (*orderedmap.OrderedMap, error)`. This method will be recursive and use a `switch` on the `info.Kind` to handle different Go types.

-   **`scanner.KindStruct`**:
    -   Iterate through `info.Struct.Fields`.
    -   For each exported field, parse its struct tag (`json`, `required`, `jsonschema-override`, etc.) to determine the property name and attributes. The logic for parsing tags can be largely adapted from the original `genschema`.
    -   Recursively call `Generate()` on the field's type (`field.Type.ObjectInfo`).
    -   Collect property definitions and a list of required fields.
    -   Read field comments from `field.Doc` to use as the `description`.

-   **`scanner.KindNamed`**:
    -   This handles named types (e.g., `type MyInt int`).
    -   Check the `seen` map to detect circular references. If a type has been seen, return a `$ref` to its definition.
    -   If it's the first time seeing this type, add it to the `seen` map.
    -   Recursively call `Generate()` on its underlying type (`info.Named.Underlying.ObjectInfo`).
    -   Store the result in the `defs` map with a unique name.
    -   Return a schema object containing a `$ref` to the stored definition (e.g., `{"$ref": "#/$defs/MyType"}`).

-   **`scanner.KindSlice` / `scanner.KindArray`**:
    -   Generate a schema of `type: "array"`.
    -   Recursively call `Generate()` on the element type (`info.Slice.Elem.ObjectInfo`) to create the `items` schema.

-   **`scanner.KindMap`**:
    -   Generate a schema of `type: "object"`.
    -   Recursively call `Generate()` on the map's value type (`info.Map.Value.ObjectInfo`) to create the `additionalProperties` schema.

-   **`scanner.KindPointer`**:
    -   The schema is generated from the element type (`info.Pointer.Elem.ObjectInfo`). Fields with pointer types are considered optional by default during struct processing.

-   **`scanner.KindBasic`**:
    -   Map the Go basic type (`string`, `int`, `bool`, etc.) to the corresponding JSON Schema type (`"string"`, `"integer"`, `"boolean"`).

-   **`scanner.KindInterface`**:
    -   For `interface{}`, generate a generic object schema (`type: "object", additionalProperties: true`), consistent with the original tool.

## 5. Implementation Plan

1.  **Step 1: Scaffolding.** Create the `tools/genschema/main.go` file with the CLI flag parsing and basic application structure.
2.  **Step 2: Type Loading.** Implement the logic to use `go-scan` to load the `scanner.ObjectInfo` for the type specified in the `--query` flag.
3.  **Step 3: Core Generator.** Implement the `Generator` struct and the `Generate` method. Start with support for basic types and structs without complex features.
4.  **Step 4: Add Composite Types.** Extend the `Generator` to handle slices, maps, and pointers.
5.  **Step 5: Handle Named Types and `$ref`.** Implement the logic for named types, including circular reference detection and the creation of shared definitions under `$defs`.
6.  **Step 6: Implement Tag and Comment Parsing.** Add the detailed logic for parsing struct tags (`json`, `required`, `jsonschema-override`) and extracting descriptions from doc comments.
7.  **Step 7: Testing.** Develop a suite of tests to verify the generated schema against various Go struct definitions, covering all supported features and edge cases.
8.  **Step 8: Finalization.** Ensure all CLI options are respected and the output matches the original tool's format.

This plan provides a clear path to re-implementing `genschema` using `go-scan`, ensuring it integrates seamlessly into our existing tooling ecosystem.
