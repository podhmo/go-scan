# Plan to Support Recursive Type Definitions and Circular Dependencies

This document outlines the plan to enhance `go-scan` to robustly handle recursive type definitions (e.g., a struct containing a field that is a pointer to itself) and circular dependencies between packages during type resolution.

## Current Implementation and Limitations

The current type resolution mechanism is implemented in the `FieldType.Resolve()` method in `scanner/models.go`. It works as follows:

1.  It checks if the type is already resolved and cached in `FieldType.Definition`.
2.  If not, it uses the `PackageResolver` (typically a `goscan.Scanner` instance) to scan the package where the type is defined using `ScanPackageByImport()`.
3.  It then searches for the type by name within the scanned package's `Types` list.
4.  If found, it caches the resulting `TypeInfo` in `FieldType.Definition` and returns it.

The primary limitation of this approach is its susceptibility to infinite loops when encountering recursive types or circular package dependencies.

### Scenarios

1.  **Direct Recursion:** A struct contains a field that is a pointer to itself.
    ```go
    type Node struct {
        Value int
        Next  *Node
    }
    ```
    When resolving `Node.Next`, `Resolve()` for `*Node` will be called, which in turn needs to resolve `Node`, leading back to itself.

2.  **Mutual Recursion:** Two types refer to each other.
    ```go
    // package a
    type A struct {
        B *b.B
    }

    // package b
    type B struct {
        A *a.A
    }
    ```
    Resolving `a.A` requires resolving `b.B`, which in turn requires resolving `a.A`, creating a resolution loop.

The current implementation lacks a mechanism to detect and handle these cycles. It does not track the "in-progress" state of resolutions, so when a cycle occurs, it re-enters the resolution process for the same type, leading to an infinite recursion and eventually a stack overflow.

## Proposed Implementation Plan

To address this, we will introduce a context-aware resolution mechanism that tracks the resolution path and handles cycles gracefully. The core idea is to modify `FieldType.Resolve()` to detect when it is asked to resolve a type that is already in the process of being resolved.

### 1. Introduce a Resolution Context

We will add a resolution context (or a similar tracking mechanism) to be passed through the `Resolve` calls. This context will maintain a set of type identifiers (e.g., fully-qualified type names) that are currently in the resolution stack.

A simple way to implement this is by adding a `map[string]bool` to the `Resolve` method's parameters, or by using a `context.Context` to carry this information.

### 2. Modify `FieldType.Resolve()`

The `FieldType.Resolve()` method signature will be updated to accept the resolution context.

```go
// In scanner/models.go
func (ft *FieldType) Resolve(ctx context.Context, resolving map[string]bool) (*TypeInfo, error) {
    // ...
}
```

The new logic within `Resolve` will be:

1.  **Check for Cycles:** At the beginning of the method, construct a unique identifier for the current type being resolved (e.g., `ft.fullImportPath + "." + ft.typeName`). Check if this identifier is already in the `resolving` map.
    *   If it is present, a cycle is detected. Instead of proceeding, the method should immediately return the currently available (but possibly incomplete) `TypeInfo` if one has been partially constructed, or `nil` if not. It should **not** return an error, as a recursive reference is not an error condition. The partially constructed `TypeInfo` is crucial for the caller to link to.

2.  **Mark as Resolving:** If no cycle is detected, add the type's identifier to the `resolving` map.

3.  **Defer Cleanup:** Use a `defer` statement to remove the identifier from the `resolving` map before the function returns. This ensures the state is cleaned up regardless of whether the resolution succeeds or fails.

4.  **Proceed with Resolution:** Continue with the existing resolution logic (check cache, call `ScanPackageByImport`, etc.). When making a recursive call to `Resolve()` for a sub-type (e.g., a struct field's type), pass the same `resolving` map down.

### 3. Update the `PackageResolver` Interface and Call Sites

The `PackageResolver` interface itself doesn't need to change, but the entry points that trigger resolution will need to initialize the resolution context. The initial call to `Resolve` from outside the resolution process will start with an empty `resolving` map.

For example, a new public-facing method on `goscan.Scanner` could be introduced to hide this implementation detail:

```go
// In goscan.go (or another appropriate public-facing package)
func (s *Scanner) ResolveType(ctx context.Context, fieldType *scanner.FieldType) (*scanner.TypeInfo, error) {
    return fieldType.Resolve(ctx, make(map[string]bool))
}
```

### 4. Handling Partially Resolved Types

When a cycle is detected, `Resolve` returns a `TypeInfo` that may not be fully populated yet (e.g., its own fields might not be resolved). This is acceptable. The key is that the pointer to the `TypeInfo` object itself is returned. As the resolution stack unwinds, the fields of this `TypeInfo` will be filled in. Because the pointer is shared, all references to it will see the fully populated object once the entire resolution process completes.

For this to work correctly, the `TypeInfo` object must be created and placed in the package's type map *before* its fields are recursively resolved. The `scanner.Scanner` logic should be reviewed to ensure this is the case.

### Summary of Changes:

1.  **`scanner/models.go`:**
    *   Update `FieldType.Resolve()` to accept a `map[string]bool` parameter for tracking in-progress resolutions.
    *   Implement cycle detection logic at the start of `Resolve()`.
    *   Add the current type to the tracking map and defer its removal.
    *   Pass the tracking map down in any recursive `Resolve()` calls.

2.  **Call Sites:**
    *   Update all internal call sites of `Resolve()` to pass the tracking map.
    *   Provide a clean public API for starting a resolution that initializes an empty tracking map.

3.  **Testing:**
    *   Add new test cases with direct and mutual recursion scenarios to verify that the cycle detection works and that types are resolved correctly without stack overflows.
    *   Test cases should include:
        *   A recursive `Node` struct.
        *   Two packages with types that reference each other.
        *   A complex scenario with multiple levels of dependencies.

This plan allows `go-scan` to correctly navigate complex type graphs, making it more robust and reliable for advanced code generation and analysis tasks.
