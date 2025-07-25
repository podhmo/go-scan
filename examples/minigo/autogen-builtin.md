# Proposal for Auto-generating MiniGo Builtin Functions

## Overview

To streamline the process of adding new builtin functions to the MiniGo interpreter and ensure consistency, this document proposes the introduction of a tool to auto-generate the necessary code for builtin functions (such as `BuiltinFunction` object definitions, argument processing, and type checking).

## Background

Currently, adding new builtin functions (e.g., `fmt.Sprintf`, `strings.Join`) involves the following manual steps:

1.  Implement an `evalXxx` function that contains the actual logic of the function.
2.  Write code to wrap the `evalXxx` function and create a `BuiltinFunction` object.
3.  Write boilerplate code to check the number and types of arguments.
4.  Add an entry to the interpreter's registration map (e.g., `GetBuiltinFmtFunctions`).

These tasks are repetitive and can become cumbersome and error-prone as the number of functions increases.

## Design Policy

### 1. Input: Annotated Go Source Files

The specifications for builtin functions will be defined by writing special comment-based annotations in Go source files.

*   **Annotation Target:** Go source files (`*.go`) located in a dedicated directory (e.g., `builtins_src/`).
*   **Annotation Format:**
    *   `//minigo:builtin name=<minigo_func_name> [target_go_func=<go_func_name> | wrapper_func=<custom_wrapper_name>]`
        *   `name`: The function name within the MiniGo interpreter (e.g., `"strings.Contains"`, `"fmt.Sprintf"`).
        *   `target_go_func` (optional): A Go standard library function or a custom Go function to be called directly (e.g., `"strings.Contains"`). If specified, the auto-generation logic will handle argument and return value type conversions.
        *   `wrapper_func` (optional): The name of a custom Go function that implements the entire logic from argument processing to calling the Go function and converting return values (e.g., `"main.evalFmtSprintfOriginal"`). Intended for functions with more complex logic. Mutually exclusive with `target_go_func`.
    *   `//minigo:args [variadic=true] [format_arg_index=<idx>]`
        *   `variadic`: Specifies if the function takes a variable number of arguments.
        *   `format_arg_index`: Specifies the index of the format string argument for functions like `fmt.Sprintf`.
    *   `//minigo:arg index=<idx> name=<arg_name> type=<MINIGO_TYPE> [go_type=<GO_TYPE>]`
        *   `index`: The argument index (0-based).
        *   `name`: The argument name (for documentation and error messages).
        *   `type`: The expected type in MiniGo (`STRING`, `INTEGER`, `BOOLEAN`, `ARRAY`, `MAP`, `ANY`, etc.).
        *   `go_type` (optional): The Go type when calling `target_go_func`. If omitted, it's inferred from `MINIGO_TYPE`.
    *   `//minigo:return type=<MINIGO_TYPE> [go_type=<GO_TYPE>]`
        *   `type`: The return type in MiniGo.
        *   `go_type` (optional): The Go type of the return value from `target_go_func`. If omitted, it's inferred from `MINIGO_TYPE`.

*   **Stub Functions:** Annotations are written immediately before a Go function declaration. This Go function acts as a stub. If `target_go_func` is specified, its body can be empty and is used for type checking and documentation hints. When using `wrapper_func`, a stub can also be defined for reference regarding argument structure.

**Example (`builtins_src/strings_builtins.go`):**
```go
package builtins_src

import "strings" // For use with target_go_func

//minigo:builtin name="strings.Contains" target_go_func="strings.Contains"
//minigo:arg index=0 name=s type=STRING go_type=string
//minigo:arg index=1 name=substr type=STRING go_type=string
//minigo:return type=BOOLEAN go_type=bool
func containsStub(s string, substr string) bool { return false }

//minigo:builtin name="custom.StringLength" wrapper_func="main.evalCustomStringLength"
//minigo:arg index=0 name=str type=STRING
//minigo:return type=INTEGER
func customStringLengthStub(str string) int { return 0 }
```

### 2. Generated Code

The auto-generation tool will parse the above annotations and generate the following Go code (e.g., in `builtin_generated.go`):

*   **`BuiltinFunction` Object Definitions:** Generate a slice or map of `object.BuiltinFunction` based on annotations.
*   **Wrapper Functions:**
    *   If `target_go_func` is specified:
        *   Check the number and types of arguments.
        *   Convert MiniGo `Object` types to the specified `go_type`.
        *   Call `target_go_func`.
        *   Convert the resulting Go type back to a MiniGo `Object` type.
        *   Handle errors.
    *   If `wrapper_func` is specified:
        *   A simple wrapper that calls the specified `wrapper_func`. Basic argument count checks can be standardized.
*   **Registration Helper Function:** A function that provides access to all generated builtin functions (e.g., `GetGeneratedBuiltinFunctions() map[string]*object.BuiltinFunction`).

### 3. Application to Existing Builtin Functions

*   **For `fmt.Sprintf`, `strings.Join` (current special implementations), etc.:**
    *   These will use `wrapper_func` to specify the existing functions like `evalFmtSprintf` or `evalStringsJoin` (renamed if necessary, e.g., to `main.evalFmtSprintfOriginal`).
    *   Annotated stub functions will be placed under `builtins_src/`.
    ```go
    // builtins_src/fmt_builtins.go
    package builtins_src

    //minigo:builtin name="fmt.Sprintf" wrapper_func="main.evalFmtSprintfOriginal"
    //minigo:args variadic=true format_arg_index=0
    //minigo:return type=STRING
    func SprintfStub(format string, a ...interface{}) string { return "" }
    ```
*   This approach allows leveraging existing complex logic while centralizing definition management.

## Auto-generation Tool Interface

### Command Name: `minigo-builtin-gen`

### Command-Line Options:

*   `minigo-builtin-gen -source <source_dir> -output <output_file>`
    *   `-source <source_dir>`: Directory containing Go source files with annotations (e.g., `./builtins_src`).
    *   `-output <output_file>`: Output file path for the generated Go code (e.g., `builtin_generated.go`).
    *   (Optional) `-package <pkg_name>`: Package name for the generated code (default: `main`).
    *   (Optional) `-v`: Verbose logging output.

### Integration with `go:generate`:

Add the following to a key Go file in the interpreter (e.g., `main.go`):
```go
//go:generate minigo-builtin-gen -source ./builtins_src -output builtin_generated.go
package main
```
Run `go generate ./...` to execute code generation.

## Advantages

*   **Improved Development Efficiency:** Adding new builtin functions becomes faster and easier.
*   **Ensured Consistency:** Argument handling and error handling styles are unified.
*   **Reduced Bugs:** Fewer errors from writing boilerplate code.
*   **Improved Readability:** Builtin function specifications are consolidated in annotations, improving clarity.
*   **Enhanced Maintainability:** The impact of specification changes is localized to annotations and the generation tool.

## Future Considerations

*   Expanding the types and argument patterns expressible via annotations (e.g., `ANY` type, types satisfying specific interfaces).
*   Customizability of generated error messages.
*   More advanced type inference (e.g., automatic determination of `go_type`).
*   Possibility of auto-generating test code.

## Implementation Challenges and Considerations (Based on strings package generation simulation)

Simulating the generation of builtin functions for the `strings` package using the proposed annotation method revealed areas for consideration:

### 1. Handling of `strings.Join` (Post-Refactor)

Previously, `strings.Join` in MiniGo had a special variadic signature (elements followed by a separator). This signature historically arose when MiniGo's support for array/slice types in builtin function signatures was less direct. This posed challenges for annotation (P1) and led to discrepancies between stub and actual logic (P2) if using `wrapper_func`.

**However, `strings.Join` has since been refactored to accept an `object.Slice` of `object.String`s as its first argument and an `object.String` as its second argument (the separator).** This aligns its signature much more closely with the standard Go `strings.Join(slice []string, sep string)`.

This refactoring significantly simplifies its definition for auto-generation:

*   **Simplified Annotations:** The P1 challenge (Annotation Syntax for Special Argument Parsing) is no longer a major concern for `strings.Join`. It now has a standard two-argument structure. An annotation could look like:
    ```go
    //minigo:builtin name="strings.Join" wrapper_func="main.evalStringsJoin" // evalStringsJoin is the refactored version
    //minigo:arg index=0 name=elements type=SLICE // Ideally SLICE<STRING> if supported by auto-gen
    //minigo:arg index=1 name=sep type=STRING
    //minigo:return type=STRING
    func stringsJoinStub(elements []string, sep string) string { return "" }
    ```
*   **Improved Stub Accuracy:** The P2 challenge (Discrepancy Between Stub Function Signature and Actual Logic) is greatly reduced. The stub function `func stringsJoinStub(elements []string, sep string) string` is now a good representation of the function's parameters.
*   **Potential for `target_go_func`:** While the example above uses `wrapper_func` (pointing to the Go function `main.evalStringsJoin` that handles `object.Slice` to `[]string` conversion), the refactored signature makes `strings.Join` a better candidate for `target_go_func` if:
    *   The auto-generation tool becomes capable of handling the conversion from `object.Slice` (with `type=SLICE<STRING>`) to a Go `[]string` automatically.
    *   Or, a simple Go helper function `func goStringsJoin(elems []string, sep string) string` is created, and `target_go_func` points to this, with the auto-generator handling `object.Slice` -> `[]string` and `object.String` -> `string` conversions.

This evolution of `strings.Join` demonstrates a positive path for aligning builtins with auto-generation ideals. The main remaining challenge for collection types like slices in `target_go_func` is the auto-generation tool's capability to infer or be explicitly told about the element types within the collection (e.g., `SLICE<STRING>`) and generate the necessary conversion and type-checking logic for these elements.

### 2. `wrapper_func` Dependencies and Scope

*   **P3: Visibility and Package Dependency of `wrapper_func`:**
    *   If an annotation specifies a `wrapper_func` from the interpreter's main package (e.g., `wrapper_func="main.evalStringsJoinOriginal"`), the annotation processing tool and the generated code must be able to access that function correctly.
    *   If the `builtins_src` directory is managed as a separate package from `main` (as is typical), the generated code will need proper imports to call functions in the `main` package. The generation tool might need to resolve this automatically, or there should be a convention that `wrapper_func` must be a publicly accessible function satisfying a certain interface.
    *   Circular dependency issues also need to be considered.

### 3. Convenience for Simple Functions (e.g., `strings.Contains`, `strings.ToUpper`)

*   Functions that can be directly mapped to Go standard library functions, like `strings.Contains(s string, substr string) bool` or `strings.ToUpper(s string) string`, are expected to be auto-generated relatively smoothly using the proposed `target_go_func` annotation.
    ```go
    // builtins_src/strings_builtins.go
    package builtins_src
    import "strings"

    //minigo:builtin name="strings.Contains" target_go_func="strings.Contains"
    //minigo:arg index=0 name=s type=STRING go_type=string
    //minigo:arg index=1 name=substr type=STRING go_type=string
    //minigo:return type=BOOLEAN go_type=bool
    func stringsContainsStub(s string, substr string) bool { return false }
    ```
    The main task here would be generating the standard conversion code between MiniGo types (`STRING`, `BOOLEAN`) and Go types (`string`, `bool`).

Addressing these challenges might involve expanding the annotation vocabulary, clarifying conventions for using `wrapper_func`, or refining the logic of the generation tool.

This proposal is expected to make the development of MiniGo builtin functions more robust and efficient.
