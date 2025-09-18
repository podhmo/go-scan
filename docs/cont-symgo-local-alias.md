
### Initial Prompt

> Fix a bug in the `find-orphans` example, which is triggered when running it in library mode (`--mode lib`). The bug manifests as an `identifier not found: Alias` error, originating from the `symgo` symbolic execution engine.

### Goal

The main goal is to fix the `symgo` evaluator so that it correctly handles local type definitions within function bodies. This will allow the `find-orphans` tool to analyze code that uses this pattern, such as the `UnmarshalJSON` methods in the codebase, without crashing.

### Initial Implementation Attempt

My first attempt involved identifying the root cause, which was that `evalGenDecl` in `symgo/evaluator/evaluator.go` only handled `var` declarations and ignored `type` declarations. My plan was to:
1.  Add a new test case (`symgo/evaluator_local_type_test.go`) that specifically reproduced the bug in a controlled manner.
2.  Modify `evalGenDecl` to handle `token.TYPE` by resolving the local type and adding it to the current function's environment.
3.  Modify `evalCompositeLit` to look up types in the environment first before falling back to the scanner.

### Roadblocks & Key Discoveries

The initial implementation was more complex than anticipated and led to several roadblocks:

1.  **Massive Regressions:** My first attempt to modify `evalCompositeLit` was flawed. I made it prioritize resolving types using `e.Eval()`, which checks the local environment. When it failed to find a top-level type (which is expected, as they aren't in the *local* environment), my code immediately returned an error. This broke the fallback path that used the scanner's global type information. The key discovery was that **the fallback logic is not optional; it's essential for all top-level type resolution**, and my error handling was preventing it from ever being reached.

2.  **Panic on Complex Literals:** The flawed logic also introduced a panic when evaluating composite literals that contained function literals. This was a side effect of the same error-handling bug, where a `nil` value was passed down the line after an initial resolution error.

3.  **The `object.Type` and `scan.FieldType` Mismatch:** A core challenge was that `e.Eval()` on a type identifier returns an `object.Type` (which contains a `scanner.TypeInfo`), but the rest of `evalCompositeLit` is built to work with a `scanner.FieldType`. There is no straightforward, built-in way to get from the resolved `TypeInfo` of an alias back to a correctly-formed `FieldType` that represents that alias. My attempts to reconstruct it manually were buggy. The key discovery here was that the `scanner.TypeInfo` contains a `Node` field pointing back to the original AST declaration (`*ast.TypeSpec`). This provides a reliable way to get the RHS of the type definition and resolve its `FieldType`, which can then be adapted for the alias.

### Major Refactoring Effort

Based on these discoveries, the refined approach was:
1.  **`evalGenDecl`**: This part remained simple. It recognizes `case token.TYPE` and creates a new `TypeInfo` for the local type, making sure to store the `*ast.TypeSpec` node in the `Node` field. It then puts an `object.Type` containing this new `TypeInfo` into the local environment.
2.  **`evalCompositeLit`**: This function was refactored significantly.
    - It first calls `e.Eval()` on the composite literal's type identifier.
    - It does **not** treat an error as fatal. Instead, it checks if the result is a valid `*object.Type`.
    - If it is, it uses the `Node` field from the contained `TypeInfo` to get back to the `*ast.TypeSpec`. From there, it can safely resolve the `FieldType` of the *underlying* type and create a new `FieldType` for the alias.
    - If `e.Eval()` fails or doesn't return an `*object.Type`, the logic gracefully falls back to using `e.scanner.TypeInfoFromExpr()`, preserving the original behavior for all top-level types and preventing the regressions.

### Current Status

The code has been reverted to its original state before the failed attempts. The logic described in "Major Refactoring Effort" is the correct path forward, but I was unable to apply it due to repeated, trivial errors in using the available tooling. The immediate next step is to correctly apply this two-part patch.

### References

*   `docs/trouble-symgo3.md`: Contains the history of this bug and the failed implementation attempts.
*   `symgo/evaluator/evaluator.go`: The file containing the functions that need to be modified.
*   `symgo/evaluator_local_type_test.go`: The test file created to reproduce this specific bug.

### TODO / Next Steps

1.  Apply the correct patch to `symgo/evaluator/evaluator.go` to modify `evalGenDecl` so it handles `token.TYPE`.
2.  Apply the correct patch to `symgo/evaluator/evaluator.go` to modify `evalCompositeLit` with the robust fallback logic.
3.  Run all tests (`go test -v ./...`) to confirm that `TestEval_LocalTypeDefinitionInMethod` passes and that no regressions have been introduced.
4.  Address the remaining failure in `TestEval_LocalTypeDefinition`, which expects the underlying type name instead of the alias name. This may require a minor adjustment to how an `object.Instance`'s `TypeName` is constructed.
5.  Submit the final, working solution.
