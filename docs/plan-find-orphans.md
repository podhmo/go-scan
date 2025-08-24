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
    - **Multi-Workspace Support**: Provide an option to scan all Go modules found under a given directory (e.g., a repository root). This allows considering usage in sub-projects (like those in `examples/`) as valid uses for code in the main module.
- **Detailed Reporting**: By default, the tool will only report orphan functions and their locations. When the verbose flag (`-v`) is enabled, the tool will also report on non-orphan functions, listing each used function followed by a detailed list of every location (e.g., package and function name) where it is called or referenced.
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
- **CLI Parsing**: Use the standard `flag` package to parse command-line arguments: the target package path, the `-all` flag, the `--include-tests` flag, and the `-v` (verbose) flag.
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

## 5. Analysis of `symgo` and Missing Features

While `symgo` is a powerful foundation, its current implementation has several gaps that must be addressed to build the "find-orphans" tool. This analysis is based on a review of the `symgo/evaluator/evaluator.go` and related files.

### 1. Lack of a "Catch-All" Intrinsic Hook

- **Observation**: The evaluator's intrinsic mechanism is key-based, requiring an exact match for a fully qualified function name (e.g., `"fmt.Println"`). There is no way to register a default or "wildcard" handler that fires for every function call.
- **Impact**: We cannot easily track all function calls to build our usage map.
- **Required Change**: The `evalCallExpr` function in the evaluator needs to be modified. After checking for a specific intrinsic key, it should be updated to check for and invoke a registered "default" intrinsic if one exists. This default handler would receive the function object being called, allowing the tool to log every usage it encounters.

### 2. No Automatic Interface Implementation Discovery

- **Observation**: `symgo` can recognize calls on interface methods but treats the result as a generic symbolic placeholder. It does not attempt to find all the concrete types in the module that satisfy the interface. The existing `BindInterface` feature is for manual overrides, not automatic discovery.
- **Impact**: A call to an interface method (e.g., `io.Writer.Write`) will not be counted as a use of the `Write` method on the concrete types that implement it (e.g., `bytes.Buffer.Write`).
- **Required Change**: This logic must be built as part of the "find-orphans" tool itself. The tool will need to:
    1. Pre-emptively scan all types in the target workspace(s) and build a map of interfaces to their concrete implementing types.
    2. The usage-tracking intrinsic, when it intercepts a call on an interface method, will use this map to identify all relevant concrete methods and mark them as "used".

### 3. Single-Module Architecture (Resolved)

- **Observation**: The `symgo.Interpreter` was tied to a single `go-scan.Scanner` instance, which was designed to operate on a single Go module.
- **Impact**: The tool could not natively look for usages across different modules.
- **Resolution**: This limitation has been resolved by making the `goscan.Scanner` itself workspace-aware.
    - A new `WithModuleDirs` option allows the `Scanner` to be initialized with multiple module roots.
    - The `Scanner` now manages a list of `locator.Locator` instances, one for each module.
    - Its `ScanPackageByImport` method was enhanced to automatically use the correct module's locator when resolving a package, thus providing a unified view to the `symgo` interpreter without requiring changes to `symgo` itself.

## 6. Implementation Task List

This section breaks down the work required to implement the `find-orphans` tool in `examples/find-orphans`.

### Phase 1: Project Scaffolding & Basic Scanning
- [ ] Create directory `examples/find-orphans` and `main.go`.
- [ ] Set up CLI flag parsing using the standard `flag` package for:
    - [ ] `-all` (bool)
    - [ ] `--include-tests` (bool)
    - [ ] `--workspace-root` (string)
    - [ ] `-v` (bool, for verbose output)
- [ ] Implement initial `go-scan.Scanner` setup, potentially managing multiple scanners for multi-workspace support.
- [ ] Implement logic to walk the target directory/module and find all Go packages, respecting the CLI flags.
- [ ] Implement logic to scan all found packages and collect a master list of all function and method declarations (`allDeclarations`).

### Phase 2: Core Usage Analysis with `symgo`
- [ ] **(Prerequisite)** Modify `symgo` to support a "catch-all" intrinsic.
    - [ ] Add a mechanism to `symgo.Interpreter` or `symgo.evaluator.Evaluator` to register a default handler.
    - [ ] Update `evalCallExpr` to invoke this default handler for any call that doesn't match a specific intrinsic.
- [ ] In `find-orphans`, set up the `symgo.Interpreter`.
- [ ] Implement the usage-tracking intrinsic.
    - [ ] It should receive the called function object.
    - [ ] It should resolve the function's fully qualified name.
    - [ ] It should record the usage in a global `usageMap` (Key: FQN, Value: List of caller locations).
- [ ] Implement the main analysis loop: iterate through all functions in `allDeclarations` and execute them with `symgo.Interpreter.Apply()`.

### Phase 3: Advanced Usage Analysis (Interfaces)
- [ ] Implement the interface-to-concrete-type mapping.
    - [ ] Create a data structure to map interface FQNs to a list of their implementing concrete types.
    - [ ] Populate this map by iterating through all scanned types and checking their method sets against all scanned interfaces.
- [ ] Enhance the usage-tracking intrinsic to handle interface method calls.
    - [ ] When an interface method call is detected, use the map to find all corresponding concrete methods.
    - [ ] Mark each concrete method as "used" in the `usageMap`.

### Phase 4: Reporting and Final Touches
- [ ] Implement the final result analysis logic (compare `allDeclarations` with `usageMap`).
- [ ] Implement the check for the `//go:scan:ignore` annotation before reporting an orphan.
- [ ] Implement the final output formatting logic.
    - [ ] Default mode: Print only the list of orphans.
    - [ ] Verbose (`-v`) mode: For non-orphans, print the function name followed by the detailed list of locations where it was used.

## 7. Implemented Feature: Multi-Module Workspace Analysis

The `find-orphans` tool now supports analyzing a "workspace" containing multiple Go modules as a single, unified project.

### Feature Details

- **Usage**: The `--workspace-root` flag enables this mode. When provided, the tool scans the given directory for all `go.mod` files to identify the modules in the workspace.
- **Analysis Scope**: All packages from all discovered modules are included in the analysis. A function in one module is correctly considered "used" if it is called by code from any other module within the same workspace.

### Technical Implementation

Instead of creating a separate management layer or multiple `Scanner` instances, the core `goscan.Scanner` was made workspace-aware.

1.  **Workspace-Aware Scanner**: The `goscan.Scanner` can be initialized with a list of module directories via a new `WithModuleDirs` option. When this option is used, the scanner creates and manages a `locator.Locator` for each module.
2.  **Unified Package Resolution**: The `Scanner.ScanPackageByImport` method was enhanced to be workspace-aware. When asked to resolve a package, it first determines which module the package belongs to and uses the corresponding `locator`. This allows it to seamlessly find and parse packages from any module in the workspace. Standard library packages are also handled correctly.
3.  **Package Discovery**: The `find-orphans` tool's analysis logic was updated. In workspace mode, it constructs absolute path patterns for each module (e.g., `/path/to/moduleA/...`, `/path/to/moduleB/...`) and passes this combined list to the `ModuleWalker`. This ensures that the initial discovery phase finds all packages across all modules in a single walk.

## 8. Future Enhancement: Go Workspace (`go.work`) Support

To further improve multi-module analysis, the tool could be enhanced to natively understand Go workspace files (`go.work`). This would provide a more idiomatic and precise way to define the analysis scope compared to the current `--workspace-root` directory walk.

### Goal

Allow the `find-orphans` tool to use a `go.work` file as the source of truth for which modules should be included in the analysis.

### Proposed CLI

A new flag, `--go-work <path/to/go.work>`, would be introduced. If this flag is provided, it takes precedence over `--workspace-root`. The tool would parse the `go.work` file and only include modules specified in the `use` directives.

### Technical Approach

1.  **`go.work` Parser**:
    - A parser for the `go.work` file format would be needed. The format is simple (similar to `go.mod`), so it could be parsed with a regular expression or a more robust custom parser. The standard library's `golang.org/x/mod/workfile` package is the ideal candidate for this, as it's the official parser.
    - The parser would need to extract the relative paths from all `use` directives.

2.  **Module Path Resolution**:
    - The paths in `go.work`'s `use` directives are relative to the directory containing the `go.work` file.
    - The tool would need to resolve these relative paths into absolute directory paths.

3.  **Integration with `goscan.Scanner`**:
    - Once the absolute paths to all modules in the `use` directives are collected, this list of paths can be passed directly to the existing `goscan.WithModuleDirs` option when creating the `goscan.Scanner`.
    - The rest of the analysis would proceed as it does for the current `--workspace-root` implementation, as the underlying mechanism is the same.

### Example Flow

1.  User runs: `find-orphans --go-work /path/to/my/project/go.work`
2.  The tool parses `/path/to/my/project/go.work`.
3.  It finds `use ( ./module-a )` and `use ( ./libs/module-b )`.
4.  It resolves these to `/path/to/my/project/module-a` and `/path/to/my/project/libs/module-b`.
5.  It calls `goscan.New(goscan.WithModuleDirs([]string{"/path/to/my/project/module-a", "/path/to/my/project/libs/module-b"}))`.
6.  The analysis continues as normal.
