# Trouble with symgo: Unexported Function Resolution Across Packages

This document details a bug in the `symgo` symbolic execution engine where it fa
ils to resolve unexported functions when the call originates from a different pa
ckage, leading to incorrect "identifier not found" errors and, consequently, fal
se positives in tools like `find-orphans`.

## Problem Description

When running the `find-orphans` tool on the `examples/convert` directory, the fu
nction `github.com/podhmo/go-scan/examples/convert.formatCode` is incorrectly re
ported as an orphan.

This is a symptom of a deeper issue. The symbolic execution fails during the ana
lysis of the `convert` example. The log shows the following error:

```
level=ERROR msg="identifier not found: processPackage" in_func=Parse in_func_pos
=$HOME/ghq/github.com/podhmo/go-scan/examples/convert/main.go:121:15
```

The call chain leading to the error is:
1.  `main.run()` (in `examples/convert/main.go`) calls `parser.Parse()`.
2.  `parser.Parse()` (in `examples/convert/parser/parser.go`) immediately calls
`processPackage()`, which is an **unexported** function within the same `parser`
 package.

The error message is revealing:
-   `identifier not found: processPackage`: The engine cannot find the function.
-   `in_func=Parse`: The error occurs while executing `Parse`.
-   `in_func_pos=.../main.go:121:15`: The position reported is the call site of
`parser.Parse()` in the `main` package, not the call site of `processPackage()`
inside the `parser` package.

This indicates that when `symgo`'s evaluator is executing the body of `parser.Pa
rse`, it is incorrectly using the scope or environment of the *calling* package
(`main`) instead of the *callee's* package (`parser`). Since `processPackage` is
 unexported, it is not visible in the `main` package's scope, leading to the fai
lure.

## How to Reproduce

1.  Build the `find-orphans` tool.
    ```bash
    go build -o /tmp/find-orphans ./tools/find-orphans
    ```

2.  Run `find-orphans` on the `examples/convert` directory.
    ```bash
    (cd examples/convert && /tmp/find-orphans -v ./ )
    ```

You will observe the "identifier not found" error for `processPackage` and a sub
sequent incorrect orphan report for `formatCode`.

## Root Cause Analysis

The root cause lies in `symgo/evaluator/evaluator.go`, specifically in how the e
valuation context (the `pkg *scanner.PackageInfo` parameter) is propagated durin
g recursive calls to the `Eval` function.

While `applyFunction` correctly identifies the callee's package (`fn.Package`) a
nd uses it to evaluate the function's body, there appears to be a flaw in how th
e `pkg` context is maintained through the chain of `eval*` function calls. When
`evalCallExpr` is invoked for an unexported function like `processPackage` from
inside the `parser` package, it seems to be doing so with the `pkg` context of t
he original `main` package still active.

When `evalIdent` is then called to resolve `processPackage`, it receives the inc
orrect package context. It looks for the unexported function in the wrong packag
e, fails to find it, and throws the error.

The fix requires ensuring that the `pkg *scanner.PackageInfo` parameter in the `
Eval` function and all its subsidiary `eval*` methods always reflects the packag
e of the code currently being executed. The `pkg` context must be correctly upda
ted when `applyFunction` begins evaluating a function from a different package.
The current implementation in `applyFunction` seems to attempt this, but a subtl
e flaw is causing the context to be lost or incorrect during the subsequent eval
uation of the function body's expressions.

A suspicious code block in `applyFunction` manually adds imports to the function
's environment. This logic is likely flawed, redundant, and may interfere with t
he correct lexical scoping provided by the enclosing package environment, `fn.En
v`. Removing or correcting this logic is a probable path to a solution.

## Deeper Investigation and New Findings

Further investigation revealed that the initial hypothesis, while plausible, was
 not the root cause. The problem is more fundamental, related to how package-lev
el environments are populated for imported packages.

### Initial Fix Attempts and Failures

The investigation started by focusing on the suspicious block in `applyFunction`
 that re-populates import information. Two fixes were attempted:

1.  **Removing the Block:** The entire block was commented out. This did not res
olve the issue.
2.  **Using the Package Cache:** The block was modified to use the evaluator's p
ackage cache (`getOrLoadPackage`) instead of creating new, empty package objects
. This also failed to fix the bug.

These failures, confirmed with a more reliable and isolated test case, indicated
 that the problem lay elsewhere.

### Discovery: Package-Level Variables are Not Evaluated

To create a more controlled testing environment, a new test (`TestCrossPackageUn
exportedResolution`) was added to `symgo/symgo_scope_test.go`. This test simulat
ed a cross-package call and checked for symbol resolution. The test used a packa
ge-level variable (`var count = 0`) to control a recursive call. This led to a c
ritical discovery when the test failed with:

```
identifier not found: count
```

This revealed a fundamental bug: **package-level `var` declarations from importe
d packages are not being evaluated at all.**

The function `ensurePackageEnvPopulated` in `symgo/evaluator/evaluator.go` is re
sponsible for populating the environment for imported packages on-demand. A clos
e inspection showed that it correctly handles `func` and `const` declarations, b
ut completely ignores `var` declarations. This is the true root cause of the res
olution failures seen in both the test and the `find-orphans` tool.

### Clarification: Unexported Function Resolution Works

To confirm that the `var` issue was the true root cause, a more minimal version
of the test (`TestCrossPackageUnexportedResolution_Minimal`) was created. This v
ersion removed the package-level variable and the recursion, testing only the cr
oss-package call to an unexported function.

**This minimal test passed.**

This result is significant because it proves that the initial hypothesis—a gener
al failure in resolving unexported functions across packages—was incorrect. The
symbolic execution engine *can* correctly resolve and execute unexported functio
ns from other packages, provided no unevaluated package-level state (like `var`s
) is involved.

Therefore, the failure of `find-orphans` to resolve `processPackage` is not a si
mple scoping bug but a side effect of the same underlying problem: the environme
nt for the `parser` package is not correctly populated with all its necessary co
mponents due to the failure to handle `var` declarations, which likely leads to
an unstable state that prevents subsequent lookups from succeeding.

### The Final Roadblock: Missing AST Information

An attempt was made to fix `ensurePackageEnvPopulated` by adding logic to evalua
te `var` declarations. This immediately hit a roadblock: the `scanner.VariableIn
fo` struct, which is provided by the `go-scan/scanner` dependency, does not stor
e the necessary `*ast.GenDecl` node required to re-evaluate the variable declara
tion. It only contains a more generic `ast.Node`, which is likely the `*ast.Valu
eSpec` for the variable.

This means a complete fix requires changes to the `scanner` package itself to ex
pose the full `*ast.GenDecl` node. The current plan is to modify `scanner.Variab
leInfo` and the scanner logic to include this information, and then use it in `s
ymgo` to correctly populate package environments. This documentation is being up
dated to record these findings before proceeding with the cross-package modifica
tion.

## Addendum: Lazy Evaluation and New Regressions

Based on the findings above, a fix was implemented.

### Solution: Lazy Evaluation of Package-Level Variables

The bug was fixed by implementing a lazy-evaluation mechanism for package-level variables. This ensures that a variable's initializer is only evaluated when the variable is first accessed, guaranteeing that the environment is in the correct state at that moment.

The fix involved several key changes:

1.  **Scanner Enhancement**: The `go-scan/scanner` package was modified first. The `scanner.VariableInfo` struct now includes a pointer to the `*ast.GenDecl` of the variable. This provides `symgo` with the full declaration context, which is necessary for lazy evaluation.

2.  **Lazy Variable Objects**: The `symgo/object.Variable` struct was enhanced to store the necessary context for lazy evaluation, including its `Initializer` expression, its declaration environment (`DeclEnv`), and its declaration package (`DeclPkg`).

3.  **Updated Population Logic**: The `ensurePackageEnvPopulated` function in the evaluator was rewritten. Instead of trying to evaluate variables when a package is first imported, it now creates "lazy" `object.Variable` instances for each package-level variable and stores them in the package's environment. This new logic correctly handles multi-variable declarations (e.g., `var a, b = 1, 2`) and declarations without initializers.

4.  **On-Demand Evaluation**: A new helper function, `evalVariable`, was introduced. This function is called whenever an identifier resolves to a variable. It checks an `IsEvaluated` flag. If the flag is false, it evaluates the variable's stored `Initializer` expression within the correct declaration environment and package context. The result is then cached in the `Value` field, and the `IsEvaluated` flag is set to true for subsequent accesses.

### Verification and New Findings

This fix successfully resolves the original issue. The `TestCrossPackageUnexportedResolution_WithVar` test, which specifically checks for the resolution of an unexported package-level variable, now passes.

However, as per the user's direction, this change was merged even though it causes a significant number of regressions in the existing test suite. This indicates that while the core problem of *populating* unexported variables is solved, the *triggering* of their lazy evaluation is not robust enough.

The new failures fall into several categories:
- **Incorrect "Orphan" Detections**: Tests like `TestFindOrphans` now fail because functions and methods that are in use are being reported as orphans. This suggests the evaluator is not correctly tracing all call paths.
- **Incorrect Symbolic Values**: Tests like `TestFeature_SprintfIntrinsic` fail because a function receives a symbolic placeholder (`<Symbolic: zero value for uninitialized variable>`) instead of a concrete value. This is the clearest evidence that a variable is being passed as an argument without its lazy initializer being triggered.
- **Infinite Recursion**: The `TestCrossPackageUnexportedResolution` test, which previously passed, now fails with an infinite recursion error. This indicates that the state of the `count` variable is not being correctly updated and propagated across the recursive calls in the new lazy model.

These regressions point to a systemic issue: lazy evaluation is only triggered when an identifier is directly resolved (in `evalIdent`), but not when a variable is used in other contexts, such as being an argument to a function. The next step will be to fix these regressions by ensuring that `evalVariable` is called whenever a variable's value is needed.

---
## Feedback (Post-Analysis)

A recent test run (`go test ./symgo/...`) confirms the findings of this document, particularly the "Addendum" section.

1.  **Correct Root Cause**: This document correctly identifies that the initial problem was a failure to evaluate package-level `var` declarations. The fix, implementing lazy evaluation, was the correct architectural choice.
2.  **Regressions Persist**: The test run confirms that the regressions introduced by the lazy-evaluation fix are still present in the codebase. The `TestCrossPackageUnexportedResolution` test still fails with an infinite recursion error, validating the analysis that the new lazy model has unresolved issues with state management in recursive calls.

The analysis and "next steps" outlined in the Addendum remain the correct path forward for stabilizing the `symgo` engine.
