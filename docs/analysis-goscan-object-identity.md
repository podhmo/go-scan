# Analysis of Object Identity in go-scan

This document analyzes the object identity model within the `go-scan` ecosystem, particularly concerning objects derived from AST declarations (`ast.Decl`). It identifies a key issue where the current implementation does not guarantee that two objects representing the same source code declaration are the same instance in memory. This document proposes a solution to enforce this identity.

The primary goal is to establish a premise where consumers of `go-scan`, such as `symgo`, can safely assume that if two objects (e.g., `*scanner.FunctionInfo`) are derived from the same underlying `ast.Decl`, they are the same object instance (i.e., their pointers are equal).

## 1. Investigation Summary

The investigation covered three main components: `symgo`, `locator`, and `go-scan` (specifically the `scanner` and top-level `goscan` packages).

### `symgo` (`docs/analysis-symgo-implementation.md`)

-   `symgo` is a primary consumer of `go-scan`. It uses `scanner.PackageInfo`, `scanner.FunctionInfo`, and other objects to perform symbolic execution.
-   Its effectiveness and simplicity would be greatly improved if it could rely on object identity. For example, associating metadata with a function object (e.g., tracking visited functions in a call graph) would be trivial if `func DoSomething()` always resolved to the same `*FunctionInfo` instance. Without this guarantee, `symgo` must use less convenient keys, such as the `ast.Node` pointer, to manage its state.

### `locator` (`locator/locator.go`)

-   The `locator` package resolves Go import paths to filesystem directory paths. It is a utility that provides paths to the scanner and does not participate in AST creation or semantic object generation.
-   **Conclusion**: The `locator` is not the source of the object identity issue.

### `scanner` (`scanner/scanner.go`)

-   This is the core parsing engine. The `scanner.Scanner` type takes file paths, parses them, and traverses the resulting AST.
-   During traversal, it creates semantic objects "from scratch" on every scan:
    -   `parseFuncDecl` creates a new `*scanner.FunctionInfo` for each `*ast.FuncDecl`.
    -   `scanGoFiles` (within its loop) creates a new `*scanner.TypeInfo` placeholder for each `*ast.TypeSpec`.
-   The `scanner.Scanner` is intentionally stateless regarding the identity of the objects it produces. It holds no long-term cache that maps a declaration to a `scanner` object. This is the direct source of the problem.

### `goscan` (`goscan.go`)

-   The top-level `goscan.Scanner` acts as a session or workspace manager. It holds a `packageCache` (a `map[string]*Package`) which caches the `*scanner.PackageInfo` result of a scan to avoid re-parsing files for the same import path.
-   However, this cache is at the package level. It does not guarantee the identity of individual objects *within* the package. If a tool triggers a re-scan or a different analysis path processes the same file again, new, distinct `*FunctionInfo` and `*TypeInfo` objects will be created for the same declarations.

## 2. The Core Problem: Lack of Object Interning

The root of the issue is the absence of an "interning" mechanism for AST-derived objects. The stateless nature of the `scanner.Scanner` means that every time it encounters an AST declaration, it creates a new corresponding semantic object.

Consider this workflow:
1. A tool calls `goscan.Scanner.ScanPackageFromImportPath(ctx, "example.com/foo")`.
2. `goscan` finds the package, and its internal `scanner.Scanner` parses the files.
3. For a function `func Bar()`, the `scanner` creates a `*FunctionInfo` instance, let's call it `F1`.
4. Later, another analysis path within the same `goscan.Scanner` session needs to process the same package or file again.
5. The `scanner.Scanner` will create a *new* `*FunctionInfo` instance, `F2`, for `func Bar()`.
6. Although `F1.AstDecl.Pos() == F2.AstDecl.Pos()`, the object instances themselves are different: `F1 != F2`.

This forces consumers like `symgo` to use workarounds, making their implementation more complex and less reliable.

## 3. Proposed Solution: A Session-Level IdentityMap

To solve this, we will introduce a centralized caching layer that ensures a one-to-one mapping between a source code declaration and its corresponding `scanner` object. This cache, which we'll call `IdentityMap`, will be managed at the session level, corresponding to the lifecycle of the top-level `goscan.Scanner`.

### Keying Strategy: `token.Pos`

The key for the `IdentityMap` will be `token.Pos`. This choice is deliberate for several reasons:
- **Robustness**: `token.Pos` is an integer value representing a byte offset within the `token.FileSet`. It is a stable and reliable identifier for a declaration's position in the source code.
- **Uniqueness**: Within a single `token.FileSet` (which is managed by `goscan.Scanner` for the entire session), the `Pos()` of each declaration (`ast.Decl`) is unique.
- **Efficiency**: Using an integer (`token.Pos`) as a map key is more efficient than using a pointer (`ast.Node`).
- **Simplicity**: The position is easily retrieved from any AST node via the `node.Pos()` method.

### Implementation Plan

The plan involves three main steps:

#### 1. Define `IdentityMap`
A new file, `scanner/identity_map.go`, will define the `IdentityMap` struct. It will be responsible for thread-safe access to the underlying maps.

```go
// In scanner/identity_map.go (conceptual)
package scanner

import (
	"go/token"
	"sync"
)

// IdentityMap provides a session-wide cache to ensure that each AST declaration
// maps to a single, unique semantic object (*FunctionInfo, *TypeInfo, etc.).
type IdentityMap struct {
	mu        sync.RWMutex
	Functions map[token.Pos]*FunctionInfo
	Types     map[token.Pos]*TypeInfo
	// Potentially add Constants/Variables later if needed
}

// NewIdentityMap creates a new, initialized IdentityMap.
func NewIdentityMap() *IdentityMap {
	return &IdentityMap{
		Functions: make(map[token.Pos]*FunctionInfo),
		Types:     make(map[token.Pos]*TypeInfo),
	}
}

// ... methods for Get/Set for each type ...
```

#### 2. Integrate `IdentityMap` into the Scanners

-   **`goscan.go`**: The `goscan.Scanner` struct will be augmented with an `IdentityMap` field.
    ```go
    // In goscan.go
    type Scanner struct {
        // ... existing fields
        identityMap *scanner.IdentityMap
    }

    // In goscan.New()
    s := &Scanner{
        // ...
        identityMap: scanner.NewIdentityMap(),
    }
    ```
-   **`scanner.go`**: The internal `scanner.Scanner` will receive a reference to the `IdentityMap`.
    ```go
    // In scanner.go
    type Scanner struct {
        // ... existing fields
        identityMap *IdentityMap
    }

    // In goscan.New(), when creating the internal scanner:
    initialScanner, err := scanner.New(s.fset, ..., s.identityMap, ...)
    ```

#### 3. Modify Object Creation Logic in `scanner.Scanner`

The core logic change happens in `scanner/scanner.go` where objects are created. The pattern will be "check-and-get or create-and-set."

-   **For `TypeInfo` (in `scanGoFiles`)**:
    The loop that processes `*ast.TypeSpec` will be modified.
    ```go
    // In scanner.Scanner.scanGoFiles
    // ... inside loop over decls ...
    if ts, ok := spec.(*ast.TypeSpec); ok {
        pos := ts.Pos()
        if typeInfo := s.identityMap.GetType(pos); typeInfo != nil {
            // Found in cache, use it.
            info.Types = append(info.Types, typeInfo)
            continue
        }

        // Not in cache, create new, then register it.
        typeInfo := &TypeInfo{ /* ... initialize ... */ }
        s.identityMap.SetType(pos, typeInfo)
        info.Types = append(info.Types, typeInfo)
    }
    ```

-   **For `FunctionInfo` (in `parseFuncDecl`)**:
    The function will be modified to follow the same pattern.
    ```go
    // In scanner.Scanner.parseFuncDecl
    pos := f.Pos()
    if funcInfo := s.identityMap.GetFunction(pos); funcInfo != nil {
        // Found in cache, return it.
        return funcInfo
    }

    // Not in cache, create new, then register it.
    funcInfo := &FunctionInfo{ /* ... initialize ... */ }
    s.identityMap.SetFunction(pos, funcInfo)
    // ... rest of the function logic ...
    return funcInfo
    ```

### Benefits of this Approach

-   **Guaranteed Identity**: Consumers like `symgo` can safely use object pointers as unique identifiers, simplifying state management and improving algorithm correctness.
-   **Centralized Logic**: The identity management is centralized within the `goscan.Scanner` session, and the implementation details are localized to the `scanner` package.
-   **Efficiency**: Reduces object allocations and garbage collection overhead in scenarios involving re-scans of the same files.
-   **Simpler Consumers**: Drastically simplifies the logic in tools built on top of `go-scan`, as they no longer need to implement their own identity tracking mechanisms.