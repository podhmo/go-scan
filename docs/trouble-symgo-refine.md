# `symgo` Refinement 2: Troubleshooting Analysis Regressions

This document details the troubleshooting process for fixing the `symgo` analysis engine, as outlined in `plan-symgo-refine2.md`. It includes historical context from a previous fix and analysis of new, similar regressions.

## Current Status

As of the latest run of `make -C examples/find-orphans e2e`, the "not a function" error has been resolved. However, the "infinite recursion" error persists.

### Analysis of Failures

#### 1. [RESOLVED] Standard Library Interaction Failure (`not a function: ...`)

*   **Symptom**: Multiple `main` functions across the `examples/` directory fail during symbolic execution with an error like `not a function: TYPE` or `not a function: INTEGER`. This error consistently occurs on lines that call functions from the standard `flag` package (e.g., `flag.String()`, `flag.Bool()`).
*   **Root Cause**: The `ScanPolicy` correctly prevents `symgo` from scanning the source code of the standard library. However, the fallback logic in `evalSelectorExpr` and `ResolveFunction` was flawed. When encountering an identifier in an external package that it cannot resolve, it was returning a generic `*object.SymbolicPlaceholder` instead of a more specific `*object.UnresolvedFunction`.
*   **Resolution**: The logic in `symgo/evaluator/resolver.go`'s `ResolveFunction` and `symgo/evaluator/evaluator.go`'s `evalSelectorExpr` was corrected to always return an `*object.UnresolvedFunction` for functions in packages that are not scanned. This allows the `applyFunction` logic to gracefully handle these external calls by creating a symbolic result based on the function's signature.

#### 2. [UNRESOLVED] Infinite Recursion (`infinite recursion detected: New`)

*   **Symptom**: The symbolic execution of `examples/minigo.main` fails with an infinite recursion error. The recursion is triggered when `symgo` attempts to analyze the `goscan.New` function.
*   **Root Cause**: The issue appears when the `find-orphans` tool analyzes code that itself uses the `go-scan` library, leading to a self-referential analysis loop. The existing recursion check (`frame.Fn.Def == f.Def && frame.Fn.Receiver == f.Receiver`) should theoretically prevent this, but is failing for reasons that are not yet understood. Further investigation is required.

### Conclusion

The `not a function` errors have been fixed by making the handling of external symbols more robust. The `infinite recursion` issue remains and requires a deeper investigation into how function definitions and package environments are cached and compared during self-referential analysis.

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

1.  **Correct Scoping for Placeholders**: Instead of removing the placeholder logic from `applyFunction`, it was corrected. The line `extendedEnv.Set(name, &object.Package{... Env: object.NewEnvironment()})` was changed to `extendedEnv.Set(name, &object.Package{... Env: object.NewEnclosedEnvironment(e.UniverseEnv)})`. This ensured that the placeholder package objects created for imports had a correctly-scoped environment from the beginning, fixing the `identifier not found` issue without altering the evaluator's fundamental logic.

2.  **Robust Test Configuration**: With the scoping fixed, the `infinite recursion` issue in the `find-orphans` e2e test persisted. This was solved by making the tool itself more robust. A `ScanPolicyFunc` was added to `examples/find-orphans/main.go` to prevent the `symgo` interpreter from analyzing packages outside the current workspace (like the standard library). This is the correct architectural approach for a tool that should only be concerned with user code. This change also fixed a regression in the `docgen` tests. A similar fix was applied to a failing unit test (`TestSymgo_IntraPackageConstantResolution`), which needed `goscan.WithGoModuleResolver()` added to its scanner configuration to correctly locate standard library packages during testing.

After applying this combination of fixes, all unit tests and the `find-orphans` e2e test were reported to pass successfully. The current failures indicate a regression.
