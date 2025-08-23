# Analysis and Suggestions for Naming and Refactoring

## Renaming Suggestions

This section proposes changes to existing names of structs, methods, and packages to improve clarity, reduce ambiguity, and align names more closely with their responsibilities.

### 1. `goscan.Scanner` vs. `scanner.Scanner`

**Problem:**
The name `Scanner` is used for two different core structs in the library, leading to potential confusion.
- `goscan.Scanner`: This is a high-level, user-facing struct. It acts as a session or controller, managing configuration, caching (`packageCache`, `symbolCache`), package location (`locator`), and orchestrating the overall scanning process.
- `scanner.Scanner`: This is a lower-level, internal struct. Its primary role is to parse ASTs from a given set of files and build the `PackageInfo` model. It is the core parsing engine.

**Proposal:**
Rename these structs to better reflect their distinct roles.

- **`goscan.Scanner` -> `goscan.Session`**
  - **Reasoning:** `Session` accurately describes its role as a stateful object that a user creates to interact with the library over a series of operations. It manages the context (caches, settings) for a scanning session. Alternatives could be `ScannerSession` or `Context`.

- **`scanner.Scanner` -> `scanner.Parser`**
  - **Reasoning:** `Parser` clearly communicates that this struct's responsibility is to parse source code. It takes Go files as input and produces a structured representation (`PackageInfo`). This avoids the clash with the high-level `Scanner` and is more specific about its function. An alternative could be `Engine`.

### 2. `scanner.FieldType`

**Problem:**
The name `FieldType` is ambiguous because this struct is used in many contexts beyond struct fields. It represents any reference to a type, including:
- Struct fields
- Function parameters
- Function results
- Type alias underlying types
- Generic type constraints

Its core responsibility is to act as a lazy-loading reference to a full `TypeInfo` definition, which can be resolved on-demand.

**Proposal:**
- **`scanner.FieldType` -> `scanner.TypeRef`**
  - **Reasoning:** `TypeRef` (short for Type Reference) is a much more accurate and general name. It clearly conveys that this struct is a *reference* to a type that may or may not be resolved yet. This name fits all its use cases. Alternatives could be `ResolvableType` or `LazyType`.

### 3. `Scan...` Method Suite in `goscan.Scanner`

**Problem:**
The `goscan.Scanner` has several methods for initiating a scan, distinguished by the type of input they accept (`ScanPackage`, `ScanPackageByImport`, `ScanFiles`). While the names are functional, they could be more explicit to prevent misuse.

- `ScanPackage(pkgPath string)`: Takes a filesystem directory path.
- `ScanPackageByImport(importPath string)`: Takes a Go import path.

**Proposal:**
- **`ScanPackage(pkgPath string)` -> `ScanPackageFromDir(pkgDirPath string)`**
  - **Reasoning:** `FromDir` explicitly states that the input is a filesystem directory. This reduces the chance of a user passing an import path to it by mistake. `pkgDirPath` for the parameter name further clarifies this.

- **`ScanPackageByImport(importPath string)` -> (keep as is or `ScanPackage(importPath string)`)**
  - **Reasoning:** `ScanPackageByImport` is already quite clear. If `ScanPackage` is renamed to `ScanPackageFromDir`, then the original `ScanPackage` name could be repurposed for the import path version, as it's the more common use case. However, keeping `ScanPackageByImport` is also a good, explicit option.

### 4. `FindSymbol...` Methods in `goscan.Scanner`

**Problem:**
The methods `FindSymbolDefinitionLocation` and `FindSymbolInPackage` have similar names but very different behavior and return types, which can be confusing.
- `FindSymbolDefinitionLocation(symbolFullName string) (string, error)`: Returns the *file path* (`string`) where a symbol is defined. It's a direct lookup that uses caches and falls back to scanning.
- `FindSymbolInPackage(importPath string, symbolName string) (*scanner.PackageInfo, error)`: Iteratively scans files in a package one-by-one until the symbol is found. It returns the cumulative `PackageInfo` of all files scanned up to that point.

**Proposal:**
Rename them to highlight their different goals and return values.

- **`FindSymbolDefinitionLocation(...)` -> `FindSymbolLocation(...)` or `LocateSymbolFile(...)`**
  - **Reasoning:** `Location` is a bit shorter and still clear. `LocateSymbolFile` is very explicit about returning a file path.

- **`FindSymbolInPackage(...)` -> `ScanPackageUntilSymbol(...)`**
  - **Reasoning:** This name better describes the operational behavior: it's a *scanning* operation that continues *until* a symbol is found. It also hints that the return value is related to the scan (i.e., a `PackageInfo`) rather than just a location string.

## Refactoring Suggestions

This section outlines potential refactoring opportunities to improve the codebase's structure, maintainability, and internal consistency.

### 1. Package and File Structure

**Problem:**
Some packages and files have a large number of responsibilities, which could be broken down for better separation of concerns.

- **`scanner` package**: Currently, `scanner.go` contains all the AST parsing and traversal logic, while `models.go` contains all the core data structures (`PackageInfo`, `TypeInfo`, etc.). This is a reasonable starting point, but the data models are fundamental to the entire library and could be more independent.
- **`goscan.go` file**: This file is very large. It contains the main `Scanner` (proposed: `Session`) struct, all of its methods, option types, and the entire implementation of the persistent `symbolCache`.

**Proposal:**

- **Create a `model` (or `schema`) sub-package**:
  - Move `scanner/models.go` to a new, dedicated package, for example, `github.com/podhmo/go-scan/model`.
  - **Reasoning:** The data structures are the core "language" of the library. Placing them in a stable, independent package with minimal dependencies makes them easier to reuse and reference from other parts of the system (like the `parser` and `session`). The `scanner` package would then be purely responsible for parsing and would depend on the `model` package.

- **Split `goscan.go` into multiple files**:
  - `session.go`: Contains the `Session` struct and its primary methods (`Scan...`, `Find...`).
  - `options.go`: Contains the `ScannerOption` type and all `With...` option functions.
  - `symbol_cache.go`: Extract all the `symbolCache` related logic (the struct, `newSymbolCache`, `load`, `save`, `setSymbol`, etc.) into its own file. This logic is complex and self-contained enough to warrant its own file.
  - **Reasoning:** This follows standard Go practice of organizing code by feature within a package. It makes the `goscan` package easier to navigate and reduces the cognitive load of working with a single, massive file.

### 2. API and Logic Consistency

**Problem:**
There is an inconsistency in how `PackageInfo` objects are handled. A comment in `goscan.go` suggests a "no merge" philosophy, but the `FindSymbolInPackage` method explicitly merges `PackageInfo` objects from multiple file scans.

```go
// in goscan.Scanner.FindSymbolInPackage
if cumulativePkgInfo == nil {
    cumulativePkgInfo = pkgInfo
} else {
    // This is a simplified merge...
    cumulativePkgInfo.Types = append(cumulativePkgInfo.Types, pkgInfo.Types...)
    // ... and so on for Functions, Constants, etc.
}
```

This ad-hoc merge is brittle and contradicts the stated design principle.

**Proposal:**
Commit to a consistent strategy.

- **Option A: Embrace Merging (Recommended)**
  - Create a formal `Merge(other *PackageInfo)` method on the `PackageInfo` struct. This method would handle the logic for combining two `PackageInfo` objects, including deduplicating types, functions, files, etc.
  - Refactor `FindSymbolInPackage` and any other relevant places to use this robust `Merge` method.
  - **Reasoning:** Merging partial package scans is a powerful and necessary feature for iterative scanning. Formalizing it in a dedicated method makes the logic reusable, testable, and reliable.

- **Option B: Strictly Enforce No Merging**
  - Refactor `FindSymbolInPackage` to avoid merging. This would likely require a significant redesign of the function, perhaps by returning only the `TypeInfo` of the found symbol rather than a `PackageInfo`.
  - **Reasoning:** This would enforce the original design principle, but it might reduce the utility of the function.

### 3. Struct Decomposition

**Problem:**
The primary structs (`goscan.Scanner` and `scanner.FieldType`) are quite large, holding both data and configuration. While this is often pragmatic, it's worth noting.

**Proposal:**
This is a minor point, as the current structure is functional. However, for future consideration:

- **`goscan.Session` (proposed name):** The separation of the `symbolCache` into its own file (as suggested above) is the most important first step. The struct itself acts as a dependency injection container for a single session, so its size is somewhat justified.
- **`scanner.TypeRef` (proposed name):** This struct mixes descriptive fields (`IsPointer`, `Elem`) with resolution logic (`Resolver`, `Definition`). One could imagine separating these into a `TypeDescriptor` and a `TypeResolver`, but this would likely add more complexity than it removes. The current design is a reasonable trade-off. No immediate action is recommended here beyond the renaming.
