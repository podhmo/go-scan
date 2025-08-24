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

### 3. Single-Module Architecture

- **Observation**: The `symgo.Interpreter` is tied to a single `go-scan.Scanner` instance, which is designed to operate on a single Go module (defined by one `go.mod`).
- **Impact**: The tool cannot natively look for usages across different modules, such as a main project and its examples in a sub-directory.
- **Required Change**: This requires a significant architectural enhancement, likely starting at the `go-scan` level. The "find-orphans" tool will need to orchestrate multiple `Scanner` instances. A potential long-term solution involves creating a "workspace" or "multi-scanner" concept that can manage multiple modules and present a unified view to the `symgo` interpreter. For the initial implementation, the tool may need to manage a list of scanners and query them all.

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

## 7. Future Enhancement: Multi-Module Workspace Analysis

### Goal

To allow `find-orphans` to analyze a "workspace" containing multiple Go modules (e.g., a main project and sub-projects in `examples/`) as a single unit. This means a function in one module will be considered "used" if it's called by code from another module within the same workspace.

### Proposed CLI

The `--workspace-root` flag will enable this mode. When provided, the tool will:

1.  Find all `go.mod` files under the workspace root.
2.  Include all packages from all discovered modules in the analysis.

### Technical Approach

The current `goscan.Scanner` is designed to work with a single `go.mod` file. To support multiple modules, a simple management layer will be introduced.

1.  **Module-Specific Scanners**: For each `go.mod` found, a dedicated `goscan.Scanner` instance will be created.
2.  **Package Lookups**: When `symgo` needs to analyze a package, this new management layer will direct the request to the correct `Scanner` responsible for that module. This allows `symgo` to see the source code of packages from any module in the workspace.

This approach focuses only on resolving calls between user-written code in the workspace and does not need to handle complex dependency conflicts between external libraries.
