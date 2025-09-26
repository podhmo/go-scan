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

The `symgo.Evaluator` has its own cache, `funcCache`, to store `*object.Function` instances that it resolves. However, this cache uses a string-based key, generated from the package path and function name.

```go
// In symgo/evaluator/evaluator.go (before change)
key := fmt.Sprintf("%s.(%s).%s", pkg.Path, funcInfo.Receiver.Type.String(), funcInfo.Name)
// ... or ...
key := fmt.Sprintf("%s.%s", pkg.Path, funcInfo.Name)
```

While this reduces work, it does not guarantee identity. If different analysis paths resolve the same function at different times, they might use slightly different (but semantically identical) `scanner.FunctionInfo` objects, potentially leading to different cache keys or cache misses, resulting in duplicate `*object.Function` instances.

### 2.2. Proposed Solution for `symgo`

We can now leverage the fact that every `scanner.FunctionInfo` passed to `symgo` is a unique, canonical instance for that declaration. Therefore, we can use its unique `token.Pos` as the key for `symgo`'s `funcCache`.

This change ensures that `symgo` will only ever create **one** `*object.Function` for each `scanner.FunctionInfo`.

### Implementation Plan for `symgo`

The change is localized to `symgo/evaluator/evaluator.go`:

1.  **Update `funcCache` Type**:
    The `funcCache` field in the `symgo.Evaluator` struct will be changed from `map[string]object.Object` to `map[token.Pos]object.Object`.

    ```go
    // In symgo/evaluator/evaluator.go
    type Evaluator struct {
        // ...
        funcCache map[token.Pos]object.Object
        // ...
    }

    // In New()
    funcCache: make(map[token.Pos]object.Object),
    ```

2.  **Modify `getOrResolveFunction` Logic**:
    This method is the central point for creating `*object.Function` instances. The key generation logic will be updated to use the position of the function's AST declaration.

    ```go
    // In symgo.Evaluator.getOrResolveFunction
    func (e *Evaluator) getOrResolveFunction(ctx context.Context, pkg *object.Package, funcInfo *scan.FunctionInfo) object.Object {
        // The AST declaration is guaranteed to exist for functions from source.
        if funcInfo.AstDecl == nil {
            // Handle functions without an AST node (e.g., synthetic) separately if needed.
            // For now, these won't be cached.
            return e.resolver.ResolveFunction(ctx, pkg, funcInfo)
        }

        key := funcInfo.AstDecl.Pos()

        // Check cache first.
        if fn, ok := e.funcCache[key]; ok {
            return fn
        }

        // Not in cache, resolve it.
        fn := e.resolver.ResolveFunction(ctx, pkg, funcInfo)

        // Store in cache for next time.
        e.funcCache[key] = fn
        return fn
    }
    ```

### Overall Benefits

By implementing these changes in both `go-scan` and `symgo`, we establish a strong guarantee of object identity from the lowest level of scanning up to the highest level of symbolic execution. This simplifies the entire system, eliminates a class of potential bugs related to state management, and makes consumers like `symgo` more robust and easier to reason about.