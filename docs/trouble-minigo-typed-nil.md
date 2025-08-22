# Trouble: Implementing Method Values on Typed Nil Pointers

- **Date**: 2025-08-22
- **Author**: Jules
- **Status**: Unresolved

## 1. Objective

The goal was to implement support for resolving method values on typed `nil` pointers within the `minigo` interpreter. This would allow expressions like `(*MyStruct)(nil).MyMethod` to be evaluated, resolving to a `BoundMethod` object. This is a prerequisite for a type-safe `docgen` configuration feature.

## 2. Summary of Work

The implementation plan involved three main code modifications:

1.  **Modify `object.Pointer`**: The `object.Pointer` struct was modified to hold type information separately from its element. The `PointerType` field was added to store the pointer's type, allowing a `nil` pointer to retain this crucial information.

    ```go
    // in minigo/object/object.go
    type Pointer struct {
        PointerType Object // e.g., *object.PointerType
        Element     *Object
    }
    ```

2.  **Update Pointer Creation Logic**: Key parts of the evaluator in `minigo/evaluator/evaluator.go` were updated to populate this new `PointerType` field:
    - `evalAddressOfExpression` (the `&` operator).
    - The `new` builtin function.
    - `evalGenDecl` (to handle `var p *S` declarations, creating a typed `nil`).

3.  **Update Selector Logic**: The core logic in `evalSelectorExpr` was modified to handle the typed `nil` case. An `if/else` block was added to the `case *object.Pointer:` block. The `if` condition checks for a `nil` element and, if true, uses the `PointerType` field to look up the method on the underlying `StructDefinition`.

## 3. The Blocking Issue

Despite the logical changes appearing correct, the new test case consistently fails with the following error:

```
runtime error: base of selector expression is not a package or struct
```

This error originates from the `default` case of the main `switch` statement in `evalSelectorExpr`. This indicates that when the selector `p.SayHello` is evaluated, the `left` part (`p`) is evaluated to an `*object.Pointer`, but the `case *object.Pointer:` block is exited without returning a value.

### What Was Debugged

-   **Initial Build Errors**: Several build errors occurred due to a field/method name collision ("Type") and copy-paste errors (`ctx.NewError` vs `e.newError`). These were all resolved.
-   **File Patching Failures**: The `replace_with_git_merge_diff` tool repeatedly failed to apply patches correctly. This was worked around by deleting and re-creating the file with `create_file_with_block` to ensure the code was in the state I intended.
-   **Logic Flow in `evalSelectorExpr`**: The primary suspect was the logic flow within `case *object.Pointer:`. I confirmed that every possible code path within the `if l.Element == nil` block and the `else` block (containing the non-nil `switch`) has a `return` statement. The logic appears sound and should not "fall through".

### Conclusion

The root cause of the failure remains elusive. The `evalSelectorExpr` function does not behave as expected when encountering a typed `nil` pointer, and I was unable to find the subtle flaw in the Go code or the interpreter's evaluation flow that is causing this. The task is blocked.

The code is being submitted in its current, non-functional state to preserve the work done. The new failing test is in `minigo/minigo_nil_test.go`, and the partial implementation is in `minigo/object/object.go` and `minigo/evaluator/evaluator.go`.
