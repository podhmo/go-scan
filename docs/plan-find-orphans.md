# Plan: Find Orphan Functions and Methods

This document outlines the plan to create a new tool for identifying "orphan" functions and methods within a Go module. An orphan is defined as a function or method that is not used anywhere within the same module.

## 1. Goals

The primary goal is to build a static analysis tool with the following capabilities:

- **Identify Orphans**: Find all functions and methods in a specified package (or all packages) that are not used within the Go module.
- **Define "Usage" Broadly**: A function/method is considered "used" if it is:
    1.  Called directly.
    2.  Assigned to a variable.
    3.  Passed as an argument to another function.
    4.  A concrete implementation of an interface method that is called.
- **Granular Control**:
    - Allow users to target a specific package.
    - Provide an `-all` flag to scan every package in the module.
    - Include an option `--include-tests` to consider usage within test files (`_test.go`). By default, test usage will be ignored.
- **Detailed Reporting**: For each orphan found, report its name and location (file and line number). For functions that *are* used, track where they are used (e.g., "used in package `bar` by function `Baz`").
- **Exclusion Mechanism**: Allow developers to mark functions/methods with a special comment (e.g., `//go:scan:ignore`) to prevent them from being reported as orphans. This is useful for functions intended for use via reflection, `cgo`, or other non-standard mechanisms.

## 2. Technical Approach

After evaluating the existing tools in this repository, the chosen approach is to build the tool on top of the `symgo` symbolic execution engine, which in turn uses `go-scan` for parsing and type resolution.

### Why `symgo`?

- **Value Tracking**: `symgo` naturally tracks the flow of values. When a function is assigned to a variable or passed as an argument, `symgo`'s evaluator understands the relationship. A simpler AST walker (`go-scan` alone) would struggle to connect an indirect call (e.g., `fn()`) back to its original declaration when `fn` is a variable.
- **Scope Management**: `symgo` maintains a lexical scope, which is crucial for correctly resolving which function is being called, especially in complex codebases.
- **Extensibility via Intrinsics**: The intrinsic system in `symgo` provides a clean and powerful mechanism to intercept all function and method calls during symbolic execution. We can use this to build our usage map.

The prohibition on `go/packages` and `go/types` in this repository's `AGENTS.md` makes the `go-scan` -> `symgo` stack the only viable option.

## 3. Implementation Steps

### Step 1: Foundational Scaffolding

- **Create a new command**: Add a new directory under `examples/`, let's call it `find-orphans`.
- **CLI Parsing**: Use the standard `flag` package to parse command-line arguments: the target package path, the `-all` flag, and the `--include-tests` flag.
- **Initial Setup**: The `main` function will initialize the `go-scan.Scanner` and the `symgo.Interpreter`. It will respect the `--include-tests` flag when walking the module.

### Step 2: Collect All Function and Method Definitions

- Use `go-scan`'s `ModuleWalker` to find all packages in the module.
- For each package, use the `scanner` to parse the files and collect a comprehensive list of all defined functions and methods.
- Store these definitions in a map, let's call it `allDeclarations`. The key will be the fully qualified name (e.g., `github.com/podhmo/go-scan/scanner.ScanPackage`) and the value will be an object containing information about the definition (e.g., `*scanner.Func`, its position, etc.).

### Step 3: Track Usage via Symbolic Execution

This is the core of the implementation.

- **Create a Global Usage Map**: Define a map, `usageMap`, to store usage information. The key will be the fully qualified name of the function/method being used, and the value will be a list of locations where it's used.
- **Implement a "Catch-All" Intrinsic**:
    - We will need a way to apply an intrinsic to *every* function call `symgo` encounters. This might require modifying `symgo`'s evaluator slightly to support a "default" intrinsic if one doesn't exist.
    - This intrinsic function will be the heart of the usage tracking. When it's triggered for a call to function `F`:
        1.  It will resolve the fully qualified name of `F`.
        2.  It will record the current execution location (package and function name) from the `symgo` interpreter's state.
        3.  It will add this location to `usageMap[F]`.
- **Handle Interface Methods**: This is the most complex part of the usage analysis.
    1.  When the intrinsic encounters a call on an interface method, it must identify this.
    2.  It will then need to find all named types in the module (`allDeclarations`) that implement this interface.
    3.  For each implementing type, the corresponding concrete method will be marked as "used" in the `usageMap`.
- **Execute Symbolically**: Iterate through every function and method in `allDeclarations` and execute it using `symgo.Interpreter.Apply()`. This will run the symbolic execution for the entire module, populating the `usageMap`.

### Step 4: Analyze and Report Results

- **Compare Declarations and Usage**: Iterate through the `allDeclarations` map. For each function/method, check if an entry exists in the `usageMap`.
- **Identify Orphans**: If a function from `allDeclarations` is not present as a key in `usageMap`, it is an orphan.
- **Check for Ignores**: Before reporting an orphan, parse its associated `ast.CommentGroup` to see if it contains the `//go:scan:ignore` annotation. If so, skip it.
- **Format Output**: Print a clean, human-readable list of all identified orphan functions, including their file path and line number.

## 4. Simulation and Feasibility Check (Self-Correction)

To validate this plan, we can simulate the process on the `go-scan` codebase itself.

- **Target**: `github.com/podhmo/go-scan`
- **Simulation**:
    1.  `go-scan` would first collect all functions, e.g., `goscan.New`, `scanner.Scanner.ScanPackage`, `astwalk.Walk`, etc.
    2.  `symgo` would then be configured with the usage-tracking intrinsic.
    3.  The tool would start symbolically executing functions, for example `goscan.New`.
    4.  Inside `goscan.New`, it sees calls to `locator.FindModuleRoot`, `modulewalker.New`, etc. The intrinsic fires for each, adding them to the `usageMap`.
    5.  If it encounters a call like `someInterface.SomeMethod()`, the logic would kick in to find all types implementing `someInterface` and mark their `SomeMethod` implementations as used.
- **Potential Challenge**: The performance of symbolically executing an entire large module.
    - **Mitigation**: We will leverage `go-scan`'s existing symbol caching mechanism to the fullest extent possible to avoid re-parsing files unnecessarily.
- **Potential Challenge**: The complexity of implementing the "catch-all" intrinsic and the interface resolution logic.
    - **Mitigation**: This will be developed carefully with extensive unit tests. The `symgo` test suite will be used as a reference for how to manipulate the interpreter and its scope.

This plan provides a clear path forward. The use of `symgo` is ambitious but necessary to meet all the requirements correctly. The implementation will proceed step-by-step, starting with the basic scaffolding and progressively adding the more complex analysis features.
