# Trouble with symgo: Unexported Variable Resolution Across Packages

This document details a bug in the `symgo` symbolic execution engine where it failed to resolve unexported, package-level variables when they were accessed from a different package. This led to incorrect "identifier not found" errors and, consequently, false positives in tools like `find-orphans`.

## Problem Description

When running analysis that involved cross-package evaluation, `symgo` would fail if a function in Package B accessed an unexported, package-level variable also defined in Package B, but the analysis was initiated from Package A.

For example, consider the following setup:

**`lib/lib.go`**
```go
package lib

// unexported package-level variable
var secret = "hello from unexported var"

// exported function
func GetGreeting() string {
	return secret
}
```

**`myapp/main.go`**
```go
package main

import "example.com/lib"

func main() string {
	return lib.GetGreeting()
}
```

When `symgo` attempted to evaluate `main.main`, it would fail with `identifier not found: secret`. This occurred because the environment for the `lib` package was not correctly populated with its package-level variables before functions from that package were executed.

## Root Cause Analysis

The root cause was that `symgo/evaluator/evaluator.go`'s `ensurePackageEnvPopulated` function, which is responsible for setting up the environment for imported packages, did not handle `var` declarations. It correctly populated functions and constants, but completely ignored package-level variables.

When `applyFunction` was called for `lib.GetGreeting`, the environment for `lib` was missing the `secret` variable, leading to the "identifier not found" error.

## Solution: Lazy Evaluation of Package-Level Variables

The bug was fixed by implementing a lazy-evaluation mechanism for package-level variables. This ensures that a variable's initializer is only evaluated when the variable is first accessed, guaranteeing that the environment is in the correct state at that moment.

The fix involved several key changes:

1.  **Scanner Enhancement**: The `go-scan/scanner` package was modified first. The `scanner.VariableInfo` struct now includes a pointer to the `*ast.GenDecl` of the variable. This provides `symgo` with the full declaration context, which is necessary for lazy evaluation.

2.  **Lazy Variable Objects**: The `symgo/object.Variable` struct was enhanced to store the necessary context for lazy evaluation, including its `Initializer` expression, its declaration environment (`DeclEnv`), and its declaration package (`DeclPkg`).

3.  **Updated Population Logic**: The `ensurePackageEnvPopulated` function in the evaluator was rewritten. Instead of trying to evaluate variables when a package is first imported, it now creates "lazy" `object.Variable` instances for each package-level variable and stores them in the package's environment.

4.  **On-Demand Evaluation**: A new helper function, `evalVariable`, was introduced. This function is called whenever an identifier resolves to a variable. It checks an `IsEvaluated` flag. If the flag is false, it evaluates the variable's stored `Initializer` expression within the correct declaration environment and package context. The result is then cached in the `Value` field, and the `IsEvaluated` flag is set to true for subsequent accesses.

This approach correctly resolves the dependencies. When `lib.GetGreeting` is executed, the `evalIdent` call for `secret` triggers `evalVariable`, which then evaluates the initializer `"hello from unexported var"` and successfully resolves the symbol. This fix has been verified to work for both simple variable access and more complex cases involving recursive functions that rely on package-level state.

## Verification

The fix was verified by running the `find-orphans` tool on the `examples/convert` package, which was a known complex case that triggered related bugs. After applying the lazy-evaluation fix and a subsequent correction to handle calls to function variables (like `flag.Usage`), the tool ran successfully and produced the correct output:

```sh
$ go run ./examples/find-orphans -v --primary-analysis-scope=github.com/podhmo/go-scan/examples/convert/parser,github.com/podhmo/go-scan/examples/convert/sampledata/source,github.com/podhmo/go-scan/examples/convert/sampledata/generated ./examples/convert
No orphans found.
...
```

The command completed without any fatal errors and correctly identified that there were no orphan functions in the target package, confirming the fix.
