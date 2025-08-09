# MiniGo Interpreter CLI

This directory contains `minigo`, a command-line interface (CLI) for the `minigo` interpreter. The core interpreter logic is located in the `minigo` package at the root of this repository.

## Overview

This example serves as a runnable demonstration of the `minigo` interpreter. It shows how the `minigo` library, which is built on `go-scan`, can be used to execute scripts that look like a subset of Go.

The `minigo` CLI takes one or more script files as arguments, evaluates them, and prints the result of the last evaluated expression to standard output.

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
    var upper = ToUpper(message)

    upper // This is the last expression, its value will be printed.
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
    var message = fmt.Sprintf("hello, %s", name) // 'name' is from file_a.mgo
    ```

3.  **Run with both files:**
    ```bash
    go run ./examples/minigo ./file_a.mgo ./file_b.mgo
    ```

4.  **Output:**
    ```
    hello, minigo
    ```
