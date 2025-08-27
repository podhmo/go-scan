# Trouble: `find-orphans` Scans Packages Outside the Workspace

This document details a bug where the `find-orphans` tool would attempt to scan Go packages outside the specified workspace, such as those in the standard library or third-party module cache, leading to unexpected behavior and panics.

## The Problem

The `find-orphans` tool uses the `symgo` symbolic execution engine to trace function calls and determine usage. The process is as follows:

1.  The tool identifies a "workspace" of Go modules to analyze.
2.  It uses `go-scan` to parse all the Go files within this workspace.
3.  It initializes a `symgo.Interpreter` with a `go-scan.Scanner` instance that is configured with `WithGoModuleResolver()`, allowing it to find and parse any Go package, including those in the standard library or module cache.
4.  The `symgo` engine begins evaluating the code, starting from entry points like `main.main`.
5.  During evaluation, `symgo` may encounter a type from a package that was not part of the initial workspace scan (e.g., a struct field of type `*encoding/json.Encoder`).
6.  To understand this type, `symgo`'s evaluator would trigger a type resolution via `scanner.FieldType.Resolve()`.
7.  This `Resolve()` method, using the powerful, unrestricted `go-scan.Scanner` provided to the interpreter, would locate the source code for the external package (e.g., `/usr/local/go/src/encoding/json`) and attempt to scan it.

This behavior was incorrect for `find-orphans`, whose analysis should be strictly confined to the user-defined workspace. It led to a panic that was intentionally placed in the code to highlight this out-of-scope access.

## The Solution

The core issue was that the `symgo` engine's scanner was not aware of the workspace boundaries defined in the `find-orphans` tool. The fix was to introduce a boundary layer within `symgo` that could enforce these limits without making the underlying `go-scan` library less general-purpose.

The solution involved the following changes, primarily within the `symgo` package:

1.  **Bounded Resolver**: A new type, `boundedResolver`, was created. It implements the `scanner.PackageResolver` interface. Its purpose is to intercept `ScanPackageByImport` calls. Before delegating to the real scanner, it uses a `scopeCheck` function to determine if the requested import path is within the allowed workspace. If it's not, it returns a minimal, empty `scanner.PackageInfo` struct, effectively treating the external package as a black box and preventing an out-of-scope file system scan.

2.  **Scannable Scanner Wrapper**: A wrapper type, `ScannableScanner`, was introduced. It embeds a `*goscan.Scanner` but overrides the `TypeInfoFromExpr` method. This overridden method is the key to the solution: when it creates a `scanner.FieldType`, it replaces the type's default `Resolver` with an instance of our new `boundedResolver`. This ensures that any subsequent call to `Resolve()` on that type will go through our boundary check.

3.  **Interface-based Dependency**: The `symgo/evaluator.Evaluator`, which previously depended on a concrete `*goscan.Scanner`, was changed to depend on a new `ScannerInterface`. This allows either a regular `*goscan.Scanner` or our new `ScannableScanner` wrapper to be used, providing the necessary flexibility.

4.  **Configuration via `WithScopeCheck`**: A new option, `symgo.WithScopeCheck(func(string) bool)`, was added to the `symgo.Interpreter`. This allows the calling application (`find-orphans`) to inject its own workspace-aware logic.

5.  **Integration in `find-orphans`**: The `find-orphans` tool was updated to use this new option. It now defines a `scopeCheck` function based on the list of discovered workspace modules and passes it to the interpreter. This function is what tells the `boundedResolver` whether an import path like `"encoding/json"` is "in" or "out" of the workspace.

This approach successfully confines the analysis to the workspace, fixing the bug while maintaining a clean separation of concerns between the general-purpose scanning library (`go-scan`) and the more specialized symbolic execution engine (`symgo`).
