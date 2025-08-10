# MiniGo Interpreter CLI

This directory contains `minigo`, a command-line interface (CLI) for the `minigo` interpreter. The core interpreter logic is located in the `minigo` package at the root of this repository.

## Overview

This example serves as a runnable demonstration of the `minigo` interpreter. It shows how the `minigo` library, which is built on `go-scan`, can be used to execute scripts that look like a subset of Go.

### Execution Model

The `minigo` CLI has a simple execution model:
1.  It loads all script files provided as command-line arguments.
2.  It evaluates all top-level declarations (`var`, `const`, `func`, `type`) sequentially across all files.
3.  After all declarations are processed, it prints the string representation (`Inspect()`) of the value from the **very last declaration** that was evaluated.

This model is simple for quick scripts but for more complex applications, the `minigo` library's `Interpreter.Call()` method provides a more robust way to execute a specific entry point function.

## Core `minigo` & `go-scan` Features Highlighted

-   **Go Interoperability**: The interpreter can import and use real Go functions and variables. This example registers functions from the `fmt` and `strings` packages, making them available within scripts.
-   **Dynamic Symbol Resolution**: When a script imports a Go package (e.g., `import "strings"`), the `minigo` engine uses `go-scan` to find and load symbols from that package on-demand.
-   **File-Scoped Imports**: The interpreter correctly handles file-scoped imports, including standard aliased imports (`import f "fmt"`) and dot imports (`import . "strings"`). The scope of these imports is properly restricted to the file in which they are declared.
-   **Multi-File Projects**: The CLI can accept multiple script files, which are treated as being part of the same package. Symbols (variables, functions) defined in one file are available in others, but imports are not.

## Usage

To run the `minigo` interpreter, provide it with the path to a script file.

1.  **Write a script.** Create a file named `myscript.mgo` with some Go-like code:
    ```go
    // myscript.mgo
    package main
    import . "strings"

    var message = "Hello, World!"

    // The CLI evaluates this declaration last, so its value will be printed.
    var upper = ToUpper(message)
    ```

2.  **Run the CLI.** From the `go-scan` project root, execute the following command:
    ```bash
    go run ./examples/minigo ./myscript.mgo
    ```

3.  **Output:**
    ```
    HELLO, WORLD!
    ```

### Multi-File Example

1.  **Create `file_a.mgo`:**
    ```go
    // file_a.mgo
    package main
    import "fmt"
    var name = "minigo"
    ```

2.  **Create `file_b.mgo`:**
    ```go
    // file_b.mgo
    package main
    import "fmt"

    // 'name' is from file_a.mgo.
    // Since this is the last declaration evaluated across both files,
    // its value will be the script's output.
    var message = fmt.Sprintf("hello, %s", name)
    ```

3.  **Run with both files:**
    ```bash
    go run ./examples/minigo ./file_a.mgo ./file_b.mgo
    ```

4.  **Output:**
    ```
    hello, minigo
    ```
