# Analysis of `minigo` Implementation

This document analyzes the implementation of the `minigo` script engine. `minigo` is a pragmatic, AST-walking **interpreter** designed to execute a subset of the Go language. Its primary purpose is to serve as a sandboxed, embeddable engine, most notably for using Go syntax as a dynamic configuration language.

This analysis focuses on the key design choices that differentiate `minigo`'s dynamic, runtime evaluation model from the static, compile-time model of a standard Go compiler.

## 1. Core Questions

The primary questions to be answered are:

1.  **How does `minigo`'s runtime evaluation of control flow differ from a Go compiler's static analysis?** This will establish its identity as a classic interpreter that faithfully simulates Go's runtime behavior.
2.  **What is `minigo`'s philosophy on syntax and error handling?** This will cover its direct use of `go/ast` and its approach of deferring semantic errors to runtime.
3.  **How does `minigo`'s dynamic `import` handling contrast with a Go compiler's static linking?** The analysis will cover its on-demand loading and virtual package systems, which are impossible in a compiled model.
4.  **What primary use-case does `minigo` enable that a standard Go compiler does not?** The analysis will use `examples/docgen` to demonstrate how `minigo`'s interpretive nature enables the "Go as a configuration language" paradigm.

## 2. Dynamic Execution vs. Static Compilation

The fundamental difference between `minigo` and a standard Go compiler is the "when" of execution. `minigo` evaluates code at runtime, while a compiler analyzes it ahead of time. This leads to core design differences in syntax handling, error reporting, and execution.

### 2.1. Syntax and Parsing Philosophy

A key design choice in `minigo` is its direct use of the standard `go/parser` and `go/ast` packages.

-   **`minigo` (Direct AST Interpretation)**: `minigo`'s parser accepts any syntactically valid Go code. It does not have its own pre-validation or analysis pass to check for unsupported language features. If a script uses `go` statements or `chan` types, for example, the code is parsed into a standard Go AST without complaint. This design ensures that existing Go tools—such as formatters (`gofmt`), linters, and language servers (`gopls`)—work perfectly on `minigo` scripts, as they are simply valid Go files.

-   **Go Compiler (Static Analysis & Compilation)**: A Go compiler's parser also builds an AST, but this is just the first step. It then performs extensive static analysis, type checking, and validation. It would immediately reject a program that uses channels incorrectly at compile time, long before any code is run.

### 2.2. Error Handling: Runtime vs. Compile Time

The consequence of this parsing philosophy is *when* errors are reported.

-   **`minigo` (Runtime Errors)**: Since `minigo` accepts all valid Go syntax, errors related to unsupported features are deferred until execution. An attempt to *use* a channel, for example, will result in a runtime error from the evaluator (e.g., "evaluation not implemented for `<-`"). This is a classic characteristic of an interpreter: errors are discovered as the code is run.

-   **Go Compiler (Compile-Time Errors)**: The Go compiler detects the vast majority of semantic and type errors during its static analysis phase, preventing the program from compiling at all.

### 2.3. Control Flow and Function Calls

`minigo`'s evaluator directly simulates the runtime behavior of Go, which contrasts with a compiler's abstract analysis.

-   When it encounters an `if` statement (`evalIfElseExpression`), it evaluates the condition to a concrete boolean and executes exactly one branch.
-   Its `for` loop (`evalForStmt`) iterates based on a runtime condition.
-   Function calls (`applyFunction`) involve pushing a new frame onto a call stack and executing the function body to completion.

In all cases, `minigo` is not analyzing code in the abstract; it is performing the concrete steps of an execution, statement by statement. This interpretive approach is what defines its behavior and distinguishes it from a static compiler.

## 3. Dynamic `import` Handling vs. Static Linking

`minigo`'s `import` system is another area where its dynamic nature diverges significantly from a Go compiler.

-   **Go Compiler (Static Linking)**: A Go compiler resolves all `import` statements at compile time. It finds the necessary packages, checks for dependencies, and links them into the final executable binary. This is a static, upfront process.

-   **`minigo` (Dynamic, Multi-Faceted Imports)**: `minigo` treats imports as a dynamic, runtime concern, offering a flexible, three-tiered system that a compiler cannot.

    1.  **On-Demand Source Loading**: `minigo` employs a **lazy-loading** strategy. An `import` statement itself does very little. Only when the script first attempts to access a symbol from an imported package (e.g., `pkg.Symbol`) does the `findSymbolInPackage` function trigger `go-scan` to find, parse, and evaluate the package's source code. This on-demand approach minimizes startup time, a key feature for a scripting engine.

    2.  **Virtual Packages via `interp.Register()`**: The host Go application can create "virtual packages" at runtime using `Interpreter.Register()`. It can register a set of native Go functions under an import path (e.g., `"strings"`). When a script imports and uses this path, `minigo` calls the registered Go functions directly via reflection, without ever looking for Go source files. This provides a secure and controlled mechanism to expose host functionality.

    3.  **FFI Bindings**: For performance-critical standard library packages, `minigo` supports pre-generated FFI (Foreign Function Interface) bindings. This mechanism generates Go code that registers a package's functions with the interpreter directly, bypassing both source parsing and runtime reflection.

This dynamic, multi-faceted approach to dependency resolution is a core feature that makes `minigo` highly adaptable as an embedded engine, a capability that lies outside the scope of a traditional static compiler.

## 4. Primary Use-Case: "Go as a Configuration Language"

The dynamic capabilities of `minigo` enable a powerful paradigm that is impossible with a standard Go compiler: using Go itself as a dynamic configuration language. The `examples/docgen` tool is the primary case study for this design.

A Go compiler requires a `main` function and a full compilation process to produce an executable. It cannot simply "run a file" to extract a variable value. `minigo`, as an interpreter, is designed for exactly this scenario.

The `docgen` workflow is as follows:

1.  **User-Defined Configuration**: A user writes a standard Go file (e.g., `patterns.go`) defining a `Patterns` variable. This file is not a complete, runnable program. It is a script whose sole purpose is to define and populate this configuration variable. The user can leverage Go's full syntax—functions, loops, variables—to build the final configuration value dynamically.

2.  **Host Application Embeds `minigo`**: The main `docgen` application embeds the `minigo` interpreter. It does not know the content of the user's script, only that it needs to execute it.

3.  **Script Execution**: `docgen` reads the user's Go file as a string and executes it using `interp.EvalString()`. `minigo` parses and evaluates the code on the fly.

4.  **Data Extraction via Reflection Bridge**: After the script runs, `docgen` accesses the interpreter's global environment to find the `Patterns` variable (`interp.GlobalEnvForTest().Get("Patterns")`). This returns a `minigo` internal object (`object.Array` of `object.StructInstance`). It then uses the `result.As(&configs)` method. This powerful feature acts as a bridge, using reflection to "unmarshal" the interpreter's internal objects back into a native, statically-typed Go slice (`[]patterns.PatternConfig`).

This workflow is the core value proposition of `minigo`. It allows an application to expose a configuration surface that is far more powerful than static formats like JSON or YAML, enabling logic, reuse, and type safety, all without requiring users to compile their configurations. This dynamic, script-based approach is a perfect complement to Go's static, compiled nature.

## 5. Conclusion

The analysis confirms that `minigo` is a well-designed, classic interpreter whose purpose and behavior are distinct from, and complementary to, a standard Go compiler.

-   **`minigo` is a dynamic runtime, not a static analyzer.** It faithfully simulates Go's execution behavior, including its control flow and error handling, but defers semantic error detection until runtime. Its design philosophy of accepting any valid Go syntax makes it compatible with the existing Go toolchain.

-   **Its primary value is enabling "Go as a Configuration Language."** By embedding `minigo`, a compiled Go application can execute external Go scripts on the fly. This provides a dynamic, expressive, and type-safe configuration layer that is impossible to achieve with a static compiler alone.

-   **Dynamic dependency handling is its key enabler.** `minigo`'s on-demand source loading and its ability to create virtual packages at runtime are core features that differentiate it from a static linker, making it highly suitable for a flexible, embedded scripting environment.

In summary, `minigo` is a fit-for-purpose interpreter that successfully leverages Go's syntax and AST tooling to create a powerful bridge between the static, compiled world of a host application and the dynamic, scriptable world of configuration.
