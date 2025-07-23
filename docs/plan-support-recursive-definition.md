# Plan to Support Recursive Type Definitions and Circular Dependencies

This document outlines the plan to enhance `go-scan` to robustly handle recursive type definitions (e.g., a struct containing a field that is a pointer to itself) and circular dependencies between packages during type resolution.

**Status: Not Yet Implemented.** The plan described here is the designated approach, but the code changes have not been made yet. The current implementation is still susceptible to infinite loops.

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

### 1. Introduce a Resolution History Tracker

We will pass a map through the `Resolve` calls to act as a history tracker. This map will maintain a set of type identifiers (e.g., a fully-qualified type name like `"example.com/mymodule/models.User"`) that are currently in the resolution stack for the ongoing resolution chain.

The implementation will use a `map[string]struct{}` for this purpose, as it's slightly more memory-efficient for set-like behavior than `map[string]bool`.

### 2. Modify `FieldType.Resolve()`

The `FieldType.Resolve()` method signature will be updated to accept the resolution tracker.

**Current:**
```go
func (ft *FieldType) Resolve(ctx context.Context) (*TypeInfo, error)
```

**Proposed:**
```go
// In scanner/models.go
func (ft *FieldType) Resolve(ctx context.Context, resolving map[string]struct{}) (*TypeInfo, error) {
    // ...
}
```

The new logic within `Resolve` will be:

1.  **Construct Type Identifier:** Create a unique string identifier for the type being resolved (e.g., `ft.fullImportPath + "." + ft.typeName`).

2.  **Check for Cycles:** At the beginning of the method, check if this identifier already exists in the `resolving` map.
    *   If it does, a cycle is detected. The method should immediately return `nil, nil`. This is not an error. It signals to the caller in the resolution chain that this type is already being processed further up the stack. The caller can then link to the partially-resolved `TypeInfo` that already exists.

3.  **Mark as Resolving:** If no cycle is detected, add the type's identifier to the `resolving` map.
    ```go
    resolving[typeIdentifier] = struct{}{}
    ```

4.  **Defer Cleanup:** Use a `defer` statement to remove the identifier from the `resolving` map before the function returns. This ensures the state is cleaned up for the next independent resolution task.
    ```go
    defer delete(resolving, typeIdentifier)
    ```

5.  **Proceed with Resolution:** Continue with the existing logic (check cache, call `ScanPackageByImport`, etc.).

### 3. Update Call Sites and Entry Points

The `PackageResolver` interface itself doesn't need to change. However, the initial "entry point" for resolution must be updated. We will introduce a public-facing method on the primary `PackageResolver` implementation (`goscan.Scanner`) to start a resolution process.

This new method will create the initial `resolving` map, ensuring that consumers of the library don't need to manage this internal state.

```go
// In goscan.go
// ResolveType starts the type resolution process for a given field type.
// It handles circular dependencies by tracking the resolution path.
func (s *Scanner) ResolveType(ctx context.Context, fieldType *scanner.FieldType) (*scanner.TypeInfo, error) {
    // The internal Resolve method is called with a new, empty map for tracking.
    return fieldType.Resolve(ctx, make(map[string]struct{}))
}
```
All internal calls to `Resolve` must be updated to pass the `resolving` map down the call stack.

### 4. Handling Partially Resolved Types

When a cycle is detected, `Resolve` returns a `nil` `TypeInfo`. The key to this working is that the `TypeInfo` object for a given type must be created and placed in its package's type map *before* its fields are recursively resolved. This ensures that when the resolution process unwinds, the pointer to the `TypeInfo` is valid and can be used, even if its own fields are not fully populated yet. The `scanner.Scanner` logic should be reviewed to confirm this behavior.

### Summary of Changes:

1.  **`scanner/models.go`:**
    *   Update `FieldType.Resolve()` to accept a `map[string]struct{}` parameter.
    *   Implement cycle detection logic at the start of `Resolve()`.
    *   Add the current type to the tracking map and `defer` its removal.

2.  **`goscan.go` (or other `PackageResolver` implementations):**
    *   Provide a clean public API (e.g., `ResolveType`) for starting a resolution that initializes an empty tracking map.
    *   Update any internal call sites of `Resolve()` to pass the tracking map.

3.  **Testing:**
    *   Add new test cases with direct (`type T *T`) and mutual (`A -> B -> A`) recursion scenarios.
    *   Verify that these cases resolve without stack overflows and that the resulting object graph is correctly linked.
