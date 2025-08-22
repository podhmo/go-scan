# Trouble: Refactoring docgen to use type-safe function references

This document outlines the series of failed attempts to refactor the `docgen` tool's custom pattern configuration. The goal was to replace string-based keys like `Key: "my.package.MyFunc"` with direct, type-safe function references like `Fn: my.package.MyFunc` in a `minigo` script.

## The Core Challenge

The primary difficulty was handling method references on nil pointers, such as `var p *my.Type; ... { Fn: p.MyMethod }`. The `minigo` interpreter, by default, would evaluate `p` to a raw `object.NIL`, losing all type information. This made it impossible to resolve `MyMethod` from the typeless nil.

## Attempt 1: The "Right Way" with Flawed Execution

My initial and conceptually correct plan was to modify the `minigo` interpreter to support "typed nils".

- **Plan**:
  1.  Introduce a `TypedNil` object in `minigo` to hold the `scanner.TypeInfo` of a nil pointer.
  2.  Introduce a `GoMethod` object to represent a resolved method from a Go type.
  3.  Augment `minigo`'s `StructDefinition` to include a map of `GoMethods` populated by `go-scan`.
  4.  Update the `minigo` evaluator (`evalSelectorExpr`) to handle method calls on `TypedNil` objects by looking up the method in the struct's `GoMethods` map.
  5.  Update the `docgen` loader to manually unmarshal the new complex objects from the `minigo` script result.

- **Failures**: This plan failed repeatedly due to a cascade of implementation errors and a fundamental misunderstanding of the `minigo` object model and evaluator loop.
    - I repeatedly confused `scanner.FunctionInfo` with other types like `ast.FuncDecl` or a non-existent `scanner.Function`.
    - I made incorrect assumptions about which structs held which data (e.g., assuming `FunctionInfo` had a `PkgPath` when it didn't).
    - My most significant error was in making `GoSourceFunction` (a representation of a Go function) callable. My initial attempt was to convert it on-the-fly to a `minigo` `*object.Function`, but this failed because it didn't correctly propagate the function's definition environment (`DefEnv`). This broke tests that relied on transitive imports, as the called function couldn't find symbols from its own package.

This led to a frustrating loop of fixing one build error only to create a new, more subtle runtime error.

## Attempt 2: The "Simple" Workaround

After getting stuck on the interpreter modifications, I pivoted to what I thought was a simpler workaround.

- **Plan**:
  1.  Avoid modifying `minigo` entirely.
  2.  Change the `docgen` configuration to be more explicit, like:
      ```go
      {
          Fn: (*my.Type)(nil), // A typed nil to get package/type info
          MethodName: "MyMethod", // The method name as a string
          //...
      }
      ```
  3.  Update the `docgen` loader's `computeKey` function to use reflection on the `Fn`'s type and the `MethodName` string to construct the key.

- **Failure**: This failed because the `minigo` script itself couldn't resolve the identifiers. When the script `var p *my.Type` is evaluated, `minigo` doesn't automatically know about the Go types from the code being analyzed. The script runs in its own context. This approach was fundamentally flawed from the start.

## Conclusion and Path Forward

After multiple failures and a full repository reset, it is clear that **Attempt 1 was the correct path**. The complexity of the `minigo` interpreter cannot be bypassed with simple workarounds.

The key to success lies in a careful and correct implementation of the interpreter modifications:

1.  A `GoSourceFunction` object **must** be created to represent a Go function. It must store not only the `*scanner.FunctionInfo` but also the **package path** and, crucially, the **definition environment (`DefEnv`)** from the package it was defined in.
2.  The `applyFunction` logic in the evaluator **must** be updated to handle `GoSourceFunction`. When it encounters one, it must use the `DefEnv` to execute the function's body, ensuring that package-level variables and other functions are in scope.
3.  The `docgen` loader's `computeKey` function can then reliably use the `PkgPath` and `Func.Name` from the `GoSourceFunction` to generate the correct key for the analyzer.

Any future attempts should restart from a clean slate and meticulously follow this corrected version of the initial plan. The core lesson is that the environment and context of execution are critical in an interpreter, and this information must be correctly plumbed through the object model.
