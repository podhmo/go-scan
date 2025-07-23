> [!NOTE]
> This feature has been implemented.

# Proposal: Overlay Feature for go-scan

This document proposes an "overlay" feature for `go-scan`, similar to the `Overlay` option in `go/packages`. This feature will allow users to provide in-memory content for files, which will be used instead of the files on disk. This is particularly useful for tools that generate or modify Go source code before scanning it, without wanting to write the changes to the filesystem.

## 1. High-Level Design

The overlay feature will be implemented by introducing an `Overlay` map to the core `go-scan` components. To ensure portability across different environments (like CI/CD), the keys of this map will not be absolute paths. Instead, they will be one of the following:

1.  **Project-Relative Path**: A path relative to the module root (the directory containing the `go.mod` file). For example, `pkg/user/models.go`.
2.  **Package Path + File Name**: A combination of the Go package's import path and the file name. For example, `example.com/mymodule/pkg/user/models.go`.

The `locator` will be responsible for finding the module root. The `scanner` will then use this information to resolve the overlay keys into the paths it expects.

-   **`scanner.Overlay` type**: A new type `type Overlay map[string][]byte` will be defined in `scanner/models.go`.
-   **`locator.Locator`**: The `Locator` will be updated to accept an `Overlay`. It will use the overlay to read the `go.mod` file. The key for `go.mod` will simply be `go.mod`.
-   **`scanner.Scanner`**: The `Scanner` will be updated to accept an `Overlay`. It will contain the logic to resolve overlay keys against the files it's processing.

## 2. Proposed API Changes & Key Resolution Logic

### `scanner.models.go`

A new type `Overlay` will be added:

```go
// Overlay provides a way to replace the contents of a file with alternative content.
// The key is either a project-relative path (from the module root) or a
// Go package path concatenated with a file name.
type Overlay map[string][]byte
```

### `locator/locator.go`

The `Locator` will need to find the `go.mod` file from the overlay.

```go
// In locator.go

type Locator struct {
    // ... existing fields
    overlay Overlay
    rootDir string // The located module root directory
}

func New(startPath string, overlay Overlay) (*Locator, error) {
    // ... find rootDir ...

    // The key for go.mod is consistently "go.mod"
    goModFilePath := filepath.Join(rootDir, "go.mod")
    var goModBytes []byte
    var err error
    if content, ok := overlay["go.mod"]; ok {
        goModBytes = content
    } else {
        goModBytes, err = os.ReadFile(goModFilePath)
        // ...
    }
    // ...
}
```

### `scanner/scanner.go`

The `Scanner` will perform the key resolution.

```go
// In scanner.go

type Scanner struct {
    // ... existing fields
    Overlay      Overlay
    modulePath   string // From locator
    moduleRootDir string // From locator
}

func New(fset *token.FileSet, ..., overlay Overlay, modulePath, moduleRootDir string) (*Scanner, error) {
    // ... store overlay, modulePath, moduleRootDir
}

func (s *Scanner) ScanFiles(ctx context.Context, filePaths []string, pkgDirPath string, resolver PackageResolver) (*PackageInfo, error) {
    // ...
    for _, absFilePath := range filePaths {
        // Resolve the absolute file path to a key that might be in the overlay
        overlayKey, err := s.resolvePathToOverlayKey(absFilePath)
        if err != nil {
            // Handle error or log a warning
            continue
        }

        var content any
        if c, ok := s.Overlay[overlayKey]; ok {
            content = c
        }

        // The key could also be the package path + filename, so we need to check that too.
        // This requires getting the package import path for the given absFilePath.
        // This logic can get complex and might need a helper.
        // For a first pass, we can assume project-relative paths.

        fileAst, err := parser.ParseFile(s.fset, absFilePath, content, parser.ParseComments)
        // ...
    }
    // ...
}

// resolvePathToOverlayKey converts an absolute file path to a project-relative path.
func (s *Scanner) resolvePathToOverlayKey(absFilePath string) (string, error) {
    if !strings.HasPrefix(absFilePath, s.moduleRootDir) {
        return "", fmt.Errorf("file %s is outside the module root %s", absFilePath, s.moduleRootDir)
    }
    return filepath.Rel(s.moduleRootDir, absFilePath)
}

```

## 3. How `locator` and `scanner` will cooperate

1.  The user provides an `Overlay` map with project-relative paths or package paths as keys.
2.  `locator.New` is called with the `Overlay`. It finds the module root directory. It checks for a `go.mod` key in the overlay to read the module definition.
3.  The `modulePath` and `moduleRootDir` from the `locator` instance are passed to `scanner.New`, along with the `Overlay`. This avoids a direct dependency from `scanner` to `locator`, preventing a circular import.
4.  When the scanner processes files, it receives a list of absolute paths (`filePaths` in `ScanFiles`).
5.  For each absolute path, the scanner will attempt to convert it into a potential overlay key.
    *   **For project-relative keys**: It will calculate the path relative to the module root (e.g., `pkg/user/models.go`).
    *   **For package path keys**: It will need to determine the import path of the package containing the file, and append the filename. This is more complex and may require reversing the logic in `locator.FindPackageDir`.
6.  It then looks up these potential keys in the `Overlay` map. If a match is found, it uses the overlay content for parsing.

This revised design avoids absolute paths, making it robust for CI/CD environments, while still providing the flexibility of overlays. The primary keying strategy should be project-relative paths for simplicity.

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
    // Using project-relative paths as keys
    overlay := scanner.Overlay{
        "go.mod": []byte("module example.com/mymodule\n\ngo 1.21\n"),
        "main.go": []byte(`
package main

import "fmt"

func main() {
    fmt.Println("Hello from overlay!")
}
`),
    }

    fset := token.NewFileSet()
    // The API would need to be adjusted to pass the overlay.
    // The top-level goscan.New will handle creating the locator and passing the required values to the scanner.
    s, err := goscan.New(goscan.WithWorkDir("./my-project"), goscan.WithOverlay(overlay))
    if err != nil {
        log.Fatal(err)
    }

    // ScanPackage would start from "./my-project"
    pkg, err := s.ScanPackage("./my-project")
    if err != nil {
        log.Fatal(err)
    }

    log.Printf("Scanned package: %s\n", pkg.Name)
}
```
