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

## 6. Testing Strategy

The `scantest` package is the ideal tool for testing the new unified generator. It allows for the creation of isolated, in-memory test environments, preventing the need for golden files and ensuring tests are fast and self-contained.

The testing approach will be as follows:

1.  **Test Case Setup**: For each test case, use `scantest.WriteFiles` to create a temporary directory with a set of Go source files representing the input for the generator. This can include single or multiple files, and even a `go.mod` file if needed.

    ```go
    dir, cleanup := scantest.WriteFiles(t, map[string]string{
        "go.mod": "module example.com/me",
        "models.go": `
    package models
    // deriving:unmarshal
    // derivng:binding in:"body"
    type User struct {
        Name string `json:"name"`
        ID   int    `json:"id"`
    }`,
    })
    defer cleanup()
    ```

2.  **Action Definition**: Define an `ActionFunc` that will be executed by `scantest.Run`. This function will contain the core logic of the test. It will invoke the unified generator and perform assertions.

3.  **Execution and Assertion**: Use `scantest.Run` to execute the scanner and the action. The `Run` function captures any generated files in its `Result` object. The test will then assert on the content of this captured output.

    ```go
    action := func(ctx context.Context, s *scan.Scanner, pkgs []*scan.Package) error {
        // The unified generator's main logic would be called here.
        // This is a simplified representation.
        // It would internally call the json and bind generators.
        // For the test, we can simulate this by calling a wrapper that
        // orchestrates the generation and writes the output.

        // This call to SaveGoFile uses the memoryFileWriter from the context
        // provided by scantest.Run
        return unifiedGenerator.GenerateAndSave(ctx, s, pkgs)
    }

    result, err := scantest.Run(t, dir, []string{"."}, action)
    if err != nil {
        t.Fatal(err)
    }

    // Assert on the output
    if result == nil {
        t.Fatal("expected a non-nil result for a file generation action")
    }

    generatedCode, ok := result.Outputs["models_deriving.go"]
    if !ok {
        t.Fatal("expected generated file was not in the result")
    }

    // Check for content from both generators
    if !strings.Contains(string(generatedCode), "UnmarshalJSON") {
        t.Error("generated code does not contain UnmarshalJSON method")
    }
    if !strings.Contains(string(generatedCode), "Bind(r *http.Request)") {
        t.Error("generated code does not contain Bind method")
    }
    ```

This strategy allows us to test the entire flow of the unified generator—from scanning to code generation—in a controlled and reproducible manner. We can easily create test cases for various scenarios, including structs with one, both, or no annotations, and verify that the combined output is correct.

## 7. `go-scan` Library Impact

After further analysis, we can confirm that **no changes are required to the core `go-scan` library**. The existing components, including the `Scanner`, `ImportManager`, `ResolveType`, and `Implements` functions, are sufficient to support the refactored, composable generators. The library's design is flexible enough to accommodate this new unified approach without modification.

## 8. Incremental Development Plan (TODO)

This project can be broken down into the following incremental steps.

-   [ ] **Step 1: Create the `GeneratedCode` struct.**
    -   Define the shared `GeneratedCode` struct (or a similar structure) to standardize the output of all generators. This could be in a new package like `examples/internal/generator`.

-   [ ] **Step 2: Refactor the `derivingjson` generator.**
    -   Create a new `gen/generate.go` file in the `derivingjson` example.
    -   Move the `Generate` function into this new file.
    -   Modify its signature to `func Generate(...) (*generator.GeneratedCode, error)`.
    -   Update the function to return the generated code and imports instead of writing to a file.
    -   Update the original `derivingjson/main.go` to call this new function and handle the file writing, ensuring the standalone tool still works.

-   [ ] **Step 3: Refactor the `derivingbind` generator.**
    -   Repeat the process from Step 2 for the `derivingbind` example.

-   [ ] **Step 4: Implement the initial version of the unified `deriving-all` tool.**
    -   Create the `examples/deriving-all/main.go` file.
    -   Implement the core logic: initialize the scanner, scan the package, and call the refactored `json` and `bind` generators.
    -   Implement the logic to merge the `GeneratedCode` results (both code and imports) from all generators.
    -   Write the combined result to a single output file.

-   [ ] **Step 5: Implement tests for the unified generator using `scantest`.**
    -   Create a new test file, `deriving_all_test.go`.
    -   Write a test case for a struct with only the `deriving:unmarshal` annotation.
    -   Write a test case for a struct with only the `derivng:binding` annotation.
    -   Write a test case for a struct with both annotations, and verify that the output file contains the code from both generators.
    -   Write a test case for a struct with no relevant annotations to ensure an empty output is handled gracefully.

-   [ ] **Step 6: Refine and Finalize.**
    -   Clean up the code, add comments, and ensure the CLI interface for the new tool is user-friendly.
    -   Update the main `README.md` to document the new unified tool.
