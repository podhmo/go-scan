# `symgo`: Incorrect Handling of Multi-Return Assignments from Unscannable Packages

This document details an issue where the `symgo` symbolic execution engine fails to correctly analyze code that assigns the results of a multi-return function call when that function belongs to a package outside the current scanning policy.

## 1. Problem Description

When `symgo` encounters a function call from a package that is not part of the primary analysis scope (i.e., an "unscannable" or "out-of-policy" package), it correctly treats the call as a symbolic boundary. The `applyFunction` in `evaluator.go` returns a `*object.SymbolicPlaceholder` to represent the unknown result of this external call.

However, a problem arises when this external function returns multiple values and is used in a multi-value assignment statement (e.g., `val, err := ext.Func()`).

The `evalAssignStmt` function, which handles assignments, receives the single `SymbolicPlaceholder` from the right-hand side. It expects a `*object.MultiReturn` object in this context. When it doesn't find one, it incorrectly assumes the assignment is invalid and logs a warning: `expected multi-return value on RHS of assignment`. It then assigns a generic placeholder to each of the left-hand-side variables, losing any potential type information and failing to model the assignment correctly.

## 2. Example Log Output

The following log snippet, generated while analyzing the `find-orphans` tool, demonstrates the issue. The code being analyzed is `wf, err := modfile.ParseWork(...)`, where `golang.org/x/mod/modfile` is an unscannable external package.

```
time=2025-09-10T23:58:10.435+09:00 level=DEBUG source=$HOME/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator.go:173 msg="evaluating node" type=*ast.Ident pos=$HOME/ghq/github.com/podhmo/go-scan/examples/find-orphans/main.go:90:52 source=nil

time=2025-09-10T23:58:10.435+09:00 level=DEBUG source=$HOME/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator.go:2618 msg="could not scan potential package for ident" in_func=discoverModules in_func_pos=$HOME/ghq/github.com/podhmo/go-scan/examples/find-orphans/main.go:168:21 exec_pos=$HOME/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator.go:2545 ident=nil path=golang.org/x/mod/modfile error=<nil>
time=2025-09-10T23:58:10.435+09:00 level=DEBUG source=$HOME/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator.go:2618 msg=applyFunction in_func="<Symbolic: unresolved identifier ParseWork in unscannable package golang.org/x/mod/modfile>" in_func_pos=$HOME/ghq/github.com/podhmo/go-scan/examples/find-orphans/main.go:90:14 exec_pos=$HOME/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator.go:2925 in_func="<Symbolic: unresolved identifier ParseWork in unscannable package golang.org/x/mod/modfile>" in_func_pos=$HOME/ghq/github.com/podhmo/go-scan/examples/find-orphans/main.go:90:14 exec_pos=804003 type=SYMBOLIC_PLACEHOLDER value="<Symbolic: unresolved identifier ParseWork in unscannable package golang.org/x/mod/modfile>" args="<Symbolic: result of external call to Join>, <Symbolic: result of external call to ReadFile (result 0)>, nil"
time=2025-09-10T23:58:10.435+09:00 level=WARN source=$HOME/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator.go:2618 msg="expected multi-return value on RHS of assignment" in_func=discoverModules in_func_pos=$HOME/ghq/github.com/podhmo/go-scan/examples/find-orphans/main.go:168:21 exec_pos=$HOME/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator.go:2193 got_type=SYMBOLIC_PLACEHOLDER
(*object.SymbolicPlaceholder)(0x14002b15180)({
 BaseObject: (object.BaseObject) {
  ResolvedTypeInfo: (*scanner.TypeInfo)(<nil>),
  ResolvedFieldType: (*scanner.FieldType)(<nil>)
 },
 Reason: (string) (len=109) "result of calling <Symbolic: unresolved identifier ParseWork in unscannable package golang.org/x/mod/modfile>",
 UnderlyingFunc: (*scanner.FunctionInfo)(<nil>),
 Package: (*scanner.PackageInfo)(<nil>),
 Receiver: (object.Object) <nil>,
 PossibleConcreteTypes: ([]*scanner.FieldType) <nil>,
 inspectCache: (string) "",
 cacheValid: (bool) false
})
```

- `applyFunction` returns a single `SYMBOLIC_PLACEHOLDER` for the call to `ParseWork`.
- `evalAssignStmt` receives this placeholder, logs the `WARN` message, and fails to model the assignment correctly.

## 3. Root Cause and Proposed Solution

The root cause is that the symbolic execution engine does not propagate the context of the assignment (i.e., the number of expected return values) down to the function evaluation logic. The `applyFunction` returns a single placeholder because it has no information to suggest it should do otherwise.

The proposed solution is to modify `evalAssignStmt`. When it evaluates the RHS of a multi-value assignment and receives a single `*object.SymbolicPlaceholder`, it will infer the number of required return values from the number of variables on the LHS. It will then dynamically create a `*object.MultiReturn` object containing the appropriate number of new placeholders, effectively "expanding" the single placeholder to fit the assignment.

---

# `symgo`: Cross-Package Symbol Collision During Analysis

This document details a bug in the `symgo` symbolic execution engine where it incorrectly resolved function calls between separate `main` packages during a whole-workspace analysis, leading to a crash.

## 1. Symptom

When running the `find-orphans` tool on a workspace containing multiple `main` packages (e.g., via `make -C examples/find-orphans e2e`), the tool would crash with an `identifier not found` error.

The tool `find-orphans` is the executable performing the analysis. One of the packages being analyzed was `deps-walk`. The error indicated that the identifier `keys` could not be found, but `keys` is a helper function defined and used only within the `find-orphans` tool's own source code.

## 2. Evidence: The Stack Trace

The key to diagnosing the issue was the anomalous stack trace produced by the `symgo` engine at the time of the crash. The `in_func` and `in_func_pos` fields in the log refer to the source code being *analyzed*, while `exec_pos` refers to the location in the *evaluator* itself.

```
level=ERROR msg="symbolic execution failed for entry point" function=github.com/podhmo/go-scan/examples/deps-walk.main error="symgo runtime error: identifier not found: keys
	$HOME/ghq/github.com/podhmo/go-scan/examples/find-orphans/main.go:438:20:
		patternsToWalk := keys(a.scanPackages)
	$HOME/ghq/github.com/podhmo/go-scan/examples/find-orphans/main.go:272:9:	in analyze
		return a.analyze(ctx, asJSON)
	$HOME/ghq/github.com/podhmo/go-scan/examples/deps-walk/main.go:94:12:	in run
		if err := run(context.Background(), ...); err != nil {
	:0:0:	in main
```

Let's break down this trace:

-   `function=.../deps-walk.main`: The symbolic execution correctly started from the `main` function of the `deps-walk` package, which was one of the analysis targets.
-   `in run` at `.../deps-walk/main.go:94:12`: The execution correctly proceeded into the `run` function belonging to `deps-walk`.
-   `in analyze` at `.../find-orphans/main.go:272:9`: **This is the critical error.** The execution path incorrectly jumped from `deps-walk`'s `run` function into the `analyze` function of the `find-orphans` tool itself. This should be impossible, as these are two separate, unrelated `main` packages.
-   `identifier not found: keys` at `.../find-orphans/main.go:438:20`: The crash occurs because the code now executing inside `find-orphans` tries to call its internal helper function `keys`, which does not exist in the context of the `deps-walk` analysis.

This demonstrates that the `symgo` interpreter "crossed the streams," mixing the code of the analysis tool with the code of the analysis target.

## 3. Root Cause: Global Environment Contamination

The bug was caused by a state management issue in the `symgo` evaluator (`symgo/evaluator/evaluator.go`).

1.  **Shared Global Environment:** The interpreter was using a single, top-level environment for all packages being analyzed.
2.  **Flawed Symbol Loading:** The `evalFile` function, which loads a file's symbols (functions, vars, etc.), was designed to find a package-specific sub-environment. However, due to a lookup failure, it would fall back to loading all symbols directly into the shared global environment.
3.  **Function Name Collision:** Both the `find-orphans` package and the `deps-walk` package define a function named `run`. When the evaluator loaded all packages, the `run` function from one package would overwrite the `run` function from the other in the global environment.
4.  **Incorrect Function Resolution:** When the symbolic execution of `deps-walk.main` reached the call to `run()`, the interpreter looked up the identifier `run` in the contaminated global environment. It incorrectly resolved this call to the `run` function belonging to `find-orphans`, leading to the execution jump and subsequent crash.

## 4. The Fix

The solution was to enforce strict environment isolation for each package during evaluation.

The `evalFile` function in `symgo/evaluator/evaluator.go` was refactored. Instead of relying on a flawed lookup in the environment, it now uses the evaluator's internal package cache (`pkgCache`), which is reliably keyed by unique package import paths.

**Old Logic (Simplified):**
```go
// Tries to find a package environment within the global 'env'.
// Fails, and falls back to using the global 'env' itself.
targetEnv = env // Becomes the global environment.
// Populates the global environment with symbols from the current package.
ensurePackageEnvPopulated(ctx, pkgObj, targetEnv)
```

**New Logic (Simplified):**
```go
// Gets the correct package object (and its environment) from the cache.
pkgObj, err := e.getOrLoadPackage(ctx, pkg.ImportPath)
// Always use the package's own, isolated environment.
targetEnv := pkgObj.Env
// Populates the package's own environment. No global contamination.
ensurePackageEnvPopulated(ctx, pkgObj)
```

This change ensures that the symbols of each package are loaded into and resolved from their own dedicated environment, preventing collisions and ensuring the symbolic execution path correctly mirrors Go's own scoping rules.

This approach is localized, safe, and correctly models the programmer's intent as expressed by the multi-value assignment syntax.
