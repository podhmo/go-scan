# Proposal: Overlay Feature for go-scan

This document proposes an "overlay" feature for `go-scan`, similar to the `Overlay` option in `go/packages`. This feature will allow users to provide in-memory content for files, which will be used instead of the files on disk. This is particularly useful for tools that generate or modify Go source code before scanning it, without wanting to write the changes to the filesystem.

## 1. High-Level Design

The overlay feature will be implemented by introducing an `Overlay` map to the core `go-scan` components. This map will have file paths as keys and their byte-slice content as values.

-   **`scanner.Overlay` type**: A new type `type Overlay map[string][]byte` will be defined in `scanner/models.go`.
-   **`locator.Locator`**: The `Locator` will be updated to accept an `Overlay`. It will use the overlay to read the `go.mod` file if it is present in the map.
-   **`scanner.Scanner`**: The `Scanner` will be updated to accept an `Overlay`. When parsing Go files, it will first check if a file exists in the overlay. If so, it will use the content from the overlay; otherwise, it will read the file from the disk.
-   **Entry Point**: The main entry point for `go-scan` will be updated to accept the `Overlay` map, allowing users to provide overlay data.

## 2. Proposed API Changes

### `scanner.models.go`

A new type `Overlay` will be added:

```go
// Overlay provides a way to replace the contents of a file with alternative content.
// The key is the absolute file path, and the value is the content to use instead.
type Overlay map[string][]byte
```

### `locator/locator.go`

The `Locator` struct will be updated to include the `Overlay` map. The `New` function will be modified to accept it.

```go
// In locator.go

type Locator struct {
    // ... existing fields
    overlay Overlay
}

func New(startPath string, overlay Overlay) (*Locator, error) {
    // ...
    goModFilePath := filepath.Join(rootDir, "go.mod")
    var goModBytes []byte
    var err error
    if content, ok := overlay[goModFilePath]; ok {
        goModBytes = content
    } else {
        goModBytes, err = os.ReadFile(goModFilePath)
        if err != nil {
            return nil, err
        }
    }

    modPath, err := getModulePath(goModFilePath, goModBytes)
    // ...
    replaces, err := getReplaceDirectives(goModFilePath, goModBytes)
    // ...
}

func getModulePath(goModPath string, content []byte) (string, error) {
    // ... implementation using content
}

func getReplaceDirectives(goModPath string, content []byte) ([]ReplaceDirective, error) {
    // ... implementation using content
}
```

### `scanner/scanner.go`

The `Scanner` struct and its `New` function will be updated to handle the `Overlay`. The `ScanFiles` method will be modified to use the overlay content.

```go
// In scanner.go

type Scanner struct {
    // ... existing fields
    Overlay Overlay
}

func New(fset *token.FileSet, overrides ExternalTypeOverride, overlay Overlay) (*Scanner, error) {
    // ...
}

func (s *Scanner) ScanFiles(ctx context.Context, filePaths []string, pkgDirPath string, resolver PackageResolver) (*PackageInfo, error) {
    // ...
    for _, filePath := range filePaths {
        var content any
        if c, ok := s.Overlay[filePath]; ok {
            content = c
        }
        fileAst, err := parser.ParseFile(s.fset, filePath, content, parser.ParseComments)
        // ...
    }
    // ...
}
```

## 3. How `locator` and `scanner` will cooperate

1.  The user of the `go-scan` library will create an `Overlay` map containing the virtual file contents.
2.  This `Overlay` map will be passed to `locator.New` and `scanner.New`.
3.  When `locator.New` is called, it will look for the `go.mod` file path in the `Overlay` map. If found, it will use the provided content to parse the module path and replace directives. Otherwise, it will read from the filesystem.
4.  When `scanner.Scanner`'s methods (like `ScanPackage` or `ScanFiles`) are called, the scanner will iterate through the list of Go files to be parsed.
5.  For each file, the scanner will check if its path exists in the `Overlay` map.
6.  If the file path is in the `Overlay`, `parser.ParseFile` will be called with the content from the map.
7.  If the file path is not in the `Overlay`, `parser.ParseFile` will be called with `nil` for the content, which instructs it to read the file from the filesystem.

This design ensures that both the module resolution and the source code parsing respect the overlay, providing a consistent virtual view of the project.

## 4. Example Usage

```go
package main

import (
    "go/token"
    "log"

    "github.com/your-repo/go-scan/goscan"
    "github.com/your-repo/go-scan/scanner"
)

func main() {
    overlay := scanner.Overlay{
        "/path/to/project/main.go": []byte(`
package main

import "fmt"

func main() {
    fmt.Println("Hello from overlay!")
}
`),
    }

    fset := token.NewFileSet()
    // The main entry point of go-scan needs to be updated to accept the overlay.
    // Assuming a function like `goscan.NewScanner` exists and is updated.
    s, err := goscan.NewScanner(fset, nil, overlay)
    if err != nil {
        log.Fatal(err)
    }

    pkg, err := s.ScanPackage("/path/to/project")
    if err != nil {
        log.Fatal(err)
    }

    // Now, `pkg` contains information parsed from the overlay content for main.go
    // and from the filesystem for other files in the package.
    log.Printf("Scanned package: %s\n", pkg.Name)
}
```
