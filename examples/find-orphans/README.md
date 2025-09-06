# Find Orphans

A static analysis tool to find orphan functions and methods in a Go module or workspace. An orphan is a function or method that is not called by any other function within the analysis scope, starting from a set of defined entry points.

This tool serves as a powerful example of the dead-code analysis capabilities of the `symgo` symbolic execution engine, which is part of the `go-scan` ecosystem.

## How It Works

The tool finds unused code by performing a call-graph analysis using symbolic execution. Here is a summary of the process:

1.  **Scope Definition**: The tool first defines two important scopes:
    *   **Scan Scope**: This is the "universe" of code the tool will analyze to find function usages. It includes all Go packages found within the specified workspace (`--workspace-root`) or the current module.
    *   **Target Scope**: This is the subset of packages for which the tool will *report* orphans. This scope is defined by the package patterns you provide as arguments (e.g., `./...` or `example.com/mypkg`).

2.  **Entry Point Detection**: The analysis starts from a set of entry points. The tool determines these based on the `--mode` flag:
    *   **Application Mode** (`--mode=app`): The analysis starts from a single entry point: the `main.main` function. This mode is ideal for finding dead code in a self-contained executable.
    *   **Library Mode** (`--mode=lib`): The analysis starts from all exported functions within the **Scan Scope**. This mode is used for finding unused public APIs in a library.
    *   **Auto Mode** (`--mode=auto`, default): The tool automatically selects the mode. If a `main.main` function is found in the **Scan Scope**, it uses Application Mode; otherwise, it uses Library Mode.

3.  **Call Graph Analysis**: Starting from the entry points, the `symgo` engine traverses the call graph, marking every function and method that is reachable ("used"). The analysis is conservative: for interface method calls, it considers all concrete implementations of that method to be used.

4.  **Orphan Reporting**: After the analysis is complete, the tool compares the list of all functions against the map of "used" functions. Any function that is in a **Target Scope** package and was not marked as used is reported as an orphan.

## Usage

```sh
go run ./examples/find-orphans [flags] [patterns...]
```

The `patterns` are a list of Go package patterns (e.g., `example.com/me/mypkg/...`) or file path patterns (e.g., `./...`) that define the **Target Scope**. If no patterns are provided, it defaults to `./...`.

### Flags

-   `--workspace-root <path>`: Scan all Go modules found under a given directory. This defines the **Scan Scope**. If not provided, the scope is the current Go module.
-   `--mode <auto|app|lib>`: Explicitly set the analysis mode. Default is `auto`. Use `lib` to force library mode when a `main` package exists in the scan scope but you want to find unused library functions.
-   `--include-tests`: Include usage within test files (`_test.go`).
-   `--exclude-dirs <dirs>`: A comma-separated list of directory names to exclude from discovery (e.g., `testdata,vendor`).
-   `-json`: Output the list of orphans in JSON format.
-   `-v`: Enable verbose debug logging.

### Important Usage Notes

#### Scan Scope vs. Target Scope

It is crucial to understand the difference between the packages being scanned and the packages being targeted for reporting.

*   The **Scan Scope** is broad. It should contain all the code necessary to understand if a function is used. For accurate results, this is typically your entire multi-module workspace (`--workspace-root .`) or your entire module.
*   The **Target Scope** is narrow. It tells the tool: "I only care about orphans inside *these* packages."

**Example**: Imagine you want to find orphans in `module-b`, but `module-b` is used by `module-a`. To get correct results, you must scan both modules but only target `module-b`.

```sh
# Run from the workspace root containing module-a and module-b
go run ./examples/find-orphans --workspace-root . example.com/module-b/...
```
In this command:
-   `--workspace-root .` sets the **Scan Scope** to the entire workspace.
-   `example.com/module-b/...` sets the **Target Scope** to just the packages in `module-b`.

The tool will find usages in `module-a`, so functions in `module-b` called by `module-a` will not be reported as orphans.

#### Path Resolution

When using `--workspace-root`, all file path patterns (like `./...` or `cmd/tool`) are resolved **relative to the workspace root**, not the current working directory.

**Example**: To scan the entire repository but run the command from a subdirectory:
```sh
# CWD: /path/to/my-repo/cmd/tool
# To analyze the whole repo, targeting everything in it:
go run ./examples/find-orphans --workspace-root ../.. ./...
```
Here, `./...` is interpreted relative to `../../` (the workspace root).

## Limitations and Error Handling

### Analysis of External and Standard Library Code

To ensure fast and focused analysis, `find-orphans` configures its `symgo` engine with a **scan policy** that restricts deep analysis to only the packages within your workspace. This means that for external dependencies and the Go standard library, the tool can see function signatures but **does not trace the execution inside them**.

### Consequence: Potential for Execution Errors

A side effect of this policy is that `symgo` may not know how to handle calls to certain standard library functions for which it does not have a built-in model (an "intrinsic").

For example, you may see an error like `level=ERROR msg="not a function: INTEGER"` when the tool analyzes a `main` package that uses the `flag` package. This occurs because the tool encounters a call to `flag.String()` or `flag.Bool()`, and while it knows the function exists, it doesn't have a built-in way to simulate its behavior.

In these cases, the tool will log the error and will be unable to find orphans reachable from that specific entry point, but it will continue to analyze the rest of the workspace.
