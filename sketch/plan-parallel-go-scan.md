> [!NOTE]
> This feature has been implemented.

# Detailed Implementation Plan for Parallelizing `go-scan`

This document provides a concrete, step-by-step plan for refactoring the `go-scan` library to perform scanning operations concurrently, aiming to improve performance while ensuring thread safety.

## 1. Goal

The primary goal is to significantly improve the performance of `go-scan`, especially for large packages or projects with many files. This will be achieved by parallelizing the most CPU-intensive part of the process: parsing individual Go source files.

The secondary goal is to make the top-level `goscan.Scanner` object safe for concurrent use, allowing multiple high-level API calls (e.g., `ScanPackageFromImportPath`) to run in parallel without interfering with each other.

## 2. Proposed Architecture: Parallel Parse, Sequential Process

The core of the refactoring will happen in the `scanner.scanGoFiles` method. The current sequential process will be changed to a two-stage process:

1.  **Parallel Parsing Stage:** Given a list of file paths, the method will launch a dedicated Go goroutine for each file. Each goroutine will be responsible for parsing a single file using `parser.ParseFile`. This distributes the heavy lifting of parsing across all available CPU cores.

2.  **Sequential Processing Stage:** The main goroutine will collect the results from all parsing goroutines (the `*ast.File` objects). Once all files have been parsed, the main goroutine will process the list of ASTs sequentially, as is done now. This approach avoids the complexity and potential race conditions of trying to build the final `PackageInfo` struct from multiple threads concurrently. It keeps the logic for handling package name mismatches and appending to result slices simple and free of locks.

This architecture provides the maximum performance benefit (from parallel parsing) with the minimum implementation risk (by keeping the stateful processing sequential).

### 2.1 Handling Concurrent Lazy Loading and Caching

A key feature of `go-scan` is its ability to "lazy load" package information. When resolving a type from an un-scanned package, `go-scan` triggers a new, on-demand scan for that package. In a concurrent environment, multiple threads could try to resolve types from the same external package simultaneously, leading to a race to scan that package.

The proposed architecture inherently solves this problem through the same mechanisms that ensure general thread safety:

*   **Synchronized Cache Access:** All high-level scan functions (e.g., `ScanPackageFromImportPath`) will begin by checking the `packageCache` for the requested package inside a mutex-protected block.
*   **Preventing Redundant Work:**
    1.  The first goroutine to request a scan for a package (e.g., `pkg-C`) will acquire the lock, see that it's not in the cache, and proceed to scan it.
    2.  While the first goroutine is scanning, any other goroutine that requests `pkg-C` will block waiting for the lock.
    3.  Once the first goroutine finishes, it places the result in the cache and releases the lock.
    4.  The waiting goroutines will then acquire the lock one by one, check the cache, and find the pre-computed result. They will get a cache hit and return immediately without performing a redundant scan.

This "check-lock-check" pattern on the `packageCache` ensures that even with many concurrent lazy-loading requests, any given package is only ever scanned once. This not only prevents race conditions but also ensures optimal performance by avoiding duplicate work. The thread-safety measures in **Task 1** are therefore crucial for both correctness and performance in a lazy-loading context.

## 3. Detailed Task List

This task list is designed to be executed in order, with each step building on the last.

---

### **Task 1: Make `goscan.Scanner` Thread-Safe**

**File:** `goscan.go`

The `goscan.Scanner` struct contains shared state that is accessed by multiple methods. The most critical unprotected state is the `visitedFiles` map. This task is to wrap all accesses to this map with the existing `s.mu` mutex.

-   **Action:** Locate every read and write operation on `s.visitedFiles`.
-   **Reads:** Wrap read operations with `s.mu.RLock()` and `s.mu.RUnlock()`.
    -   Example locations: `ScanPackageFromFilePath`, `ScanFiles`, `UnscannedGoFiles`, `ScanPackageFromImportPath`.
-   **Writes:** Wrap write operations with `s.mu.Lock()` and `s.mu.Unlock()`.
    -   Example locations: `ScanPackageFromFilePath`, `ScanFiles`, `ScanPackageFromImportPath`, `FindSymbolInPackage`.

**Example Change in `ScanFiles`:**
```go
// Before
if _, visited := s.visitedFiles[absFp]; !visited {
    filesToParse = append(filesToParse, absFp)
}
// ...
s.visitedFiles[fp] = struct{}{}

// After
s.mu.RLock()
_, visited := s.visitedFiles[absFp]
s.mu.RUnlock()
if !visited {
    filesToParse = append(filesToParse, absFp)
}
// ...
s.mu.Lock()
s.visitedFiles[fp] = struct{}{}
s.mu.Unlock()
```

---

### **Task 2: Refactor `scanner.scanGoFiles` for Concurrent Parsing**

**File:** `scanner/scanner.go`

This is the core of the implementation. The `scanGoFiles` method will be refactored to implement the "Parallel Parse, Sequential Process" architecture.

**Sub-Task 2.1: Define a Result Struct**

Create a private struct to hold the result of a single file parse. This makes passing data back from goroutines clean and simple.

```go
// Add this struct inside scanner/scanner.go
type fileParseResult struct {
	filePath string
	fileAst  *ast.File
	err      error
}
```

**Sub-Task 2.2: Implement the Parallel Parsing Loop**

Rewrite the beginning of `scanGoFiles` to manage goroutines.

```go
// In scanGoFiles...
import (
	"sync"
	"golang.org/x/sync/errgroup"
)

func (s *Scanner) scanGoFiles(...) (*PackageInfo, error) {
	// ... (setup info struct as before)

	results := make(chan fileParseResult, len(filePaths))
	g, ctx := errgroup.WithContext(ctx)

	for _, filePath := range filePaths {
		fp := filePath // create a new variable for the closure
		g.Go(func() error {
			var content any
			// ... (logic to get overlay content) ...

			fileAst, err := parser.ParseFile(s.fset, fp, content, parser.ParseComments)

			select {
			case results <- fileParseResult{filePath: fp, fileAst: fileAst, err: err}:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}

	if err := g.Wait(); err != nil {
		close(results)
		return nil, err
	}
	close(results)
```

**Sub-Task 2.3: Implement the Result Collection Logic**

After the `g.Wait()` call, collect all the results from the channel.

```go
// In scanGoFiles, after g.Wait()...

	parsedFileResults := make([]fileParseResult, 0, len(filePaths))
	for result := range results {
		if result.err != nil {
			// Even with errgroup, a non-context error might slip through.
			// This makes the error handling robust.
			return nil, fmt.Errorf("failed to parse file %s: %w", result.filePath, result.err)
		}
		parsedFileResults = append(parsedFileResults, result)
	}

	// At this point, all files are parsed successfully.
	// Now we proceed with the sequential processing stage.
```

**Sub-Task 2.4: Adapt the Sequential Processing Logic**

The second half of the original `scanGoFiles` can now be adapted to work with the `parsedFileResults` slice instead of iterating over the file paths again.

```go
// In scanGoFiles, after collecting results...

	var dominantPackageName string
	var parsedFiles []*ast.File // This will hold the ASTs of the dominant package

	// First pass over results to determine dominant package name
	// and filter out files from other packages (e.g., 'main' in tests).
	// This logic is similar to the original sequential version, but adapted
	// for the result struct.
	// ...

	// Second pass: Process declarations from the final list of valid ASTs
	for i, fileAst := range parsedFiles {
		filePath := info.Files[i] // info.Files must be populated alongside parsedFiles
		info.AstFiles[filePath] = fileAst
		importLookup := s.buildImportLookup(fileAst)

		for _, decl := range fileAst.Decls {
			// ... existing s.parseGenDecl / s.parseFuncDecl logic ...
		}
	}

	// ... (rest of the function, e.g., s.resolveEnums)
	return info, nil
}
```

This completes the detailed plan. Executing these tasks in order will result in a thread-safe and significantly faster `go-scan` library.
