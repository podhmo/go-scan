# Plan: minigo config-as-code

This document outlines the plan to add a new feature to `minigo` that allows it to load Go files as configuration and output the result as JSON.

## Usecases

1.  **File-based configuration**: A user can specify a Go file and a function within that file to be executed. The result of this function call is then serialized to JSON.
    ```bash
    minigo --file config.go --func "Config()" --output json
    ```

2.  **Self-contained script**: A user can write a `minigo` script that imports the configuration and then explicitly calls a `toJSON` function.
    ```go
    // script.go
    package main
    import "config"
    import "encoding/json"

    var result, err = json.Marshal(config.Config())
    ```
    ```bash
    minigo script.go
    ```
    (This usecase is already supported, but we will test it.)

3.  **Command-line Snippet**: A user can provide a snippet of Go code directly on the command line for evaluation.
    ```bash
    minigo -c 'import "config"; config.Config()' --output json
    ```

## Implementation Plan

1.  **Enhance `main.go`**:
    -   Add flag parsing to support `--file`, `--func`, and `--output`.
    -   Add support for a `-c` flag for inline code snippets.
    -   Create a new `run` function that encapsulates the logic for the new execution modes.
    -   The `run` function will:
        -   Initialize the interpreter.
        -   Load the file or code snippet.
        -   Call the specified function.
        -   If `--output json` is specified, marshal the result to JSON and print it.
        -   Otherwise, print the result using the default `Inspect()` method.

2.  **Add Test Cases**:
    -   Create `main_test.go` if it doesn't exist.
    -   Add test cases for each of the three use cases.
    -   The tests will execute the `minigo` command with the new flags and verify the output.

3.  **Update `TODO.md`**:
    -   Add tasks for the implementation and testing of this new feature.
    -   Update the tasks as they are completed.
