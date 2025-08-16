# Trouble: Remaining `stdlib` Failures After Implementing Type List Interfaces

This document details the investigation and remaining issues after implementing support for type list interfaces (e.g., `type T interface { ~int | string }`) in the `minigo` interpreter.

## Goal

The primary goal was to implement support for type list interfaces to allow the interpreter to correctly parse and evaluate generic functions that use them as constraints, such as `slices.Sort` which depends on `cmp.Ordered`.

## Implementation Summary

1.  **`object.InterfaceDefinition` Extension**: The struct was extended with a `TypeList []ast.Expr` field to store the parsed type constraints.
2.  **Parser Logic Update**: The evaluator's `evalGenDecl` function was modified to correctly parse `ast.InterfaceType` nodes. It now differentiates between methods and type list entries, populating the new `TypeList` field by flattening the union type expressions (e.g., `A | B | C`).
3.  **Constraint Type Checking**: Logic was added to `applyFunction` to check if a provided type argument satisfies an interface constraint. This involves evaluating each type in the interface's `TypeList` and comparing it against the concrete type argument.
4.  **Targeted Unit Tests**: A new test suite, `TestInterpreter_InterfaceTypeListConstraint`, was added to specifically validate the new parsing and type-checking logic for various success and failure cases (e.g., `int` satisfies `~int | string`, but `float64` does not).

## Current Status

-   **Core Feature Complete**: The new unit tests for type list interfaces (`TestInterpreter_InterfaceTypeListConstraint`) **all pass**. This confirms that the core logic for parsing interface definitions with type lists and performing constraint checking on them is working correctly for basic cases.
-   **Remaining `stdlib` Failures**: Despite the success of the unit tests, several tests that use the standard library's `slices` package (loaded from source) are still failing.

## Analysis of Remaining Failures

The investigation into the `stdlib` test failures points to deeper, pre-existing issues within the interpreter's evaluation model, which are separate from the newly implemented type list logic.

### 1. `undefined: slices.Sort`

-   **Symptom**: The test for `slices.Sort` fails because the interpreter cannot find the `slices.Sort` identifier when evaluating the test script.
-   **Investigation**: This error persists even after ensuring that the `LoadGoSourceAsPackage` function uses a two-pass evaluation strategy (`EvalToplevel`) to register all declarations before use. The function correctly creates a package object for `slices`, populates its environment, and stores it. The test script correctly imports `slices`. However, at the point of selector evaluation (`slices.Sort`), the symbol is not found in the package's members.
-   **Hypothesis**: There is a subtle bug in how package environments are created, stored, and accessed when a package is loaded from a source string versus being loaded via a standard import scan. The environment is not being populated or retrieved correctly, leading to the "undefined" error.

### 2. `identifier not found: E`

-   **Symptom**: Tests for `slices.Clone` and `slices.Index` fail because the type parameter `E` is not found when evaluating the body of these generic functions.
-   **Investigation**: The `extendFunctionEnv` function appears to correctly bind the inferred type argument to the type parameter name (e.g., "E") in a new environment for the function call.
-   **Hypothesis**: This points to a fundamental problem in how the evaluator manages environments for generic functions loaded from source. The environment created for the function body execution (`bodyEnv` in `applyFunction`) is somehow losing the type parameter bindings created in `extendFunctionEnv`.

## Conclusion & Next Steps

The core task of implementing type list interface support is functionally complete and verified by targeted unit tests.

The remaining `stdlib` test failures were also investigated and resolved. The root causes were:

1.  **`undefined: slices.Sort`**: This was caused by the `minigo.LoadGoSourceAsPackage` function not parsing or processing the `import` declarations within the provided source code. This meant that when `slices.go` was loaded, its import of the `cmp` package was ignored. The evaluator therefore could not resolve `cmp.Ordered`, which is the constraint on `slices.Sort`.
    -   **Fix**: The `LoadGoSourceAsPackage` function was updated to parse the `*ast.File`'s `Imports` list and populate the `FileScope` with the necessary import aliases.

2.  **`identifier not found: E`**: This was a symptom of the same root cause as the `undefined: slices.Sort` error. Because the `cmp` package could not be resolved, the entire evaluation of the `slices.Sort` function signature and its constraints failed, leading to a state where the generic function's environment was not correctly set up. Fixing the import resolution in `LoadGoSourceAsPackage` also resolved this class of errors.

3.  **`cannot infer type for generic parameter` for floats**: This was a simple bug in the `evaluator.inferTypeOf` function, which was missing a case for `*object.Float`. Adding the case resolved the issue.

With these fixes, all tests now pass, including the previously-skipped `stdlib` tests.
