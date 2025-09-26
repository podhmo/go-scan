# Analysis of Object Identity in go-scan and symgo

This document analyzes the object identity model within the `go-scan` ecosystem, particularly concerning objects derived from AST declarations (`ast.Decl`). It identifies a key issue where the current implementation does not guarantee that two objects representing the same source code declaration are the same instance in memory. This document proposes solutions to enforce this identity for both `go-scan`'s semantic objects and `symgo`'s function objects.

The primary goal is to establish a premise where consumers can safely assume that if two objects (e.g., `*scanner.FunctionInfo` or `*object.Function`) are derived from the same underlying `ast.Decl`, they are the same object instance (i.e., their pointers are equal).

## Part 1: Identity in `go-scan`

### 1.1. Investigation Summary

The investigation covered `symgo`, `locator`, and `go-scan` (`scanner` and `goscan` packages). The root cause of identity issues was traced to the `scanner.Scanner`, which, by design, was stateless and created new semantic objects (`*scanner.FunctionInfo`, `*scanner.TypeInfo`) on every scan. The top-level `goscan.Scanner` cached packages, but not the individual objects within them.

### 1.2. The Core Problem: Lack of Object Interning

The root of the issue is the absence of an "interning" mechanism for AST-derived objects. The stateless nature of the `scanner.Scanner` means that every time it encounters an AST declaration, it creates a new corresponding semantic object. This forces consumers like `symgo` to use workarounds, making their implementation more complex and less reliable.

### 1.3. Proposed Solution: A Session-Level IdentityMap

To solve this, we introduce a centralized caching layer that ensures a one-to-one mapping between a source code declaration and its corresponding `scanner` object. This cache, `IdentityMap`, is managed at the session level by the top-level `goscan.Scanner`.

#### Keying Strategy: `token.Pos`

The key for the `IdentityMap` is `token.Pos`. This choice is deliberate for several reasons:
- **Robustness**: `token.Pos` is an integer value representing a byte offset within the `token.FileSet`. It is a stable and reliable identifier for a declaration's position.
- **Uniqueness**: Within a single `token.FileSet` (managed by `goscan.Scanner` for the entire session), the `Pos()` of each declaration is unique.
- **Efficiency**: Using an integer (`token.Pos`) as a map key is more efficient than using a pointer (`ast.Node`).
- **Simplicity**: The position is easily retrieved from any AST node via the `node.Pos()` method.

#### Implementation Plan for `go-scan`

1.  **Define `IdentityMap`**: A new file, `scanner/identity_map.go`, defines the `IdentityMap` struct, which holds maps for `*FunctionInfo` and `*TypeInfo`, keyed by `token.Pos`.
2.  **Integrate `IdentityMap`**: The `goscan.Scanner` owns the `IdentityMap` and passes a reference to the internal `scanner.Scanner` upon creation.
3.  **Modify Object Creation Logic**: The `scanner.Scanner`'s object creation methods (`scanGoFiles` for types, `parseFuncDecl` for functions) are modified to use a "check-and-get or create-and-set" pattern with the `IdentityMap`.

This change guarantees that for any given `ast.Decl` node, only one corresponding `scanner` semantic object is ever created during the lifetime of a `goscan.Scanner` session.

## Part 2: Extending Identity to `symgo`

With object identity guaranteed for `scanner.FunctionInfo`, we can now extend this principle to `symgo`'s `*object.Function`.

### 2.1. The Problem in `symgo`

The `symgo.Evaluator` has its own cache, `funcCache`, to store `*object.Function` instances that it resolves. This cache originally used a string-based key, which did not guarantee identity.

### 2.2. Solution for `symgo.funcCache`

By leveraging the fact that every `scanner.FunctionInfo` is now a unique instance, we changed the `funcCache` key to `token.Pos` (derived from `funcInfo.AstDecl.Pos()`). This ensures that `symgo` will only ever create **one** `*object.Function` for each `scanner.FunctionInfo`.

## Part 3: Simplification of Memoization Cache

The `symgo.Evaluator` also contains a `memoizationCache` to store the results of function evaluations, preventing re-evaluation of the same function.

### 3.1. Current State

The `memoizationCache` currently uses `token.Pos` as its key:
```go
// In symgo/evaluator/evaluator.go (current)
type Evaluator struct {
    // ...
    memoizationCache map[token.Pos]object.Object
}

// In applyFunction()
if f, ok := fn.(*object.Function); ok {
    if e.memoize && f.Decl != nil {
        if cachedResult, found := e.memoizationCache[f.Decl.Pos()]; found {
            return cachedResult
        }
        // ... after evaluation ...
        e.memoizationCache[f.Decl.Pos()] = result
    }
}
```
This works correctly but relies on accessing the `*ast.FuncDecl` through the `object.Function` to get its position.

### 3.2. Proposed Simplification

Now that we have a strong identity guarantee on `*object.Function` itself (thanks to the `funcCache` modifications in Part 2), we no longer need to rely on `token.Pos` as an indirect key. We can use the `*object.Function` pointer directly as the key for the memoization cache.

This is a cleaner, more object-oriented approach.

### Implementation Plan for Memoization

1.  **Update `memoizationCache` Type**:
    The `memoizationCache` field in the `symgo.Evaluator` struct will be changed from `map[token.Pos]object.Object` to `map[*object.Function]object.Object`.

    ```go
    // In symgo/evaluator/evaluator.go
    type Evaluator struct {
        // ...
        memoizationCache map[*object.Function]object.Object
    }

    // In WithMemoization() option
    e.memoizationCache = make(map[*object.Function]object.Object)
    ```

2.  **Modify `applyFunction` Logic**:
    The logic will be simplified to use the function object pointer as the key.

    ```go
    // In symgo.Evaluator.applyFunction
    if f, ok := fn.(*object.Function); ok {
        if e.memoize { // The f.Decl != nil check is no longer needed
            if cachedResult, found := e.memoizationCache[f]; found {
                return cachedResult
            }
            // ... after evaluation ...
            if !isError(result) {
                e.memoizationCache[f] = result
            }
        }
    }
    ```

### Benefits of this Final Change

- **Simplicity**: The code becomes more direct and easier to understand by using the object itself as the key.
- **Robustness**: It removes the dependency on the `*ast.FuncDecl` node within the memoization logic. This makes the system more robust and could potentially allow for memoizing functions that do not have a direct AST declaration (e.g., dynamically generated or function literals), although that is not an immediate goal.
- **Consistency**: It aligns the caching strategy with the newly established object identity principle.