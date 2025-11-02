# Definition of an "Orphan" in `find-orphans`

This document details the technical definition of an "orphan" as detected by the `find-orphans` tool.

## Basic Definition

An orphan is **a function or method that is unreachable from a defined set of entry points within the analysis scope.**

In other words, a function or method that is never called when tracing the call graph from the program's starting points is considered an orphan.

## Components of Analysis

The definition of an orphan depends on three key components:

1.  **Analysis Scope**
    -   This is the "universe" of code that the tool scans to analyze function usage.
    -   It typically includes the entire workspace (specified by `--workspace-root`) or a single Go module. All function calls within this scope are used to determine usage.

2.  **Target Scope**
    -   This is the set of packages for which the tool will **report** orphans.
    -   It is defined by the package patterns provided as command-line arguments (e.g., `./...`).
    -   Even if a function is called from code within the analysis scope, if that function does not reside within the target scope, it will not be reported as an orphan.

3.  **Analysis Mode and Entry Points**
    -   The analysis is conducted based on the mode specified by the `--mode` flag. The mode determines which functions serve as the starting points for call graph traversal and the criteria for what constitutes an "orphan."
    -   **Global Variable Initializers**:
        -   **Entry Point**: In all modes (`app`, `lib`, `auto`), the first step of the analysis is to evaluate the **initializer expressions of all global variables (`var`)**.
        -   **Orphan Judgment**: Any function called within these initializer expressions is immediately marked as "used." This occurs before the execution of `main`, `init`, or any other function, making it the most fundamental entry point for the analysis.
    -   **Application Mode (`app`)**:
        -   **Entry Point**: By default, all `main.main` functions found in the analysis scope. This behavior can be modified using the `--entrypoint-pkg` flag.
        -   **`--entrypoint-pkg` Flag**: This flag allows you to specify a comma-separated list of package import paths. When provided, only the `main.main` functions from these specific packages will be used as entry points for the analysis. This is useful for targeting a single binary in a repository that contains multiple `main` packages.
        -   **Orphan Judgment**: Any function or method unreachable from the selected `main.main` entry point(s) is an orphan. The entry point(s) themselves are considered "used" by definition and will not be reported as orphans.
        -   **Use Case**: Suitable for detecting dead code in an executable binary, especially in multi-binary repositories.
    -   **Library Mode (`lib`)**:
        -   **Entry Points**: **All exported (public) functions and methods, all `init` functions, and the `main.main` function** within the analysis scope.
        -   **Orphan Judgment**: The purpose of library mode is to find unused parts of a public API. Therefore, an exported function that serves as an entry point will **still be reported as an orphan if it is not called by any other code**. In other words, while exported functions are starting points for traversing the call graph, they are not automatically considered "used" just by virtue of being entry points.
        -   **Use Case**: Suitable for detecting unused APIs in a library.
    -   **Auto Mode (`auto`, default)**:
        -   Automatically selects application mode if a `main.main` function is found in the analysis scope; otherwise, it selects library mode.

## Summary of the Orphan Detection Process

1.  The tool first determines the analysis and target scopes.
2.  **In Application Mode**: It marks `main.main` as "used."
3.  The symbolic execution engine (`symgo`) recursively traverses the call graph starting from the entry points (just `main.main` in app mode; all exported functions/methods in library mode).
4.  All functions and methods called during this process are marked as "used."
    -   If an entry point (e.g., exported function A) can reach a function (e.g., unexported function f), and f in turn calls another function (e.g., exported function H), then H is also considered "used." All intra-package calls are taken into account.
    -   For interface method calls, a conservative analysis is performed where all methods of concrete types that implement the method are considered "used."
    -   **Error Handling**: When analyzing many functions in library mode, if an error occurs while analyzing a specific function, the tool does not halt. It logs the error and continues analyzing the remaining functions.
5.  After the analysis is complete, any function that was not marked as "used" and belongs to the **target scope** is reported as an orphan. Consequently, in library mode, an exported function or method that is not called by any other function or method can also be reported as an orphan.

## `symgo` Evaluation Target and Analysis Scope

`find-orphans` internally uses a symbolic execution engine named `symgo` to analyze function usage. Understanding the scope of code that `symgo` evaluates is crucial for comprehending the tool's behavior.

-   **Analysis Scope**
    -   The set of Go packages that `symgo` analyzes at the source code level, defined by the `--workspace-root` flag (or the current module if not specified), is called the "Analysis Scope."
    -   **Functions/Methods within Scope**: `symgo` evaluates the full implementation of these functions and methods. That is, if function A calls function B, this call is accurately traced, and B is recorded as "used."

-   **Functions/Methods outside Analysis Scope**
    -   This includes functions from external dependency libraries (e.g., `gopkg.in/yaml.v3`) that are not part of the analysis scope.
    -   Since `symgo` does not have the source code for these functions, it can recognize their function signatures (arguments and return types) but **cannot trace their internal behavior**.
    -   For example, if code within the analysis scope calls a function from an external library, `symgo` recognizes that call but does not track which functions are called internally by that external function. The analysis chain stops there.

> **Implementation Note**
> To achieve the behavior described above, the `find-orphans` tool is responsible for limiting the packages passed to the `symgo` engine to only those within the predefined analysis scope. If this filtering fails and symbol information from outside the analysis scope is passed to `symgo`, it could lead to unexpected errors.

### Excluded Cases

The following functions will not be reported as orphans:

-   `main.main` and `init` functions (as they are special entry points defined by the language specification).
-   Functions with the `//go:scan:ignore` annotation.
-   Test entry point functions within `_test.go` files (e.g., `TestXxx`, `BenchmarkXxx`). While they may be treated as analysis entry points, they are not themselves reported as orphans.

---

Related Documents:
-   `symgo` Engine Specification: [`sketch/plan-symbolic-execution-like.md`](../../docs/plan-symbolic-execution-like.md)
