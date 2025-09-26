# Analysis of Object Identity in go-scan

This document analyzes the object identity model within the `go-scan` ecosystem, particularly concerning objects derived from AST declarations (`ast.Decl`). It identifies a key issue where the current implementation does not guarantee that two objects representing the same source code declaration are the same instance in memory. This document proposes a solution to enforce this identity.

The primary goal is to establish a premise where consumers of `go-scan`, such as `symgo`, can safely assume that if two objects (e.g., `*scanner.FunctionInfo`) are derived from the same underlying `ast.Decl`, they are the same object instance (i.e., their pointers are equal).

## 1. Investigation Summary

The investigation covered three main components: `symgo`, `locator`, and `go-scan` (specifically the `scanner` and top-level `goscan` packages).

### `symgo` (`docs/analysis-symgo-implementation.md`)

-   `symgo` is a consumer of `go-scan`. It uses the `scanner.PackageInfo`, `scanner.FunctionInfo`, and other objects provided by `go-scan` to perform symbolic execution.
-   Its effectiveness and simplicity would be greatly improved if it could rely on object identity. For example, tracking visited functions or associating metadata with a function object would be trivial if `func DoSomething()` always resolved to the same `*FunctionInfo` instance.

### `locator` (`locator/locator.go`)

-   The `locator` package is responsible for resolving Go import paths to filesystem directory paths. It reads `go.mod` files, including `replace` directives, to find where a package's source code is located.
-   It does not participate in the creation of ASTs or the semantic objects (`FunctionInfo`, etc.). It is a utility that provides paths to the scanner.
-   **Conclusion**: The `locator` is not the source of the object identity issue.

### `scanner` (`scanner/scanner.go`, `scanner/models.go`)

-   This is the core parsing engine. The `scanner.Scanner` type takes file paths, parses them using `go/parser`, and traverses the resulting AST.
-   During traversal, it creates semantic objects that represent the code's structure:
    -   `parseFuncDecl` creates a `*scanner.FunctionInfo` for each `*ast.FuncDecl`.
    -   `parseTypeSpec` creates a `*scanner.TypeInfo` for each `*ast.TypeSpec`.
-   These objects are created "from scratch" on every scan. The `scanner.Scanner` is essentially stateless regarding the identity of the objects it produces. It holds no long-term cache that maps an `ast.Node` to a `scanner` object.

### `goscan` (`goscan.go`)

-   The top-level `goscan.Scanner` acts as a session or workspace manager. It holds a `packageCache` (a `map[string]*Package`) which caches the `*scanner.PackageInfo` result of a scan to avoid re-parsing files for the same *import path*.
-   However, this cache is at the package level. It does not address the identity of the individual objects *within* the package. If a tool were to clear the cache or use a different `goscan.Scanner` instance with a shared `token.FileSet`, it would receive new, distinct `*FunctionInfo` and `*TypeInfo` objects for the same declarations.
-   The fundamental problem is that the responsibility for creating objects lies in the stateless `scanner.Scanner`, and the caching in `goscan.Scanner` is not granular enough to solve the identity problem.

## 2. The Core Problem: Lack of Object Interning

The root of the issue is the absence of an "interning" mechanism for AST-derived objects.

Consider this workflow:

1.  A tool calls `goscan.Scanner.ScanPackageFromImportPath(ctx, "example.com/foo")`.
2.  `goscan` finds the package, and its internal `scanner.Scanner` parses the files.
3.  For a function `func Bar()`, the `scanner` creates a `*FunctionInfo` instance, let's call it `F1`. `F1.AstDecl` points to the `*ast.FuncDecl` node from the AST.
4.  The resulting `*PackageInfo` (containing `F1`) is returned and cached in `goscan.Scanner.packageCache`.
5.  Later, for any number of reasons (e.g., a different import path resolves to the same directory, or a different analysis path triggers a re-scan), the same `*ast.FuncDecl` is processed again.
6.  The `scanner.Scanner` will create a *new* `*FunctionInfo` instance, `F2`.
7.  Although `F1.AstDecl == F2.AstDecl`, the object instances themselves are different: `F1 != F2`.

This forces consumers like `symgo` to use the `ast.Decl` pointer as the key for any metadata they want to associate with the object, rather than using the object pointer itself, which is less convenient and more error-prone.

## 3. Proposed Solution: An Identity Cache

To solve this, we must introduce a caching layer that ensures a one-to-one mapping between an `ast.Decl` and its corresponding `scanner` object. This cache should be managed at the session level, which corresponds to the top-level `goscan.Scanner`.

The plan is as follows:

1.  **Define `IdentityCache`**:
    A new struct, `IdentityCache`, will be created. It will contain maps to store the canonical instances of our key objects. The `ast.Node` pointer is a suitable key because it is unique for each declaration within a given `token.FileSet`.

    ```go
    // (Conceptual)
    type IdentityCache struct {
        mu         sync.RWMutex
        Functions  map[ast.Node]*scanner.FunctionInfo
        Types      map[ast.Node]*scanner.TypeInfo
        Constants  map[ast.Node]*scanner.ConstantInfo
    }
    ```

2.  **Integrate `IdentityCache` into `goscan.Scanner`**:
    The `goscan.Scanner` in `goscan.go` will own the `IdentityCache`. It will be initialized in `goscan.New` and will have the same lifetime as the scanner session.

3.  **Pass the Cache to `scanner.Scanner`**:
    The internal `scanner.Scanner` in `scanner/scanner.go` will be given a reference to this `IdentityCache`.

4.  **Modify Object Creation in `scanner.Scanner`**:
    The core logic change happens in the `scanner` package. Methods like `parseFuncDecl` and `parseTypeSpec` will be updated:
    -   **Before creating an object**: Check if the `ast.Decl` node already exists as a key in the `IdentityCache`.
    -   **If it exists**: Return the cached `*FunctionInfo` or `*TypeInfo` instance immediately.
    -   **If it does not exist**: Create the new object as usual, add it to the `IdentityCache` with the `ast.Decl` as the key, and then return the new object.

This change centralizes object creation and guarantees that for any given `ast.Decl` node, only one corresponding semantic object is ever created during the lifetime of a `goscan.Scanner`.

### Benefits of this Approach

-   **Guaranteed Identity**: Consumers can safely use object pointers as unique identifiers.
-   **Centralized Logic**: The change is localized to the `scanner` and `goscan` packages, without affecting consumers.
-   **Performance**: Reduces object allocations and garbage collection overhead in scenarios involving re-scans.
-   **Simpler Consumers**: Simplifies the logic in tools like `symgo` that need to associate state with scanned objects.