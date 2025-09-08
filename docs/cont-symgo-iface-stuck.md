# Continuation Document for symgo Interface Discovery Task

This document outlines the progress made on the symgo interface discovery task and the blocking issue that prevented its completion.

## Task Goal

The goal is to improve `symgo`'s ability to analyze code that uses interfaces, specifically to handle cross-package implementations and to be independent of the order in which interfaces, implementations, and usages are discovered. A key requirement is the "conservative" tracing of method calls: when a method is called on an interface, the call should be traced to all known concrete implementers of that interface.

## Progress and Completed Changes

The following steps of the original plan were successfully completed:

1.  **Enhanced Type Relation Checking (`type_relation.go`)**:
    - The `Implements` function was refactored into a new `ImplementsContext` function.
    - This new function uses a `scanner.PackageResolver` to correctly look up methods and resolve types across package boundaries.
    - Type comparison was made more robust by using `FieldType.Resolve()` to compare canonical type definitions instead of simple name strings.

2.  **Introduced a Central Type Relation Registry**:
    - A new file `symgo/relations.go` was created, defining a thread-safe `TypeRelations` struct.
    - This registry is responsible for tracking all discovered structs and interfaces and maintaining a map of which structs implement which interfaces.
    - The `object` package was updated with `ImplementationPair` and `InterfaceCall` structs.
    - The `symgo.Interpreter` was updated to own an instance of this registry.

3.  **Initial Integration with the Evaluator**:
    - The `symgo.Interpreter.Eval` method was modified to call `relations.AddType` whenever it processes a package, ensuring the registry is populated as new types are discovered.

## Blocking Issue

The task is currently blocked on **Step 3: Integrate Registry with the Evaluator**.

The core logic requires modifying `symgo/evaluator/evaluator.go` to make it interact with the `TypeRelations` registry. Specifically, the `evalGenDecl` and `evalSelectorExpr` functions need to be updated.

**The Problem:** I am unable to modify the file `symgo/evaluator/evaluator.go`.

- My automated file editing tools (`overwrite_file_with_block` and `replace_with_git_merge_diff`) are consistently failing for this specific file.
- `overwrite_file_with_block` has repeatedly resulted in a corrupted or incomplete file on disk, leading to a large number of `undefined method` compilation errors.
- `replace_with_git_merge_diff` is consistently failing to find the search blocks (anchors) needed to apply targeted changes, even after re-reading the file to get the correct content. This suggests a subtle and persistent issue with the file's state that I cannot diagnose or resolve.
- I have attempted to restore the file to its original state with `restore_file` and re-apply the changes in small, careful increments, but these attempts have also failed.

Without the ability to modify this core file, the implementation cannot be completed.

## Next Steps

To continue this task, a developer with direct access to the file system needs to manually apply the following changes to `symgo/evaluator/evaluator.go`:

1.  **Modify `evalGenDecl`** to delegate `TYPE` declarations to a new `evalTypeDecl` function.
2.  **Add a new `evalTypeDecl` function**. This function should:
    - Find the `scanner.TypeInfo` for the declaration.
    - Call `e.relations.AddType(ctx, typeInfo)`.
    - For each new `ImplementationPair` returned, call a new `e.processNewImplementation` helper function.
3.  **Add a new `processNewImplementation` function**. This function should:
    - Get pending calls for the interface using `e.relations.GetPendingCalls()`.
    - For each pending call, find the corresponding method on the new struct implementation.
    - Trigger a symbolic call to that method using `e.applyFunction()`.
4.  **Modify `evalSelectorExpr`**. In the `case *object.SymbolicPlaceholder:` block, add logic at the beginning to check if the placeholder's `TypeInfo` is an interface. If it is:
    - Record a pending call using `e.relations.AddPendingCall()`.
    - Get all current implementers using `e.relations.GetImplementers()`.
    - For each implementer, dispatch a symbolic call to the corresponding method.
    - Return a new `SymbolicPlaceholder` for the result of the interface method call.
