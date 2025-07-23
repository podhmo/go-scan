# Handling of Recursive Type Definitions and Circular Dependencies

This document outlines how `go-scan` robustly handles recursive type definitions (e.g., a struct containing a field that is a pointer to itself) and circular dependencies between packages during type resolution.

**Status: Implemented.** The logic described here is implemented and tested.

## The Challenge: Infinite Loops

Without a mechanism to track the history of type resolutions, a scanner can easily fall into an infinite loop when encountering recursive types or circular dependencies.

### Scenarios

1.  **Direct Recursion:** A struct contains a field that is a pointer to itself. When resolving `Node.Next`, the scanner would be asked to resolve `Node`, which would then require resolving `Node.Next` again, leading to an infinite loop.
    ```go
    type Node struct {
        Value int
        Next  *Node
    }
    ```

2.  **Mutual Recursion:** Two types in different packages refer to each other. Resolving `a.A` requires resolving `b.B`, which in turn requires resolving `a.A`, creating a resolution loop.
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

## Implemented Solution

The solution involves a context-aware resolution mechanism that tracks the resolution path and handles cycles gracefully. The core of this logic resides in the `scanner.FieldType.Resolve()` method.

### 1. Resolution History Tracking

A map, `resolving map[string]struct{}`, is passed through all `Resolve` calls. This map acts as a history tracker for the current resolution chain, storing unique identifiers for each type currently being resolved (e.g., `"example.com/mymodule/models.User"`).

### 2. Cycle Detection in `FieldType.Resolve()`

The `FieldType.Resolve()` method has the following signature:

```go
// In scanner/models.go
func (ft *FieldType) Resolve(ctx context.Context, resolving map[string]struct{}) (*TypeInfo, error)
```

The logic within `Resolve` is as follows:

1.  **Check for Cycles:** At the beginning of the method, it constructs a unique identifier for the current type. It then checks if this identifier already exists in the `resolving` map.
    *   If it does, a cycle is detected. Instead of proceeding, the method immediately looks up the already-allocated (but partially resolved) `TypeInfo` from the package cache and returns a pointer to it. This is the key to correctly linking the nodes in the type graph.

2.  **Mark as Resolving:** If no cycle is detected, it adds the type's identifier to the `resolving` map.

3.  **Defer Cleanup:** A `defer` statement ensures the identifier is removed from the `resolving` map before the function returns, cleaning up the state for the next independent resolution task.

4.  **Proceed with Resolution:** It continues with the standard resolution logic (checking cache, calling `ScanPackageByImport`, etc.).

5.  **Cache Result:** Upon successfully finding a `TypeInfo`, it caches it in `ft.Definition` before returning. This is crucial for subsequent lookups, including those that break cycles.

### 3. Public Entry Point for Resolution

To hide the complexity of managing the `resolving` map, a clean public API is provided on the `goscan.Scanner`:

```go
// In goscan.go
func (s *Scanner) ResolveType(ctx context.Context, fieldType *scanner.FieldType) (*scanner.TypeInfo, error)
```

This method initializes a new, empty `resolving` map for each top-level resolution request, ensuring that resolution chains from different starting points do not interfere with each other.

This implementation allows `go-scan` to correctly navigate complex and recursive type graphs, making it a robust tool for advanced code generation and analysis.
