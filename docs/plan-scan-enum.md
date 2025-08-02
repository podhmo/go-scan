# Plan: Enum-like Constant Scanning Feature

## 1. Introduction

This document outlines the plan to add a feature to `go-scan` for detecting and grouping "enum-like" constant definitions. In Go, enums are not a built-in language feature but are conventionally implemented using a custom type and a group of related constants. The goal is to make `go-scan` recognize these conventions and present the constants as members of their corresponding type.

This will allow tools built on `go-scan` to easily understand and work with these idiomatic Go enums, for example, when generating OpenAPI specifications or data validation logic.

## 2. Definition of an Enum

For the purpose of this feature, an "enum" is defined by the following pattern:

1.  A **type definition**, typically an alias for an integer (`int`, `uint8`, etc.) or `string` type.
    ```go
    // The type that defines the enum
    type Status int
    ```
2.  One or more **constant definitions** that are explicitly typed with the enum's type. These constants are considered the "members" of the enum.
    ```go
    // The members of the Status enum
    const (
        Unknown Status = iota // value is 0
        Todo                  // value is 1
        Done                  // value is 2
    )
    ```

### Key Characteristics to Recognize:

-   The constants must have an **explicit type** that matches a type defined within the same package.
-   The constants can be defined in a single `const (...)` block or across multiple separate `const` declarations. As long as they share the same type, they belong to the same enum.
-   Untyped constants (e.g., `const MaxRetries = 5`) will not be considered part of any enum.

## 3. API and Retrieval Methods

There are two primary ways this feature could be exposed to a developer:

### Method 1: Package-Level Discovery

This is the primary, discovery-focused approach. The scanner processes an entire package and automatically identifies all types that fit the enum pattern.

-   **Use Case:** A code generator that needs to find all enums in a package to create OpenAPI schemas, validation rules, or string-conversion methods for all of them.
-   **Implementation:** This is achieved by the two-pass scanning process described in section 5. The results are embedded directly into the `PackageInfo` structure.

### Method 2: Symbol-Specific Lookup

This is a targeted lookup approach. The developer already knows the type they are interested in and wants to retrieve its enum members.

-   **Use Case:** A tool that is analyzing a specific struct field and, upon finding its type is, for example, `models.Status`, needs to look up the possible values for `Status`.
-   **Implementation:** This can be built on top of the data gathered by Method 1. A helper function could be provided:
    ```go
    // Example of a possible helper function
    func GetEnumMembers(scanner *goscan.Scanner, typeName string) ([]*scanner.ConstantInfo, error) {
        // 1. Resolve the type symbol to get its TypeInfo.
        // 2. Check if TypeInfo.IsEnum is true.
        // 3. Return TypeInfo.EnumMembers.
    }
    ```

This plan focuses on implementing the foundational **Method 1**, which in turn enables the future implementation of **Method 2**.

## 4. Proposed Data Structure Changes

To support this feature, the following changes will be made to the data models in `scanner/models.go`.

### `scanner.TypeInfo`

A new field will be added to the `TypeInfo` struct to hold the enum members. An additional boolean flag will make it easy to identify enum types.

```go
// scanner/models.go

type TypeInfo struct {
	Name       string           `json:"name"`
	PkgPath    string           `json:"pkgPath"`
	// ... existing fields ...
	Underlying *FieldType       `json:"underlying,omitempty"` // For alias types

	// --- New Fields ---
	IsEnum      bool            `json:"isEnum,omitempty"`      // True if this type is identified as an enum
	EnumMembers []*ConstantInfo `json:"enumMembers,omitempty"` // List of constants belonging to this enum type
}
```

-   `IsEnum`: A boolean flag that will be set to `true` if one or more constants of this type are found in the package. This provides a quick way for consumers to check if a type is an enum.
-   `EnumMembers`: A slice to store pointers to the `ConstantInfo` structs that are members of this enum. This creates a direct link from the enum type to its values.

No changes are required for `ConstantInfo`.

## 5. Proposed Scanning Process Changes

The current scanning process is a single pass that populates `PackageInfo.Types` and `PackageInfo.Constants` independently. To reliably associate constants with their types, a second "linking" pass is required after the initial scan of a package is complete.

The updated process will be:

1.  **First Pass (Existing Logic):** The `scanGoFiles` function in `scanner/scanner.go` will parse all `.go` files in the package as it currently does. It will populate `info.Types` and `info.Constants` with all the types and constants found. At this stage, there is no link between them.

2.  **Second Pass (New Logic):** After the file parsing loop in `scanGoFiles` is finished, a new private method will be called, for example, `info.resolveEnums()`. This method will implement the linking logic.

### `resolveEnums()` Method Logic:

This method will be added to `PackageInfo`.

```go
// Can be a method on PackageInfo
func (p *PackageInfo) resolveEnums() {
    // The type lookup is already efficient thanks to PackageInfo.Lookup()

    for _, c := range p.Constants {
        // Skip constants without an explicit type or with a built-in type.
        if c.Type == nil || c.Type.IsBuiltin || c.Type.FullImportPath == "" {
            continue
        }

        // We only care about constants whose type is defined in the current package.
        if c.Type.FullImportPath != p.ImportPath {
            continue
        }

        // Find the TypeInfo for this constant's type.
        typeName := c.Type.TypeName
        typeInfo := p.Lookup(typeName)

        if typeInfo != nil {
            // Found the corresponding type. Link the constant to it.
            typeInfo.IsEnum = true
            typeInfo.EnumMembers = append(typeInfo.EnumMembers, c)
        }
    }
}
```

This method will be called at the end of `scanGoFiles` in `scanner/scanner.go` before returning the `PackageInfo`.

```go
// scanner/scanner.go -> scanGoFiles(...)

    // ... end of the for loop iterating through files ...

    // NEW: Perform enum linking
    info.resolveEnums() // Assuming it's a method on PackageInfo

    return info, nil
}
```

## 6. Implementation Steps

1.  **Modify `scanner/models.go`:**
    -   Add the `IsEnum bool` and `EnumMembers []*ConstantInfo` fields to the `TypeInfo` struct.

2.  **Implement the Linking Logic:**
    -   Create a new method `resolveEnums()` on `PackageInfo` in `scanner/models.go` (or as a private function in `scanner/scanner.go` that takes `*PackageInfo`). The logic will be as described in section 5.

3.  **Update the Scanner:**
    -   In `scanner/scanner.go`, call the new linking function/method at the end of `scanGoFiles` before returning the `PackageInfo`.

4.  **Add Unit and Integration Tests:**
    -   Create a new test file, e.g., `goscan_enum_test.go`.
    -   Add test cases in the `testdata` directory with various enum patterns:
        -   A simple enum with `iota`.
        -   A string-based enum.
        -   Constants for one enum defined across multiple `const` blocks.
        -   A file with multiple different enum types.
        -   A file with types and constants that should *not* be matched as enums.
    -   The tests should scan the testdata and assert that:
        -   `TypeInfo.IsEnum` is correctly set.
        -   `TypeInfo.EnumMembers` contains the correct `ConstantInfo` objects.
        -   The correct number of enums and members are found.

## 7. Considerations

-   **Cross-Package Enums:** The proposed logic only links constants to types defined within the same package. Associating a constant with a type from an external package is out of scope for this initial implementation.
-   **Performance:** The linking step involves one loop over all constants in the package. For a typical package, this should have a negligible impact on performance.
-   **Sorting:** The `EnumMembers` will be appended in the order that constants are found. If a consistent order is required (e.g., sorted by value or name), this should be explicitly handled.
-   **Targeted Lookup Assumption:** For the symbol-specific lookup (Method 2), it could be assumed that the enum's type definition and its constant values reside in the same file. The `TypeInfo` struct contains the `FilePath`, making it possible to get the file path for a type and then perform a targeted search for `const` declarations within that file only. This could be an optimization for the targeted lookup, but the package-level scan must check all files in the package.
