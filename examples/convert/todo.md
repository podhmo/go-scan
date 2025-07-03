# TODO: Go Type Conversion Code Generation with go-scan

## Main Objective

Utilize the `go-scan` library to generate code for converting from one specific Go type to another.

## Specific Requirements for Conversion Code

*   **Source and Destination Types**: Must support `src` (source) and `dst` (destination) structs.
*   **Field Handling**:
    *   Must handle `embedded` fields.
    *   Must support arbitrary field names (i.e., names can differ between `src` and `dst`).
*   **Context Propagation**: Conversion functions should be able to accept a `context.Context` argument.
*   **Internal Processing**: Allow for the integration of internal processing steps within the conversion logic (e.g., translating text from English to Japanese).
*   **Conversion Function Visibility**:
    *   Conversion functions for specified "top-level types" should be `exported`.
    *   Conversion functions for other internal types should be `unexported`.
    *   The criteria for what constitutes a "top-level type" are yet to be determined.

## Current Challenges / Open Questions

### 1. Location of Conversion Functions

*   **Options**:
    *   Define in a separate package.
    *   Define as methods on either the source type or the destination type.
*   **Primary Use Case**: Conversion to DTOs (Data Transfer Objects).
*   **Reason for Conversion**: To handle subtle differences in structure (e.g., a field defined as a pointer in `src` becomes a value type in `dst`). The behavior for `nil` pointer-to-value conversions needs further detailing.

### 2. Describing `src` to `dst` Relationships (Core Issue for this Example)

*   **Goal**: Find a convenient way to describe these relationships for `go-scan`.
*   **Alternatives Considered**:
    *   A DSL similar to `minigo` (a subset of Go).
    *   Using marker comments (e.g., specific keywords in comments).
*   **Current Leaning**: A `minigo`-like approach is preferred over marker comments because:
    *   Marker comments lack IDE autocompletion.
    *   Typos in marker comments would likely lead to runtime errors, which are harder to detect than compile-time errors potentially offered by a DSL.
