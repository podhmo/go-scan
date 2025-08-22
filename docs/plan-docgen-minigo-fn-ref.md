# Plan: Implementing Type-Safe Function References in `docgen`

## 1. Objective

The primary goal of this task is to refactor the `docgen` tool's custom pattern configuration to be type-safe and developer-friendly. The current method relies on string-based keys to identify target functions and methods (e.g., `Key: "my/pkg.MyFunc"`), which is fragile and error-prone.

This will be replaced by a mechanism that allows developers to use direct function and method references in the `minigo` configuration script (e.g., `Fn: mypkg.MyFunc`).

## 2. Investigation & Analysis

A thorough investigation, including analysis of previous failed attempts, has been conducted to understand the core challenges.

### 2.1. The `minigo` Interpreter Context

The `minigo` script that defines the patterns runs in its own isolated context. It does not automatically have knowledge of the Go types or functions from the user's code that is being analyzed. A simple approach, such as passing a string for a method name alongside a typed `nil` value, fails because the interpreter cannot resolve the type identifier in the first place. Therefore, the interpreter itself must be enhanced to bridge this context gap.

### 2.2. The "Typed Nil" Problem for Method References

A key requirement is to support method references, such as `Fn: (*MyType)(nil).MyMethod`. To achieve this, the `minigo` interpreter must be able to handle expressions on `nil` pointers without losing their type information. The standard `nil` value in Go is untyped, but for this to work, we need a "typed nil".

The solution requires modifying the `minigo` evaluator to recognize when a selector expression (e.g., `.MyMethod`) is being applied to a `nil` pointer that has a known Go type. Instead of producing a nil-pointer-dereference error, the evaluator must look up the method on the type itself and return a new object representing that method value.

### 2.3. The Crucial Role of the Definition Environment (`DefEnv`)

The most critical insight from past attempts is the importance of a function's execution environment. When a Go function from source is represented as an object within the `minigo` interpreter, it is not enough to simply store a reference to its AST. It **must** also store a reference to the environment of the package in which it was defined (`DefEnv`).

When `symgo` (the symbolic execution engine) later "calls" this function, it must do so within that function's original `DefEnv`. If this context is lost, the function's body will be unable to resolve any other functions, variables, or types from its own package, leading to cascading "identifier not found" errors. Prior attempts failed precisely because this `DefEnv` was not correctly captured and propagated.

## 3. Detailed Implementation Plan

The implementation will proceed by first enhancing `minigo` and then updating `docgen` to use the new capabilities.

### 3.1. Enhance `minigo` to Represent Go Source Functions

A new, context-aware object will be introduced to represent a Go function.

- **File**: `minigo/object/object.go`
- **Action**:
    1.  Define a new object type: `GoSourceFunction`.
    2.  This struct must encapsulate all necessary context:
        ```go
        type GoSourceFunction struct {
            PkgPath string                 // The full package path, e.g., "github.com/my/pkg/api"
            Func    *scanner.FunctionInfo  // Metadata from go-scan (AST, name, etc.)
            DefEnv  *object.Environment    // CRITICAL: The environment of the package where the function was defined.
        }
        ```
    3.  Implement the `object.Object` interface (`Type()` and `Inspect()` methods) for `GoSourceFunction`.

### 3.2. Enhance the `minigo` Evaluator

The evaluator must be taught how to create and handle the new `GoSourceFunction` objects.

- **File**: `minigo/evaluator/evaluator.go`
- **Actions**:
    1.  **Symbol Resolution**: Modify the `findSymbolInPackage` function. When it resolves a function from a `goscan.PackageInfo`, it must create and return a `*object.GoSourceFunction`, ensuring it correctly captures and stores the package's environment (`pkg.Env`) in the `DefEnv` field.
    2.  **Method Resolution**: Update the selector logic in `evalSelectorExpr`. When it resolves a method on a typed `nil` pointer, it should also create and return a `GoSourceFunction` representing that method, again ensuring the `DefEnv` of the receiver type's package is captured.
    3.  **Function Application**: Modify `applyFunction`. Add a `case` to handle `*object.GoSourceFunction`. When this type is called, the evaluator must use the captured `fn.DefEnv` as the base environment for evaluating the function's body. This is the key to fixing the symbol resolution issue.

### 3.3. Update `docgen` Pattern Loading

The `docgen` tool will be updated to leverage the new, richer objects from `minigo`.

- **File**: `examples/docgen/patterns/patterns.go`
- **Action**:
    -   Modify the `PatternConfig` struct. Remove the string `Key` field and replace it with `Fn any`.

- **File**: `examples/docgen/loader.go`
- **Action**:
    -   Modify the `convertConfigsToPatterns` function. It will now receive `PatternConfig` structs where `Fn` holds a `*object.GoSourceFunction`.
    -   Compute the internal matching `Key` required by `symgo` by reliably combining the `PkgPath` and `Func.Name` from the received `GoSourceFunction` object. This eliminates the need for reflection hacks like `runtime.FuncForPC`.

## 4. Testing Strategy

Previous attempts were blocked by module resolution issues in file-based tests. The new strategy will use a more hermetic, programmatic approach to avoid these environmental problems.

- **File**: `examples/docgen/main_test.go`
- **Action**:
    1.  Create a new test function (`TestDocgen_WithFnPatterns_Programmatic`).
    2.  Define required API types and functions (e.g., `type api.User`, `func api.SendJSON(...)`) as local helper types/funcs inside the test file itself.
    3.  Construct the `minigo` pattern script as a multi-line string within the test.
    4.  Use the `minigo.WithGlobals` interpreter option to inject the helper functions and types from the test directly into the `minigo` script's global scope. This completely bypasses all filesystem and Go module resolution issues.
    5.  Execute the `docgen` analysis using this interpreter.
    6.  Verify the generated OpenAPI output against a golden file.

## 5. Task List

1.  **`minigo` Object**: Define the `GoSourceFunction` struct in `minigo/object/object.go`.
2.  **`minigo` Evaluator**: Update `findSymbolInPackage` and `evalSelectorExpr` in `minigo/evaluator/evaluator.go` to create `GoSourceFunction` objects with the correct `DefEnv`.
3.  **`minigo` Evaluator**: Update `applyFunction` in `minigo/evaluator/evaluator.go` to correctly use the `DefEnv` when executing a `GoSourceFunction`.
4.  **`minigo` Test**: Add a unit test to verify that a `GoSourceFunction` can correctly resolve symbols from its own package via its `DefEnv`.
5.  **`docgen` Struct**: Update `PatternConfig` in `examples/docgen/patterns/patterns.go` to use the `Fn any` field.
6.  **`docgen` Loader**: Update `convertConfigsToPatterns` in `examples/docgen/loader.go` to compute the key from the `GoSourceFunction` object.
7.  **`docgen` Test**: Implement the new programmatic testing strategy in `examples/docgen/main_test.go`.
8.  **Submit**: Once all tests pass, submit the completed feature.
