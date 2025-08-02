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

## 3. Core Implementation Strategies

To align with `go-scan`'s design philosophy of lazy and partial scanning, two distinct and independent methods will be implemented to retrieve enum information.

### Strategy 1: Package-Level Discovery

This strategy is designed for tools that need to discover all enum definitions within an entire package.

-   **Use Case:** An OpenAPI generator that needs to define schemas for all enums in a `models` package.
-   **Implementation:**
    1.  Perform a full scan of all `.go` files in the target package, populating `PackageInfo.Types` and `PackageInfo.Constants`.
    2.  Perform a second, "linking" pass that iterates over the collected constants.
    3.  For each constant with an explicit type, it finds the corresponding `TypeInfo` within the same package and appends the constant to the `TypeInfo.EnumMembers` slice.
    4.  This populates the `PackageInfo` with a complete map of all enums in the package.

### Strategy 2: Lazy Symbol-Based Lookup

This strategy is for targeted lookups of a specific enum type, designed to be fast and avoid unnecessary scanning. **This method does not depend on a prior package-level scan.**

-   **Use Case:** A tool analyzing a struct field of type `models.Status` needs to find the members of `Status` without scanning the entire `models` package.
-   **Implementation:**
    1.  The user provides a fully qualified type name (e.g., `"github.com/project/models.Status"`).
    2.  The scanner uses its internal resolver and cache (if enabled) to find the absolute file path where the `Status` symbol is defined.
    3.  The scanner then parses **only that single file's AST**.
    4.  It traverses the AST of that file to find the `TypeSpec` for `Status` and all `const` declarations.
    5.  It identifies the constants that are explicitly typed as `Status` and returns them as the enum members.

This dual approach provides both broad discovery and efficient, targeted lookups, respecting the library's core principles.

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

## 5. Implementation Steps

1.  **Modify `scanner/models.go`:**
    -   Add `IsEnum bool` and `EnumMembers []*ConstantInfo` to `TypeInfo`.

2.  **Implement Package-Level Discovery (Strategy 1):**
    -   Implement the linking logic (e.g., `resolveEnums()`) and call it at the end of the existing `scanGoFiles` function in `scanner/scanner.go`.

3.  **Implement Lazy Symbol-Based Lookup (Strategy 2):**
    -   Create a new public method on the scanner, e.g., `ScanEnumMembers(ctx context.Context, typeSymbol string) ([]*ConstantInfo, error)`.
    -   This method will implement the single-file parsing logic as described in Strategy 2. It will leverage `FindSymbolDefinitionLocation` or a similar mechanism to locate the file.

4.  **Add Tests for Both Strategies:**
    -   Create `goscan_enum_test.go`.
    -   Add tests for the package-level discovery.
    -   Add separate tests specifically for the lazy lookup, ensuring it only parses one file and correctly retrieves members.

## 6. Considerations
-   **Cache Importance:** The efficiency of the Lazy Symbol-Based Lookup is highly dependent on the symbol cache being enabled and populated, as this allows for a quick mapping from symbol to file path.
-   **Cross-Package Enums:** Both strategies will initially only support enums where the type and constants are defined within the same package.
-   **Sorting:** The `EnumMembers` will be appended in the order that constants are found. If a consistent order is required (e.g., sorted by value or name), this should be explicitly handled.
