# Plan: Type-Safe Function References in docgen Configuration

## 1. Executive Summary

The current method for configuring custom analysis patterns in `docgen` relies on string-based keys to identify functions and methods (e.g., `Key: "my.package.MyFunc"`). This approach is prone to typos and lacks IDE support (e.g., find-usages, refactoring).

This document outlines a plan to refactor this system to use direct, type-safe function and method references within the `minigo` configuration script. This will improve developer experience and the robustness of the configuration.

**Current:**
```go
// patterns.go (minigo script)
var Patterns = []patterns.PatternConfig{
	{
		Key:  "github.com/my/pkg/api.MyFunction",
		Type: patterns.RequestBody,
	},
}
```

**Proposed:**
```go
// patterns.go (minigo script)
package main

import (
    "github.com/my/pkg/api"
    "github.com/podhmo/go-scan/examples/docgen/patterns"
)

// A typed nil variable is declared for resolving method references.
var client *api.Client

var Patterns = []patterns.PatternConfig{
	{
		Fn:   api.MyFunction,      // Direct function reference
		Type: patterns.RequestBody,
	},
    {
        Fn:   client.DoRequest,    // Direct method reference
        Type: patterns.ResponseBody,
    },
}
```

## 2. Core Challenge & Investigation Insights

The primary challenge is enabling the `minigo` interpreter to resolve function and method references from a configuration script and pass them into the `docgen` tool in a structured way. Initial investigations and prior failed attempts (see reference analysis) have revealed critical insights:

1.  **Interpreter Context is Key**: A simple workaround that passes a typed `nil` and a method name as a string (e.g., `Fn: (*my.Type)(nil), MethodName: "MyMethod"`) is not feasible. The `minigo` script runs in its own context and cannot resolve types from the analyzed code without proper support from the interpreter.
2.  **The Interpreter Must Be Modified**: The only viable path is to enhance the `minigo` interpreter itself. Bypassing it leads to fundamental dead ends.
3.  **Execution Environment is Crucial**: The most significant pitfall is failing to correctly manage the execution environment of a function. When a Go function is represented as an object in the interpreter, it *must* retain a reference to the environment of the package in which it was defined (`DefEnv`). Without this, the function body cannot resolve other symbols (functions, variables) from its own package, leading to "identifier not found" errors during its symbolic execution by `symgo`.

## 3. The Corrected Implementation Plan

This plan is based on the lessons learned from previous attempts and focuses on correctly modifying the interpreter.

### Step 1: Enhance `minigo` to Represent Go Source Functions

We will introduce a new object to represent a Go function found by `go-scan`, ensuring it carries all necessary context.

- **`minigo/object/object.go`:**
    - Define a new object type, `GoSourceFunction`.
    - This object will encapsulate all necessary information about a function defined in Go source code:
        ```go
        type GoSourceFunction struct {
            PkgPath string                 // e.g., "github.com/my/pkg/api"
            Func    *scanner.FunctionInfo  // The function's metadata from go-scan
            DefEnv  *object.Environment    // CRITICAL: The environment of the package where the function was defined.
        }
        ```
    - Implement the `Type()` and `Inspect()` methods for this new object.

### Step 2: Enhance `minigo` Evaluator to Handle Go Source Functions

The evaluator needs to be taught how to find and apply these new objects.

- **`minigo/evaluator/evaluator.go`:**
    - **Symbol Resolution**: Modify `findSymbolInPackage` (or a similar function). When it resolves a function from a `goscan.PackageInfo`, it should create a `*object.GoSourceFunction`, capturing the function's info, package path, and the package's environment (`pkg.Env`).
    - **Function Application**: Modify `applyFunction`. Add a `case` for `*object.GoSourceFunction`. When it encounters this type, it must execute the function's body (`fn.Func.AstDecl.Body`) using the captured definition environment (`fn.DefEnv`) as the base. This ensures all symbols within the function's original package are resolved correctly.
    - **Method Resolution**: Update the selector logic (`evalSelectorExpr`). When resolving a method on a typed `nil` (e.g., `p.MyMethod`), it should look up the method on the receiver's type and return a `GoSourceFunction` for that method, again capturing the correct package path and definition environment.

### Step 3: Update `docgen` to Use the New Objects

The `docgen` loader will be updated to work with the new, richer objects coming from the `minigo` script.

- **`examples/docgen/patterns/patterns.go`:**
    - Modify the `PatternConfig` struct to use the `Fn any` field. The `Key` field will be removed from the user-facing configuration.

- **`examples/docgen/loader.go`:**
    - Modify `convertConfigsToPatterns`. This function receives the `[]PatternConfig` after the `minigo` script has been evaluated.
    - The `config.Fn` field will now contain a `*object.GoSourceFunction` (or a similar object for methods).
    - The logic will compute the `Key` for `symgo`'s internal matching by combining the `PkgPath` and `Func.Name` from the `GoSourceFunction` object. This is now a simple and reliable string concatenation.
        - Example for a function: `key = fmt.Sprintf("%s.%s", fn.PkgPath, fn.Func.Name)`
        - Example for a method: `key = fmt.Sprintf("(%s).%s", fn.ReceiverType.PkgPath, fn.Method.Name)`

### Step 4: Testing

- **`minigo` Unit Tests**: Add a test to verify that calling a `GoSourceFunction` correctly resolves symbols from its own package. This will directly test the `DefEnv` propagation.
- **`docgen` Integration Test**: Create an end-to-end test with a sample API and a `patterns.go` file using the new `Fn` syntax. The test will verify that the generated OpenAPI spec is correct, confirming the entire flow works as expected.
