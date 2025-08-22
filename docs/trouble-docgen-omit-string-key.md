# Trouble: Refactoring docgen to use type-safe function references

This document outlines the series of failed attempts to refactor the `docgen` tool's custom pattern configuration. The goal was to replace string-based keys like `Key: "my.package.MyFunc"` with direct, type-safe function references like `Fn: my.package.MyFunc` in a `minigo` script.

## The Core Challenge

The primary difficulty was handling method references on nil pointers, such as `var p *my.Type; ... { Fn: p.MyMethod }`. The `minigo` interpreter, by default, would evaluate `p` to a raw `object.NIL`, losing all type information. This made it impossible to resolve `MyMethod` from the typeless nil.

---

## What to Avoid (Failed Approaches)

1.  **The "Simple" Workaround**: Do not attempt to bypass interpreter modifications by creating a more explicit configuration in the `minigo` script (e.g., `{ Fn: (*my.Type)(nil), MethodName: "MyMethod" }`). This approach is fundamentally flawed because the `minigo` script runs in an isolated context and cannot resolve Go type identifiers like `my.Type` without the interpreter's help. This leads to "identifier not found" errors during the script's evaluation.

2.  **Hacking `applyFunction`**: Do not try to make `GoSourceFunction` callable by converting it to a standard `*object.Function` on the fly within `applyFunction`. This approach fails because it does not correctly propagate the function's original definition environment (`DefEnv`). This breaks the ability to resolve other symbols (functions, variables) from the same package, causing "identifier not found" errors for transitive imports.

## What is Necessary (The Correct Path)

The only robust solution is to **modify the `minigo` interpreter** to make it aware of Go's type system in a deeper way.

1.  **Typed Representations**: The interpreter needs distinct internal objects to represent Go concepts:
    *   `TypedNil`: For `nil` pointers that retain their type.
    *   `GoMethod`: To represent a method that has been successfully resolved from a type.
    *   `GoSourceFunction`: To represent a standalone Go function, crucially storing its **definition environment** and **package path**.

2.  **Rich `StructDefinition`**: When the interpreter learns about a Go struct (from `go-scan`), its internal representation (`StructDefinition`) must be augmented to store a map of its methods (`GoMethods`).

3.  **Correct Evaluation Logic**:
    *   The evaluator must be taught to create `TypedNil` objects when it encounters typed nils from Go.
    *   The selector logic (`evalSelectorExpr`) must be updated to handle method lookups on `TypedNil` objects.
    *   The function application logic (`applyFunction`) must be updated to handle `GoSourceFunction` objects, using their stored `DefEnv` to correctly execute their body in the proper scope.

---

## Recommended Incremental Task List

This breaks down the correct approach into smaller, verifiable steps.

### Part 1: Enhance `minigo` Objects and Evaluation

1.  **Task 1.1: Define New Objects**.
    *   In `minigo/object/object.go`, add the definitions for `TypedNil`, `GoMethod`, and `GoSourceFunction`.
    *   Add the `GoMethods map[string]*scanner.FunctionInfo` field to `StructDefinition`.
    *   *Verification*: The code should compile.

2.  **Task 1.2: Populate `StructDefinition.GoMethods`**.
    *   In `minigo/evaluator/evaluator.go`, locate the `findSymbolInPackageInfo` function.
    *   When a `goscan.StructKind` is processed, iterate through the `pkgInfo.Functions` and populate the `GoMethods` map on the new `object.StructDefinition`.
    *   *Verification*: This is hard to test in isolation, but is a prerequisite for the next steps.

3.  **Task 1.3: Implement `TypedNil` Creation**.
    *   In `minigo/evaluator/evaluator.go`, modify the `nativeToValue` function.
    *   In the `case reflect.Ptr, reflect.Interface:`, when `val.IsNil()` is true, add logic to resolve the `reflect.Type` to a `scanner.TypeInfo` and return a `*object.TypedNil`.
    *   *Verification*: Create a new test case in `minigo` that injects a typed nil pointer and asserts that the resulting object is a `TypedNil` with the correct type information.

4.  **Task 1.4: Implement Method Resolution on `TypedNil`**.
    *   In `minigo/evaluator/evaluator.go`, add a `case *object.TypedNil:` to `evalSelectorExpr`.
    *   This case should look up the selector in the `GoMethods` map of the struct definition corresponding to the `TypedNil`'s `TypeInfo`.
    *   If found, it should return a new `*object.GoMethod`.
    *   *Verification*: Create a new test case that evaluates a script like `var p *pkg.T; p.MyMethod` and asserts that the result is a `*object.GoMethod`.

5.  **Task 1.5: Implement `GoSourceFunction` Creation and Execution**.
    *   In `findSymbolInPackageInfo`, when a standalone function is found, return a `*object.GoSourceFunction`, populating it with the `*scanner.FunctionInfo`, the `pkgInfo.Path`, and the `pkgEnv`.
    *   In `applyFunction`, add a `case *object.GoSourceFunction:`. This case should create a temporary `*object.Function` using the `AstDecl` from the `GoSourceFunction` and, critically, the `DefEnv` from the `GoSourceFunction`. Then, fall through to the existing `*object.Function` execution logic.
    *   *Verification*: The existing `minigo` tests for transitive imports (`TestTransitiveImport`) should now pass.

### Part 2: Update `docgen`

6.  **Task 2.1: Update `PatternConfig` and Loader**.
    *   Modify `examples/docgen/patterns/patterns.go` to use `Fn any` and `MethodName string`.
    *   Modify `examples/docgen/loader.go`, implementing `computeKey` and the manual unmarshalling logic.
    *   *Verification*: The `docgen` code should compile.

7.  **Task 2.2: Update and Verify Test Cases**.
    *   Update all three `patterns.go` files in the `examples/docgen/testdata` subdirectories to use the new `Fn` syntax.
    *   Add the necessary `replace` directive to `examples/docgen/go.mod`.
    *   Run `cd /app && make test`.
    *   *Verification*: All tests, including the `docgen` tests, should now pass.
