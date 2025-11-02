# `symgo` Refinement 2: Troubleshooting Analysis Regressions

This document details the troubleshooting process for fixing the `symgo` analysis engine, as outlined in `plan-symgo-refine2.md`. It includes historical context from a previous fix and analysis of new, similar regressions.

## Current Status (Regression)

As of the latest run of `make -C tools/find-orphans e2e`, the test is failing again, indicating a regression from the previously "fixed" state. The output log is populated with numerous `ERROR` and `WARN` level messages.

### Analysis of Current Failures

The failures fall into two main categories:

1.  **Standard Library Interaction Failure (`not a function: ...`)**:
    -   **Symptom**: Multiple `main` functions across the `examples/` directory fail during symbolic execution with an error like `not a function: TYPE` or `not a function: INTEGER`. This error consistently occurs on lines that call functions from the standard `flag` package (e.g., `flag.String()`, `flag.Bool()`).
    -   **Root Cause**: The `ScanPolicy` correctly prevents `symgo` from scanning the source code of the standard library. However, the fallback logic in `evalSelectorExpr` is flawed. When it encounters an identifier in an external package that it cannot resolve as a known function, variable, or constant (like `flag.String`), it incorrectly assumes the identifier must be a type and returns an `object.Type`. When the evaluator later tries to call this object, it fails with a `not a function` error. The fix is to change the fallback to return an `*object.UnresolvedFunction`, which the `applyFunction` logic can then gracefully handle by creating a symbolic result based on the function's signature.

2.  **Infinite Recursion (`infinite recursion detected: New`)**:
    -   **Symptom**: The symbolic execution of `examples/minigo.main` fails with an infinite recursion error. The recursion is triggered when `symgo` attempts to analyze the `goscan.New` function.
    -   **Root Cause**: The recursion detection logic in `applyFunction` is too strict for complex, cross-package call chains. The check requires the function definition, receiver, **and the source code position of the call** to be identical to detect recursion. In this failure mode, `goscan.New` is called recursively, but from a different location in the code, so the check `frame.Pos == callPos` fails, and the cycle is not detected. The fix is to relax the check by removing the comparison of the call site position, which will correctly identify the recursion.

### Conclusion

The `e2e` test failures point to specific flaws in the `symgo` evaluator's handling of external package symbols and complex recursion. The `ScanPolicy` is working as intended, but the evaluator's behavior when encountering these boundary conditions needs to be made more robust. The action plan in `plan-symgo-refine2.md` addresses these specific root causes.

---

## Historical Context: Previous E2E Test Failure Analysis and Resolution

*(The following sections describe a previous debugging and resolution cycle. While the issues were marked as resolved, the "Current Status" section above indicates they have regressed or that the original fixes were incomplete.)*

### Symptom 1: `identifier not found: findModuleRoot`

Initially, the e2e test failed with errors like `identifier not found: findModuleRoot`. This happened when `symgo` was symbolically executing `locator.New`, which calls the unexported function `findModuleRoot` from the same package.

This pointed to a package scoping issue. The `symgo` evaluator's `applyFunction` was creating placeholder `object.Package` instances for a function's imports, but it was creating them with a new, empty `object.Environment()`. This environment was not enclosed by the top-level `UniverseEnv`, so when the package was later populated, it lacked access to built-in functions, and its own unexported members were not being resolved correctly across files.

### Symptom 2: `infinite recursion detected: New`

An initial attempt to fix the scoping issue was to remove the placeholder creation logic from `applyFunction` altogether. This correctly forced `evalIdent` to use the central `getOrLoadPackage` function, which uses a proper, cached, and correctly-scoped environment.

This fixed the `identifier not found` error, but uncovered a new problem: the e2e test would now fail with `infinite recursion detected: New`. The symbolic execution of `goscan.New` would lead to a call to `locator.New`, which in turn involves file system operations (`os.Stat`, etc.) to find `go.mod`. The symbolic execution engine, lacking intrinsics for these `os` functions, would attempt to analyze the entire Go standard library, get confused, and end up re-evaluating `goscan.New`.

### Final Resolution (Historical): A Two-Part Fix

The final, successful solution involved two key changes:

1.  **Correct Scoping for Placeholders**: Instead of removing the placeholder logic in `applyFunction`, it was corrected. The line `extendedEnv.Set(name, &object.Package{... Env: object.NewEnvironment()})` was changed to `extendedEnv.Set(name, &object.Package{... Env: object.NewEnclosedEnvironment(e.UniverseEnv)})`. This ensured that the placeholder package objects created for imports had a correctly-scoped environment from the beginning, fixing the `identifier not found` issue without altering the evaluator's fundamental logic.

2.  **Robust Test Configuration**: With the scoping fixed, the `infinite recursion` issue in the `find-orphans` e2e test persisted. This was solved by making the tool itself more robust. A `ScanPolicyFunc` was added to `tools/find-orphans/main.go` to prevent the `symgo` interpreter from analyzing packages outside the current workspace (like the standard library). This is the correct architectural approach for a tool that should only be concerned with user code. This change also fixed a regression in the `docgen` tests. A similar fix was applied to a failing unit test (`TestSymgo_IntraPackageConstantResolution`), which needed `goscan.WithGoModuleResolver()` added to its scanner configuration to correctly locate standard library packages during testing.

After applying this combination of fixes, all unit tests and the `find-orphans` e2e test were reported to pass successfully. The current failures indicate a regression.
