# Plan: Unifying `derivingjson` and `derivingbind` into a Single-Pass Generator

## 1. Goal

The primary objective is to combine the functionality of the `derivingjson` and `derivingbind` tools into a single, efficient command-line tool.

The key requirement is that this new tool must parse the target Go source files **only once** to generate code for both `deriving:unmarshal` and `derivng:binding` annotations. This will improve performance by eliminating redundant file I/O and AST parsing.

## 2. Analysis of Existing Implementations

Both `examples/derivingjson/main.go` and `examples/derivingbind/main.go` follow an identical pattern:

1.  **Initialization**: Parse command-line arguments (file or directory paths) and instantiate a `goscan.Scanner`.
2.  **Scanning**: Call `ScanPackage` or `ScanFiles` on the scanner instance to obtain a `scanner.PackageInfo`. This `PackageInfo` object is a complete representation of the parsed code for the target package.
3.  **Generation**: Pass the `goscan.Scanner` instance and the `scanner.PackageInfo` object to a dedicated `Generate` function. This function contains the core logic for that tool, iterating through the types in the `PackageInfo` and generating code based on specific annotations.
4.  **Output**: The `Generate` function is responsible for creating and writing the final `_deriving.go` file, including managing imports.

The critical insight is that the generation logic is well-encapsulated within each tool's `Generate` function. The shared signature `func Generate(ctx context.Context, gscn *goscan.Scanner, pkgInfo *scanner.PackageInfo) error` makes them composable.

## 3. Proposed Architecture for the Unified Tool

We will create a new orchestrator tool (e.g., in `examples/deriving-all/`) that decouples the scanning phase from the generation phase.

The new architecture will be as follows:

1.  **Scan Once**: The main tool will perform the scan exactly once for a given package.
2.  **Pluggable Generators**: The generation logic from `derivingjson` and `derivingbind` will be treated as "generators" or "plugins".
3.  **Orchestration**: The main tool will feed the single `PackageInfo` result to each generator in sequence.
4.  **Combined Output**: The orchestrator will collect the generated code snippets and required imports from each generator and merge them into a single output file.

## 4. Implementation Plan

### Step 4.1: Refactor the `Generate` Functions

The existing `Generate` functions are not directly reusable because they write to a file system. They need to be refactored to be pure, returning their results instead of causing side effects.

We will define a common structure to hold the output of a generator:

```go
// GeneratedCode holds the output of a single generator.
type GeneratedCode struct {
    Code    []byte
    Imports *goscan.ImportManager
}
```

The `Generate` functions in both `derivingjson` and `derivingbind` will be modified to match a new signature: `func Generate(...) (*GeneratedCode, error)`.

-   **Logic Change**: Instead of creating and saving a file, the function will write its generated Go code to a `bytes.Buffer`.
-   **Return Value**: The function will return a `GeneratedCode` struct containing the buffer's bytes and the `ImportManager` instance it used. All file I/O logic will be removed from these functions.

### Step 4.2: Create the Unified Orchestrator Tool

A new `main` package will be created (e.g., `examples/deriving-all/main.go`). This tool will:

1.  **Setup**: Initialize a single `goscan.Scanner`.
2.  **Scan**: For each target package specified in the command-line arguments, call `gscn.ScanPackage` or `gscn.ScanFiles` **once**.
3.  **Define Generators**: Create a list of the refactored `Generate` functions to be executed.
    ```go
    generators := []func(context.Context, *goscan.Scanner, *scanner.PackageInfo) (*generator.GeneratedCode, error){
        json_generator.Generate,
        bind_generator.Generate,
    }
    ```
4.  **Process**:
    -   Initialize a master `bytes.Buffer` for the combined code of the output file.
    -   Initialize a master `goscan.ImportManager` for the combined imports.
    -   Iterate through the `generators`:
        -   Execute the generator with the `PackageInfo`.
        -   If the generator returns a non-nil `GeneratedCode` struct:
            -   Append the generated code to the master buffer.
            -   Merge the imports from the generator's `ImportManager` into the master `ImportManager`.
5.  **Write Output**: If the master buffer is not empty, save the combined result to a single `_deriving.go` file. This involves using the master `ImportManager` to write the `import` block and the master buffer to write the code body.

### Step 4.3: New File Structure

To facilitate this, the project structure will be adjusted:

-   `examples/derivingjson/`
    -   `main.go` (The original CLI tool can remain for now)
    -   `gen/generate.go` (Contains the refactored, reusable `Generate` function)
-   `examples/derivingbind/`
    -   `main.go`
    -   `gen/generate.go`
-   `examples/deriving-all/`
    -   `main.go` (The new orchestrator tool that imports and uses the `gen` packages)
-   `examples/internal/generator/` (Optional, for the `GeneratedCode` struct if shared)

## 5. Conclusion

This plan achieves the goal of a single-pass scan by effectively separating the concerns of code-parsing and code-generation. By refactoring the existing generators to be pure functions, we can create a new tool that composes their functionalities efficiently, leading to better performance and a more modular design.
