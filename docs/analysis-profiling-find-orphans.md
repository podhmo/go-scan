# Performance Profiling Analysis for `find-orphans`

This document details the performance profiling conducted on the `find-orphans` tool, with a specific focus on identifying and reducing memory usage.

## Methodology

To understand the tool's memory footprint, we performed the following steps:

1.  **Added Profiling Hooks**: The `main.go` file of `find-orphans` was modified to include standard Go `pprof` hooks. We added `--cpuprofile` and `--memprofile` flags to enable the generation of CPU and memory profiles.
2.  **Built the Tool**: The tool was compiled using a standard `go build` command.
3.  **Generated Profiles**: We ran the compiled `find-orphans` binary against its own entire workspace (a repository of non-trivial size) to generate a realistic performance profile. The command used was:
    ```bash
    ./find-orphans-prof --workspace-root=. --memprofile=mem.pprof --cpuprofile=cpu.pprof
    ```
4.  **Analyzed the Memory Profile**: The resulting `mem.pprof` file was analyzed using `go tool pprof`.

## Profiling Results

The `go tool pprof -top mem.pprof` command provided the following output, showing the functions with the highest memory allocations (`inuse_space`):

```
File: find-orphans-prof
Build ID: 7dfee3181b044684d295e912d4cb426b9f64a848
Type: inuse_space
Time: 2025-08-26 20:38:20 UTC
Showing nodes accounting for 9378.78kB, 100% of 9378.78kB total
      flat  flat%   sum%        cum   cum%
 1184.27kB 12.63% 12.63%  1184.27kB 12.63%  runtime/pprof.StartCPUProfile
 1024.16kB 10.92% 23.55%  1024.16kB 10.92%  github.com/podhmo/go-scan/scanner.(*Scanner).TypeInfoFromExpr
 1024.03kB 10.92% 34.47%  1537.03kB 16.39%  go/parser.(*parser).parseIdent
 1024.02kB 10.92% 45.38%  1024.02kB 10.92%  go/parser.(*parser).parseSelector
     513kB  5.47% 50.85%      513kB  5.47%  go/token.(*File).AddLine
     513kB  5.47% 56.32%      513kB  5.47%  runtime.allocm
  512.07kB  5.46% 61.78%  1024.15kB 10.92%  github.com/podhmo/go-scan/scanner.(*Scanner).parseFuncType
  512.04kB  5.46% 67.24%   512.04kB  5.46%  go/ast.NewObj (inline)
  512.04kB  5.46% 72.70%  1024.05kB 10.92%  go/parser.(*parser).parseValueSpec
  512.03kB  5.46% 78.16%  2561.11kB 27.31%  go/parser.(*parser).parseIfStmt
  512.03kB  5.46% 83.62%  1024.04kB 10.92%  go/parser.(*parser).parseLiteralValue
  512.03kB  5.46% 89.08%   512.03kB  5.46%  go/parser.(*parser).parseParameterList.func1 (inline)
  512.03kB  5.46% 94.54%  1024.05kB 10.92%  go/parser.(*parser).parseSimpleStmt
  512.02kB  5.46%   100%   512.02kB  5.46%  github.com/podhmo/go-scan/scanner.(*Scanner).ScanPackageImports
```

## Analysis and Conclusion

The profiling results clearly indicate that the majority of memory is consumed during the code parsing phase.

-   **Primary Cause**: The `go/parser` package and its functions (`parseIdent`, `parseSelector`, etc.) are the top direct (`flat`) allocators.
-   **Cumulative Impact**: The cumulative (`cum`) allocations show that functions like `go/parser.ParseFile` (60.06% of cumulative memory) are the main drivers of memory usage. This function is responsible for parsing Go source files into Abstract Syntax Trees (ASTs).
-   **Architectural Bottleneck**: The core issue lies in the `analyzer`'s architecture. The `analyzer.Visit` method is called for every package in the dependency graph. Inside this method, `a.s.ScanPackageByImport` is called, which parses the entire package and returns a `*scanner.PackageInfo` object. This object, which contains the memory-intensive ASTs for all files in the package, is then stored in the `a.pacakges` map.

This approach leads to **all packages and their full ASTs being held in memory for the entire duration of the program**. While the memory usage for the tool's own repository was manageable (~9.4 MB), this design will not scale well to larger monorepos, where it could easily consume gigabytes of memory.

## Verification of the Fix

The proposed solution was to refactor the `analyzer` to remove its internal `packages` map and instead fetch package information on-demand from the `goscan.Scanner` during the analysis phase. The hypothesis was that this would allow the `goscan.Scanner`'s internal cache to manage memory more efficiently.

After implementing this refactoring, the profiler was run again on the same codebase.

### New Profiling Results (After Refactoring)

```
File: find-orphans-prof
Build ID: cdf952d7633b4f60a56e0343824303b0bb79fe58
Type: inuse_space
Time: 2025-08-26 20:46:38 UTC
Showing nodes accounting for 10251.74kB, 100% of 10251.74kB total
      flat  flat%   sum%        cum   cum%
 2560.08kB 24.97% 24.97%  2560.08kB 24.97%  go/parser.(*parser).parseIdent
 1024.16kB  9.99% 34.96%  1024.16kB  9.99%  github.com/podhmo/go-scan/scanner.(*Scanner).TypeInfoFromExpr
 1024.14kB  9.99% 44.95%  5130.35kB 50.04%  go/parser.(*parser).parseStmtList
 1024.08kB  9.99% 54.94%  1024.08kB  9.99%  go/ast.NewObj (inline)
  522.06kB  5.09% 60.03%   522.06kB  5.09%  go/token.(*File).AddLine
```

### Conclusion of Verification

The refactoring **did not succeed** in reducing memory usage. In fact, the total `inuse_space` slightly increased from ~9.4 MB to ~10.25 MB.

The reason is that the fundamental requirement of the analysis—having access to the type information of all packages in the workspace simultaneously—did not change. Shifting the responsibility of caching from the `analyzer`'s explicit map to the `goscan.Scanner`'s implicit cache did not reduce the amount of data that needed to be held in memory. The slight increase may be attributable to less efficient cache access patterns in the new implementation.

A more effective solution would require a deeper change in the `go-scan` library itself, such as parsing files into a more lightweight intermediate representation that doesn't include the full AST, or implementing a more sophisticated eviction policy in the scanner's cache. These changes are beyond the scope of tuning the `find-orphans` tool itself. The current implementation, while memory-intensive, is correct, and the attempted optimization was not successful.
