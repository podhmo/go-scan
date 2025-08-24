# Find Orphans

A static analysis tool to find orphan functions and methods in a Go module. An orphan is a function or method that is not called by any other function within the same module, starting from a set of entry points.

This tool serves as a test pilot for demonstrating the dead-code analysis capabilities of the `symgo` symbolic execution engine, which is part of the `go-scan` ecosystem.

## Usage

```sh
go run ./examples/find-orphans [flags] [patterns]
```

By default, it scans from the current directory (`./...`).

### Flags

-   `-all`: Scan every package in the module. (Note: This is often the default behavior, but the flag can be used for clarity).
-   `--include-tests`: Include usage within test files (`_test.go`).
-   `--workspace-root <path>`: Scan all Go modules found under a given directory. This is useful for multi-module repositories.
-   `-json`: Output the list of orphans in JSON format instead of plain text.
-   `-v`: Enable verbose debug logging.

## How it Works

The `find-orphans` tool is built on top of several key features of the `go-scan` library:

1.  **Package Scanning**: It uses `goscan.New` and `Walker.Walk` to discover and parse all Go packages within the target module or workspace. It can handle both single-module projects and complex multi-module setups using `go.work` files.

2.  **Symbolic Execution with `symgo`**: The core of the analysis is performed by the `symgo` engine (`symgo.NewInterpreter`). The tool performs a symbolic execution pass over the code to build a call graph.

3.  **Entry Point Detection**: The analysis starts from a set of entry points.
    -   If a `main` package with a `main` function is found, the tool runs in "application mode," and `main.main` is the sole entry point.
    -   If no `main` function is found, the tool runs in "library mode," and all exported functions in the scanned packages are considered potential entry points.

4.  **Usage Tracking**: During symbolic execution, `symgo` tracks every function and method that is called. It maintains a `usageMap` of all reachable functions.

5.  **Orphan Identification**: After the analysis is complete, the tool iterates through all the functions and methods discovered during the initial package scan. Any function that does not appear in the `usageMap` is considered an "orphan" and is reported.

This example showcases how `go-scan` and `symgo` can be combined to perform deep, inter-procedural static analysis to identify potentially dead or unused code within a Go project.
