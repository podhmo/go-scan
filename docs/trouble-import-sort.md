# Diagnosing and Fixing Transitive Dependency Failures

## The Problem

When attempting to interpret standard library functions that have their own dependencies, the `minigo` interpreter fails. The primary test case for this was `sort.Ints()`.

The script:
```go
package main
import "sort"
func main() {
    s := []int{3, 1, 2}
    sort.Ints(s)
    println(s)
}
```

Initially, this script failed with the error:
`runtime error: identifier not found: slices`

This error occurs because `sort.Ints` internally calls `slices.Sort`, but the `minigo` interpreter could not resolve the `slices` package identifier from within the context of the `sort` package.

## Root Cause Analysis

The investigation revealed a two-part problem, with the most fundamental issue residing in the `go-scan` library, which then caused the logic in the `minigo` interpreter to fail.

### Part 1: Flawed Merging Logic in `go-scan`

The primary issue was traced to `goscan.go` in the `FindSymbolInPackage` function. This function is designed to find a symbol by scanning the files of a package one by one for efficiency.

1.  **File-by-File Scanning**: The function gets a list of unscanned `.go` files for a package (e.g., for `sort`, it might get `sort.go`, `gen_sort_variants.go`, etc.).
2.  **Iterative Merging**: It scans these files one at a time. After each file is scanned, its `PackageInfo` is merged into a `cumulativePkgInfo` object.
3.  **Early Exit**: As soon as the requested symbol (e.g., `Ints`) is found in the `PackageInfo` of the most recently scanned file, the function returns the `cumulativePkgInfo` as it exists at that moment.
4.  **The Bug**: The merge logic was incomplete. While it correctly appended lists of `Types`, `Functions`, and `Constants`, it **failed to merge the `AstFiles` map**.

The consequence of this bug is that `cumulativePkgInfo` would only contain the `AstFiles` map from the *very first file* it scanned in the loop. In the case of the `sort` package, if it scanned `gen_sort_variants.go` first, the `AstFiles` map in `cumulativePkgInfo` would only contain the AST for that file. When it later scanned `sort.go` and found the `Ints` symbol, it would add the `FunctionInfo` for `Ints` to the `cumulativePkgInfo` but would *not* add the AST for `sort.go`.

The result was a `PackageInfo` object being returned to the `minigo` interpreter that was inconsistent: it contained the `FunctionInfo` for `Ints` (correctly pointing to `sort.go` as its `FilePath`), but the `AstFiles` map within that same `PackageInfo` did not have an entry for `sort.go`.

### Part 2: Consequent Failure in `minigo`

The `minigo` interpreter relied on the `PackageInfo` from `go-scan` to build a `FileScope` for newly loaded packages. The logic intended to:
1.  Receive the `PackageInfo` for a dependency (e.g., `sort`).
2.  Find the correct `*ast.File` for the symbol in question.
3.  Iterate over that file's `Imports` (e.g., `import "slices"`) to build a `FileScope`.
4.  Attach this `FileScope` to the `object.Function` for `sort.Ints`.
5.  Use this `FileScope` when evaluating the body of `sort.Ints`.

This logic failed because the `*ast.File` for `sort.go` could not be found in the `AstFiles` map due to the `go-scan` bug described above. As a result, no `FileScope` was created for the `sort` package, and the interpreter fell back to using the calling script's scope, which does not have an import for `slices`.

## The Solution

The solution requires fixing the bug in `go-scan` and then implementing the correct corresponding logic in `minigo`.

### Step 1: Fix `go-scan` Merge Logic

The `FindSymbolInPackage` function in `goscan.go` must be modified. The `else` block for the `cumulativePkgInfo` merge must also merge the `AstFiles` map.

**File**: `goscan.go`
**Function**: `FindSymbolInPackage`

```go
// ... inside the for loop ...
// Merge the just-scanned info into a cumulative PackageInfo for this package.
if cumulativePkgInfo == nil {
    cumulativePkgInfo = pkgInfo
} else {
    // This is a simplified merge. A more robust implementation would handle conflicts.
    cumulativePkgInfo.Types = append(cumulativePkgInfo.Types, pkgInfo.Types...)
    cumulativePkgInfo.Functions = append(cumulativePkgInfo.Functions, pkgInfo.Functions...)
    cumulativePkgInfo.Constants = append(cumulativePkgInfo.Constants, pkgInfo.Constants...)

    // <<< FIX START >>>
    // Merge AstFiles and Files lists
    if cumulativePkgInfo.AstFiles == nil {
        cumulativePkgInfo.AstFiles = make(map[string]*ast.File)
    }
    for path, ast := range pkgInfo.AstFiles {
        if _, exists := cumulativePkgInfo.AstFiles[path]; !exists {
            cumulativePkgInfo.AstFiles[path] = ast
        }
    }
    // <<< FIX END >>>

    // Avoid duplicating file paths
    existingFiles := make(map[string]struct{}, len(cumulativePkgInfo.Files))
    for _, f := range cumulativePkgInfo.Files {
        existingFiles[f] = struct{}{}
    }
    for _, f := range pkgInfo.Files {
        if _, exists := existingFiles[f]; !exists {
            cumulativePkgInfo.Files = append(cumulativePkgInfo.Files, f)
            existingFiles[f] = struct{}{}
        }
    }
}
// ...
```

This ensures that the `PackageInfo` returned is always consistent.

### Step 2: Implement Transitive Dependency Logic in `minigo`

With a correct `PackageInfo` from `go-scan`, the `minigo` interpreter can be enhanced to handle the transitive dependencies.

1.  **Extend Data Models**: Add a `FScope *FileScope` field to both `minigo/object/Function` and `minigo/object/Package`. This allows a function or package to carry its own import scope.

2.  **Create `FileScope` on Load**: In `minigo/evaluator/evaluator.go`, the `findSymbolInPackage` function should be updated. After receiving a `PackageInfo` from the scanner, it should iterate over *all* `AstFiles` in that `PackageInfo`, collect all `import` statements, and build a single, complete `FileScope` for the entire package. This `FScope` should be stored on the `object.Package`.

3.  **Attach `FScope` to Functions**: The `findSymbolInPackageInfo` function should be modified to accept this `FScope` and attach it to any `object.Function` it creates.

4.  **Use `FScope` on Execution**: The `applyFunction` method in the evaluator must be modified. When it's about to evaluate a function's body, it should check if `function.FScope` is non-nil. If it is, it must use that scope for the evaluation, overriding the scope of the caller. This ensures the function's body is evaluated in the context of its own file's imports.

## Secondary Issue: Sequential Declaration

After applying the fix for transitive dependencies, the `sort.Ints` test will still fail, but with a new error:
`runtime error: identifier not found: pdqsortOrdered`

This is a separate, known limitation of the `minigo` interpreter. The `slices` package defines the `Sort` function *before* its unexported helper `pdqsortOrdered`. The interpreter currently processes declarations sequentially and cannot find symbols that are defined later in the same package.

The correct fix for this is to implement a two-pass evaluation strategy, as noted in `TODO.md`. This is a separate task from fixing the transitive dependency loading.
